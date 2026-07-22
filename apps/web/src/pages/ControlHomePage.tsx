import { Link } from 'react-router-dom'
import { useSession } from '../auth/session'
import { formatDateTime } from '../domain/audit'

export function ControlHomePage() {
  const { principal, session } = useSession()

  return (
    <main className="page" id="conteudo-principal">
      <header className="page-header page-header--welcome">
        <div>
          <p className="eyebrow">Controle global</p>
          <h1>Olá, {principal.display_name}.</h1>
          <p>
            Este é o ponto de partida para operações que abrangem toda a plataforma.
          </p>
        </div>
        <div className="session-summary">
          <span aria-hidden="true" className="session-summary__indicator" />
          <p>
            <strong>Sessão ativa</strong>
            <small>Iniciada em {formatDateTime(session.started_at)}</small>
          </p>
        </div>
      </header>

      <section aria-labelledby="available-title" className="section-block">
        <div className="section-heading">
          <div>
            <p className="eyebrow">Disponível agora</p>
            <h2 id="available-title">Operações da plataforma</h2>
          </div>
        </div>
        <div className="feature-grid feature-grid--single">
          <Link className="feature-card" to="/controle/auditoria">
            <span aria-hidden="true" className="feature-card__icon">≡</span>
            <span>
              <strong>Auditoria de segurança</strong>
              <small>
                Consulte entradas, recusas, acessos negados e encerramentos de sessão.
              </small>
            </span>
            <span aria-hidden="true" className="feature-card__arrow">→</span>
          </Link>
        </div>
      </section>

      <section aria-labelledby="protections-title" className="section-block">
        <div className="section-heading">
          <div>
            <p className="eyebrow">Proteções da sessão</p>
            <h2 id="protections-title">Controles em vigor</h2>
          </div>
        </div>
        <ul className="assurance-list">
          <li>
            <span aria-hidden="true">✓</span>
            <p>
              <strong>Perfil verificado</strong>
              <small>O conteúdo só é exibido após a autorização do operador.</small>
            </p>
          </li>
          <li>
            <span aria-hidden="true">✓</span>
            <p>
              <strong>Sessão com prazo</strong>
              <small>
                Validade atual até {formatDateTime(session.absolute_expires_at)}.
              </small>
            </p>
          </li>
          <li>
            <span aria-hidden="true">✓</span>
            <p>
              <strong>Saída protegida</strong>
              <small>O encerramento usa uma solicitação autenticada e auditável.</small>
            </p>
          </li>
        </ul>
      </section>
    </main>
  )
}
