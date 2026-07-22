import { type FormEvent, useEffect, useMemo, useState } from 'react'
import { Link, useNavigate, useSearchParams } from 'react-router-dom'
import { listAuditEvents } from '../api/client'
import type { AuditEventPage, AuditFilters, AuditResult } from '../api/contracts'
import { redirectForProtectedRequest } from '../auth/session'
import { AuditResultBadge } from '../components/AuditResultBadge'
import { auditEventLabel, formatDateTime } from '../domain/audit'

const EMPTY_PAGE: AuditEventPage = { items: [], next_cursor: null }
const VALID_PERIODS = new Set(['24h', '7d', '30d'])
const VALID_RESULTS = new Set<AuditResult>(['success', 'denied', 'failure'])

function filtersFrom(params: URLSearchParams): AuditFilters {
  const period = params.get('period') ?? '7d'
  const result = params.get('result')
  return {
    period: VALID_PERIODS.has(period) ? (period as AuditFilters['period']) : '7d',
    event_type: params.get('event_type') || undefined,
    result: result && VALID_RESULTS.has(result as AuditResult)
      ? (result as AuditResult)
      : undefined,
    query: params.get('q') || undefined,
    cursor: params.get('cursor') || undefined,
    limit: 25,
  }
}

export function AuditPage() {
  const [params, setParams] = useSearchParams()
  const navigate = useNavigate()
  const filters = useMemo(() => filtersFrom(params), [params])
  const [query, setQuery] = useState(filters.query ?? '')
  const [page, setPage] = useState<AuditEventPage>(EMPTY_PAGE)
  const [status, setStatus] = useState<'loading' | 'ready' | 'error'>('loading')
  const [retry, setRetry] = useState(0)

  useEffect(() => setQuery(filters.query ?? ''), [filters.query])

  useEffect(() => {
    const controller = new AbortController()
    setStatus('loading')
    listAuditEvents(filters, controller.signal)
      .then((response) => {
        setPage(response)
        setStatus('ready')
      })
      .catch((error: unknown) => {
        if (error instanceof DOMException && error.name === 'AbortError') return
        const destination = redirectForProtectedRequest(error)
        if (destination) {
          navigate(destination, { replace: true })
          return
        }
        setPage(EMPTY_PAGE)
        setStatus('error')
      })
    return () => controller.abort()
  }, [filters, navigate, retry])

  function updateFilter(name: string, value: string) {
    const next = new URLSearchParams(params)
    if (value) next.set(name, value)
    else next.delete(name)
    if (name !== 'cursor') next.delete('cursor')
    setParams(next)
  }

  function search(event: FormEvent<HTMLFormElement>) {
    event.preventDefault()
    updateFilter('q', query.trim().slice(0, 120))
  }

  function clearFilters() {
    setQuery('')
    setParams(new URLSearchParams({ period: '7d' }))
  }

  return (
    <main aria-busy={status === 'loading'} className="page" id="conteudo-principal">
      <header className="page-header">
        <div>
          <p className="eyebrow">Segurança e conformidade</p>
          <h1>Auditoria</h1>
          <p>Consulte eventos sanitizados da operação global da plataforma.</p>
        </div>
      </header>

      <section aria-labelledby="filters-title" className="filter-panel">
        <div className="section-heading section-heading--compact">
          <h2 id="filters-title">Filtrar eventos</h2>
          <button className="text-button" onClick={clearFilters} type="button">
            Limpar filtros
          </button>
        </div>
        <form className="filters" onSubmit={search}>
          <label>
            <span>Período</span>
            <select
              onChange={(event) => updateFilter('period', event.target.value)}
              value={filters.period}
            >
              <option value="24h">Últimas 24 horas</option>
              <option value="7d">Últimos 7 dias</option>
              <option value="30d">Últimos 30 dias</option>
            </select>
          </label>
          <label>
            <span>Tipo de evento</span>
            <select
              onChange={(event) => updateFilter('event_type', event.target.value)}
              value={filters.event_type ?? ''}
            >
              <option value="">Todos os tipos</option>
              <option value="security.session_started">Sessão iniciada</option>
              <option value="security.login_rejected">Entrada rejeitada</option>
              <option value="security.access_denied">Acesso negado</option>
              <option value="security.session_expired">Sessão expirada</option>
              <option value="security.session_ended">Sessão encerrada</option>
            </select>
          </label>
          <label>
            <span>Resultado</span>
            <select
              onChange={(event) => updateFilter('result', event.target.value)}
              value={filters.result ?? ''}
            >
              <option value="">Todos os resultados</option>
              <option value="success">Concluído</option>
              <option value="denied">Negado</option>
              <option value="failure">Falhou</option>
            </select>
          </label>
          <label className="search-field">
            <span>Buscar</span>
            <span className="search-field__control">
              <input
                maxLength={120}
                onChange={(event) => setQuery(event.target.value)}
                placeholder="Ator, correlação ou contexto"
                type="search"
                value={query}
              />
              <button className="button button--secondary button--compact" type="submit">
                Buscar
              </button>
            </span>
          </label>
        </form>
      </section>

      <section aria-labelledby="events-title" className="table-card">
        <div className="table-card__header">
          <div>
            <p className="eyebrow">Registro cronológico</p>
            <h2 id="events-title">Eventos encontrados</h2>
          </div>
          {status === 'ready' && (
            <span className="record-count">
              {page.items.length} {page.items.length === 1 ? 'registro' : 'registros'} nesta página
            </span>
          )}
        </div>

        {status === 'loading' && (
          <div aria-live="polite" className="inline-state" role="status">
            <span aria-hidden="true" className="spinner spinner--small" />
            <p>
              <strong>Carregando eventos</strong>
              <small>Aguarde enquanto consultamos os registros.</small>
            </p>
          </div>
        )}

        {status === 'error' && (
          <div className="inline-state inline-state--error" role="alert">
            <span aria-hidden="true" className="inline-state__symbol">!</span>
            <p>
              <strong>Não foi possível carregar a auditoria.</strong>
              <small>Nenhum detalhe técnico foi exibido. Tente novamente em instantes.</small>
            </p>
            <button className="button button--secondary button--compact" onClick={() => setRetry((value) => value + 1)} type="button">
              Tentar novamente
            </button>
          </div>
        )}

        {status === 'ready' && page.items.length === 0 && (
          <div className="inline-state inline-state--empty">
            <span aria-hidden="true" className="inline-state__symbol">○</span>
            <p>
              <strong>Nenhum evento encontrado.</strong>
              <small>Ajuste os filtros ou consulte outro período.</small>
            </p>
          </div>
        )}

        {status === 'ready' && page.items.length > 0 && (
          <div className="table-scroll">
            <table>
              <thead>
                <tr>
                  <th scope="col">Data e hora</th>
                  <th scope="col">Evento</th>
                  <th scope="col">Ator</th>
                  <th scope="col">Contexto</th>
                  <th scope="col">Resultado</th>
                  <th scope="col"><span className="sr-only">Ação</span></th>
                </tr>
              </thead>
              <tbody>
                {page.items.map((event) => (
                  <tr key={event.id}>
                    <td data-label="Data e hora">{formatDateTime(event.occurred_at)}</td>
                    <td data-label="Evento"><strong>{auditEventLabel(event.event_type)}</strong></td>
                    <td data-label="Ator">{event.actor?.display_name ?? 'Sistema'}</td>
                    <td data-label="Contexto">{event.context}</td>
                    <td data-label="Resultado"><AuditResultBadge result={event.result} /></td>
                    <td className="table-action">
                      <Link aria-label={`Ver detalhes de ${auditEventLabel(event.event_type)}`} to={`/controle/auditoria/${encodeURIComponent(event.id)}`}>
                        Ver detalhes
                      </Link>
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        )}

        {status === 'ready' && page.next_cursor && (
          <div className="table-card__footer">
            <button
              className="button button--secondary"
              onClick={() => updateFilter('cursor', page.next_cursor ?? '')}
              type="button"
            >
              Próxima página
            </button>
          </div>
        )}
      </section>
    </main>
  )
}
