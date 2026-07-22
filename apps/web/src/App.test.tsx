import { act, render, screen, waitFor } from '@testing-library/react'
import { MemoryRouter } from 'react-router-dom'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { App } from './App'
import { SESSION_IDLE_DEADLINE_EVENT } from './api/client'
import type { SessionResponse } from './api/contracts'

const SESSION: SessionResponse = {
  principal: { id: 'operator-1', display_name: 'Ada Operadora' },
  role: 'platform_operator',
  session: {
    started_at: '2026-07-15T12:00:00Z',
    idle_expires_at: '2099-07-15T12:30:00Z',
    absolute_expires_at: '2099-07-15T20:00:00Z',
  },
  csrf_token: 'csrf-safe-value',
}

function jsonResponse(body: unknown, status = 200): Response {
  return new Response(JSON.stringify(body), {
    status,
    headers: { 'Content-Type': 'application/json' },
  })
}

function renderRoute(route: string) {
  return render(
    <MemoryRouter initialEntries={[route]}>
      <App />
    </MemoryRouter>,
  )
}

afterEach(() => {
  vi.useRealTimers()
  vi.unstubAllGlobals()
})

describe('fluxo público de acesso', () => {
  it('inicia o login por navegação GET no BFF', () => {
    renderRoute('/entrar')
    expect(screen.getByRole('link', { name: 'Entrar com minha conta' })).toHaveAttribute(
      'href',
      '/api/v1/auth/login',
    )
  })

  it('explica uma sessão expirada sem detalhes técnicos', () => {
    renderRoute('/acesso/sessao-expirada')
    expect(
      screen.getByRole('heading', { name: 'Sua sessão terminou por segurança.' }),
    ).toBeInTheDocument()
    expect(screen.getByRole('link', { name: 'Entrar novamente' })).toHaveAttribute(
      'href',
      '/api/v1/auth/login',
    )
  })

  it('tenta novamente revalidando a área protegida', () => {
    renderRoute('/acesso/servico-indisponivel')
    expect(screen.getByRole('link', { name: 'Tentar novamente' })).toHaveAttribute(
      'href',
      '/controle',
    )
  })
})

describe('proteção do controle global', () => {
  it('não renderiza conteúdo protegido enquanto a sessão está pendente', () => {
    vi.stubGlobal('fetch', vi.fn(() => new Promise<Response>(() => undefined)))
    renderRoute('/controle')
    expect(screen.getByRole('heading', { name: 'Validando acesso' })).toBeInTheDocument()
    expect(screen.queryByText('Operações da plataforma')).not.toBeInTheDocument()
  })

  it('renderiza o controle somente para o papel exato e envia logout por formulário POST', async () => {
    vi.stubGlobal('fetch', vi.fn().mockResolvedValue(jsonResponse(SESSION)))
    const { container } = renderRoute('/controle')

    expect(await screen.findByRole('heading', { name: 'Olá, Ada Operadora.' })).toBeInTheDocument()
    const form = container.querySelector<HTMLFormElement>('form[action="/api/v1/auth/logout"]')
    expect(form).not.toBeNull()
    expect(form?.method).toBe('post')
    expect(form?.querySelector<HTMLInputElement>('input[name="csrf_token"]')?.value).toBe(
      'csrf-safe-value',
    )
  })

  it('nega uma sessão com papel diferente, mesmo quando autenticada', async () => {
    vi.stubGlobal(
      'fetch',
      vi.fn().mockResolvedValue(jsonResponse({ ...SESSION, role: 'tenant_admin' })),
    )
    renderRoute('/controle')

    expect(
      await screen.findByRole('heading', {
        name: 'Esta conta não pode acessar o controle global.',
      }),
    ).toBeInTheDocument()
    expect(screen.queryByText('Operações da plataforma')).not.toBeInTheDocument()
  })

  it('falha fechado quando a API devolve uma sessão malformada', async () => {
    vi.stubGlobal('fetch', vi.fn().mockResolvedValue(jsonResponse({ role: 'platform_operator' })))
    renderRoute('/controle')

    expect(
      await screen.findByRole('heading', { name: 'Não foi possível confirmar o acesso agora.' }),
    ).toBeInTheDocument()
    expect(screen.queryByText('Operações da plataforma')).not.toBeInTheDocument()
  })

  it('oculta o conteúdo enquanto revalida uma página restaurada do cache', async () => {
    let resolveRevalidation: ((response: Response) => void) | undefined
    const fetchMock = vi
      .fn()
      .mockResolvedValueOnce(jsonResponse(SESSION))
      .mockImplementationOnce(
        () =>
          new Promise<Response>((resolve) => {
            resolveRevalidation = resolve
          }),
      )
    vi.stubGlobal('fetch', fetchMock)
    renderRoute('/controle')
    expect(await screen.findByRole('heading', { name: 'Olá, Ada Operadora.' })).toBeInTheDocument()

    const restored = new Event('pageshow') as PageTransitionEvent
    Object.defineProperty(restored, 'persisted', { value: true })
    act(() => window.dispatchEvent(restored))

    expect(screen.getByRole('heading', { name: 'Validando acesso' })).toBeInTheDocument()
    expect(screen.queryByText('Operações da plataforma')).not.toBeInTheDocument()
    await act(async () => {
      resolveRevalidation?.(jsonResponse(SESSION))
    })
    expect(await screen.findByRole('heading', { name: 'Olá, Ada Operadora.' })).toBeInTheDocument()
  })

  it('ressincroniza o limite ocioso sem ultrapassar o limite absoluto', async () => {
    vi.useFakeTimers()
    const now = new Date('2026-07-21T12:00:00Z')
    vi.setSystemTime(now)
    const nearExpiry = {
      ...SESSION,
      session: {
        ...SESSION.session,
        idle_expires_at: new Date(now.getTime() + 1_000).toISOString(),
        absolute_expires_at: new Date(now.getTime() + 5_000).toISOString(),
      },
    }
    vi.stubGlobal('fetch', vi.fn().mockResolvedValue(jsonResponse(nearExpiry)))
    renderRoute('/controle')
    await act(async () => Promise.resolve())
    expect(screen.getByRole('heading', { name: 'Olá, Ada Operadora.' })).toBeInTheDocument()

    act(() => {
      window.dispatchEvent(
        new CustomEvent(SESSION_IDLE_DEADLINE_EVENT, {
          detail: { idleExpiresAt: new Date(now.getTime() + 4_000).toISOString() },
        }),
      )
    })
    act(() => vi.advanceTimersByTime(1_500))
    expect(screen.getByRole('heading', { name: 'Olá, Ada Operadora.' })).toBeInTheDocument()

    act(() => vi.advanceTimersByTime(2_500))
    expect(
      screen.getByRole('heading', { name: 'Sua sessão terminou por segurança.' }),
    ).toBeInTheDocument()
  })
})

describe('auditoria', () => {
  it('lista somente os eventos fornecidos pela API e permite abrir o detalhe', async () => {
    const auditPage = {
      items: [
        {
          id: 'event-1',
          event_type: 'security.session_started',
          occurred_at: '2026-07-15T12:02:00Z',
          actor: { id: 'operator-1', display_name: 'Ada Operadora' },
          role: 'platform_operator',
          context: 'global_control',
          result: 'success',
          reason_code: null,
          correlation_id: 'corr-safe-1',
          source: 'web',
        },
      ],
      next_cursor: null,
    }
    const fetchMock = vi
      .fn()
      .mockResolvedValueOnce(jsonResponse(SESSION))
      .mockResolvedValueOnce(jsonResponse(auditPage))
    vi.stubGlobal('fetch', fetchMock)

    renderRoute('/controle/auditoria')

    const detailLink = await screen.findByRole('link', {
      name: 'Ver detalhes de Sessão iniciada',
    })
    expect(screen.getByText('Ada Operadora', { selector: 'td' })).toBeInTheDocument()
    expect(detailLink).toHaveAttribute(
      'href',
      '/controle/auditoria/event-1',
    )
    await waitFor(() => expect(fetchMock).toHaveBeenCalledTimes(2))
    expect(fetchMock.mock.calls[1]?.[0]).toContain('/api/v1/platform/audit-events?period=7d')
  })
})
