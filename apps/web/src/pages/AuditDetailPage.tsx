import { useEffect, useState } from 'react'
import { Link, useNavigate, useParams } from 'react-router-dom'
import { getAuditEvent } from '../api/client'
import type { AuditEvent } from '../api/contracts'
import { redirectForProtectedRequest } from '../auth/session'
import { AuditResultBadge } from '../components/AuditResultBadge'
import {
  auditEventLabel,
  auditReasonLabel,
  formatDateTime,
  formatRole,
} from '../domain/audit'

export function AuditDetailPage() {
  const { eventId = '' } = useParams()
  const navigate = useNavigate()
  const [event, setEvent] = useState<AuditEvent | null>(null)
  const [status, setStatus] = useState<'loading' | 'ready' | 'error'>('loading')
  const [retry, setRetry] = useState(0)

  useEffect(() => {
    if (!eventId) {
      setStatus('error')
      return
    }
    const controller = new AbortController()
    setStatus('loading')
    setEvent(null)
    getAuditEvent(eventId, controller.signal)
      .then((response) => {
        setEvent(response)
        setStatus('ready')
      })
      .catch((error: unknown) => {
        if (error instanceof DOMException && error.name === 'AbortError') return
        const destination = redirectForProtectedRequest(error)
        if (destination) {
          navigate(destination, { replace: true })
          return
        }
        setStatus('error')
      })
    return () => controller.abort()
  }, [eventId, navigate, retry])

  return (
    <main aria-busy={status === 'loading'} className="page" id="conteudo-principal">
      <Link className="back-link" to="/controle/auditoria">← Voltar para auditoria</Link>

      {status === 'loading' && (
        <div aria-live="polite" className="detail-card inline-state" role="status">
          <span aria-hidden="true" className="spinner spinner--small" />
          <p>
            <strong>Carregando detalhes</strong>
            <small>Aguarde enquanto consultamos o evento.</small>
          </p>
        </div>
      )}

      {status === 'error' && (
        <div className="detail-card inline-state inline-state--error" role="alert">
          <span aria-hidden="true" className="inline-state__symbol">!</span>
          <p>
            <strong>Não foi possível abrir este evento.</strong>
            <small>O registro pode não existir ou estar temporariamente indisponível.</small>
          </p>
          <button className="button button--secondary button--compact" onClick={() => setRetry((value) => value + 1)} type="button">
            Tentar novamente
          </button>
        </div>
      )}

      {status === 'ready' && event && (
        <>
          <header className="page-header detail-heading">
            <div>
              <p className="eyebrow">Evento de auditoria</p>
              <h1>{auditEventLabel(event.event_type)}</h1>
              <p>Registro sanitizado para análise operacional.</p>
            </div>
            <AuditResultBadge result={event.result} />
          </header>

          <section aria-labelledby="event-data-title" className="detail-card">
            <div className="section-heading section-heading--compact">
              <h2 id="event-data-title">Dados do evento</h2>
            </div>
            <dl className="detail-list">
              <div>
                <dt>Data e hora</dt>
                <dd>{formatDateTime(event.occurred_at)}</dd>
              </div>
              <div>
                <dt>Ator</dt>
                <dd>{event.actor?.display_name ?? 'Sistema'}</dd>
              </div>
              <div>
                <dt>Perfil</dt>
                <dd>{formatRole(event.role)}</dd>
              </div>
              <div>
                <dt>Contexto</dt>
                <dd>{event.context}</dd>
              </div>
              <div>
                <dt>Motivo</dt>
                <dd>{auditReasonLabel(event.reason_code)}</dd>
              </div>
              <div>
                <dt>Origem</dt>
                <dd>{event.source}</dd>
              </div>
              <div className="detail-list__wide">
                <dt>ID de correlação</dt>
                <dd><code>{event.correlation_id}</code></dd>
              </div>
              <div className="detail-list__wide">
                <dt>ID do evento</dt>
                <dd><code>{event.id}</code></dd>
              </div>
            </dl>
          </section>
          <p className="privacy-note">
            Este painel não exibe credenciais, tokens, endereços completos de rede ou conteúdo sensível.
          </p>
        </>
      )}
    </main>
  )
}
