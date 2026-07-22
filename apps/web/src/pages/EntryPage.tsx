import { Brand } from '../components/Brand'

export function EntryPage() {
  return (
    <main className="entry-page" id="conteudo-principal">
      <section className="entry-intro" aria-labelledby="entry-title">
        <Brand />
        <div className="entry-intro__body">
          <p className="eyebrow">Administração institucional</p>
          <h1 id="entry-title">Decisões confiáveis começam com acesso seguro.</h1>
          <p>
            Entre no ambiente de controle da plataforma para acompanhar eventos
            operacionais com rastreabilidade.
          </p>
        </div>
        <p className="entry-intro__footnote">
          Acesso restrito a pessoas autorizadas.
        </p>
      </section>
      <section className="entry-panel" aria-labelledby="login-title">
        <div className="entry-card">
          <p className="eyebrow">Área protegida</p>
          <h2 id="login-title">Acessar o controle global</h2>
          <p>
            A autenticação é realizada pelo provedor de identidade do LICE. Suas
            credenciais não são processadas nesta página.
          </p>
          <a className="button button--primary button--wide" href="/api/v1/auth/login">
            Entrar com minha conta
          </a>
          <div className="security-note">
            <span aria-hidden="true">●</span>
            <p>
              <strong>Sessão protegida</strong>
              <small>O acesso e o encerramento da sessão são auditados.</small>
            </p>
          </div>
        </div>
      </section>
    </main>
  )
}
