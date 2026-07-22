import { Link, useParams } from 'react-router-dom'
import { Brand } from '../components/Brand'

type AccessState = {
  eyebrow: string
  title: string
  description: string
  action: 'login' | 'retry' | 'home'
  actionLabel: string
  tone: 'warning' | 'error' | 'neutral' | 'success'
}

const STATES: Record<string, AccessState> = {
  negado: {
    eyebrow: 'Acesso negado',
    title: 'Esta conta não pode acessar o controle global.',
    description:
      'Entre com uma conta autorizada para operar a plataforma. Se você esperava ter acesso, fale com a administração responsável.',
    action: 'login',
    actionLabel: 'Entrar com outra conta',
    tone: 'error',
  },
  'sessao-expirada': {
    eyebrow: 'Sessão expirada',
    title: 'Sua sessão terminou por segurança.',
    description:
      'Isso pode ocorrer após um período sem atividade ou ao atingir a duração máxima permitida. Entre novamente para continuar.',
    action: 'login',
    actionLabel: 'Entrar novamente',
    tone: 'warning',
  },
  'servico-indisponivel': {
    eyebrow: 'Indisponibilidade temporária',
    title: 'Não foi possível confirmar o acesso agora.',
    description:
      'O serviço pode estar passando por uma breve instabilidade. Aguarde um momento e tente outra vez.',
    action: 'retry',
    actionLabel: 'Tentar novamente',
    tone: 'error',
  },
  'fluxo-invalido': {
    eyebrow: 'Fluxo inválido',
    title: 'Não foi possível concluir esta entrada.',
    description:
      'A solicitação de autenticação não é mais válida. Inicie um novo acesso para continuar com segurança.',
    action: 'login',
    actionLabel: 'Iniciar novo acesso',
    tone: 'warning',
  },
  'sessao-encerrada': {
    eyebrow: 'Sessão encerrada',
    title: 'O acesso local não está ativo.',
    description:
      'Você pode fechar esta página ou entrar novamente. Se ainda houver uma sessão no provedor de identidade, ela será verificada no próximo acesso.',
    action: 'login',
    actionLabel: 'Entrar novamente',
    tone: 'success',
  },
}

function Action({ state }: { state: AccessState }) {
  if (state.action === 'retry') {
    return (
      <Link className="button button--primary" to="/controle">
        {state.actionLabel}
      </Link>
    )
  }
  if (state.action === 'login') {
    return (
      <a className="button button--primary" href="/api/v1/auth/login">
        {state.actionLabel}
      </a>
    )
  }
  return (
    <Link className="button button--primary" to="/entrar">
      {state.actionLabel}
    </Link>
  )
}

export function AccessPage() {
  const { reason = '' } = useParams()
  const state = STATES[reason] ?? {
    eyebrow: 'Página não encontrada',
    title: 'Este endereço não existe.',
    description: 'Volte à página de entrada para iniciar um acesso válido.',
    action: 'home' as const,
    actionLabel: 'Voltar à entrada',
    tone: 'neutral' as const,
  }

  return (
    <main className="access-page" id="conteudo-principal">
      <Brand />
      <section className="access-card" aria-labelledby="access-title">
        <span aria-hidden="true" className={`access-symbol access-symbol--${state.tone}`}>
          {state.tone === 'success' ? '✓' : state.tone === 'neutral' ? 'i' : '!'}
        </span>
        <p className="eyebrow">{state.eyebrow}</p>
        <h1 id="access-title">{state.title}</h1>
        <p>{state.description}</p>
        <Action state={state} />
      </section>
      <p className="access-page__help">
        Nenhum detalhe técnico ou dado pessoal é exibido nesta página.
      </p>
    </main>
  )
}
