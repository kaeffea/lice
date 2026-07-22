import { afterEach, expect, it, vi } from 'vitest'
import {
  listAuditEvents,
  SESSION_IDLE_DEADLINE_EVENT,
} from './client'

afterEach(() => {
  vi.unstubAllGlobals()
})

it('publica o novo prazo ocioso somente quando o backend envia uma data válida', async () => {
  const listener = vi.fn()
  window.addEventListener(SESSION_IDLE_DEADLINE_EVENT, listener)
  vi.stubGlobal(
    'fetch',
    vi.fn().mockResolvedValue(
      new Response(JSON.stringify({ items: [], next_cursor: null }), {
        status: 200,
        headers: {
          'Content-Type': 'application/json',
          'X-Lice-Session-Idle-Expires-At': '2026-07-21T12:30:00Z',
        },
      }),
    ),
  )

  await listAuditEvents({ period: '24h' })

  expect(listener).toHaveBeenCalledTimes(1)
  expect((listener.mock.calls[0]?.[0] as CustomEvent).detail).toEqual({
    idleExpiresAt: '2026-07-21T12:30:00Z',
  })
  window.removeEventListener(SESSION_IDLE_DEADLINE_EVENT, listener)
})

it('ignora um prazo ocioso malformado', async () => {
  const listener = vi.fn()
  window.addEventListener(SESSION_IDLE_DEADLINE_EVENT, listener)
  vi.stubGlobal(
    'fetch',
    vi.fn().mockResolvedValue(
      new Response(JSON.stringify({ items: [], next_cursor: null }), {
        status: 200,
        headers: {
          'Content-Type': 'application/json',
          'X-Lice-Session-Idle-Expires-At': 'not-a-date',
        },
      }),
    ),
  )

  await listAuditEvents({ period: '24h' })

  expect(listener).not.toHaveBeenCalled()
  window.removeEventListener(SESSION_IDLE_DEADLINE_EVENT, listener)
})
