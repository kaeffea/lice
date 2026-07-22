import { NavLink, Outlet } from 'react-router-dom'
import { useSession } from '../auth/session'
import { Brand } from './Brand'

function navClassName({ isActive }: { isActive: boolean }) {
  return isActive ? 'control-nav__link control-nav__link--active' : 'control-nav__link'
}

export function ControlShell() {
  const { csrf_token: csrfToken, principal } = useSession()

  return (
    <div className="control-layout">
      <a className="skip-link" href="#conteudo-principal">
        Ir para o conteúdo principal
      </a>
      <header className="control-header">
        <Brand compact linked />
        <div className="control-header__account">
          <span className="account-copy">
            <strong>{principal.display_name}</strong>
            <small>Operação da plataforma</small>
          </span>
          <form action="/api/v1/auth/logout" method="post">
            <input name="csrf_token" type="hidden" value={csrfToken} />
            <button className="button button--quiet button--compact" type="submit">
              Sair
            </button>
          </form>
        </div>
      </header>
      <aside aria-label="Navegação do controle" className="control-sidebar">
        <p className="control-sidebar__eyebrow">Controle global</p>
        <nav className="control-nav">
          <NavLink className={navClassName} end to="/controle">
            <span aria-hidden="true" className="nav-icon">⌂</span>
            Visão geral
          </NavLink>
          <NavLink className={navClassName} to="/controle/auditoria">
            <span aria-hidden="true" className="nav-icon">≡</span>
            Auditoria
          </NavLink>
        </nav>
        <p className="control-sidebar__note">
          Ações administrativas são registradas para rastreabilidade.
        </p>
      </aside>
      <div className="control-main">
        <Outlet />
      </div>
    </div>
  )
}
