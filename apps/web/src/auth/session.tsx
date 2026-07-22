import {
  createContext,
  type ReactNode,
  useContext,
  useEffect,
  useState,
} from 'react'
import { Navigate, useLocation, useNavigate } from 'react-router-dom'
import {
  ApiError,
  getSession,
  SESSION_IDLE_DEADLINE_EVENT,
} from '../api/client'
import {
  PLATFORM_OPERATOR_ROLE,
  type SessionResponse,
} from '../api/contracts'
import { FullPageStatus } from '../components/FullPageStatus'

const SessionContext = createContext<SessionResponse | null>(null)

const EXPIRED_SESSION_CODES = new Set([
  'session_expired',
  'session_idle_timeout',
  'session_absolute_timeout',
])

function destinationFor(error: unknown): string {
  if (!(error instanceof ApiError)) return '/acesso/servico-indisponivel'
  if (error.status === 403) return '/acesso/negado'
  if (error.status === 401) {
    return error.code && EXPIRED_SESSION_CODES.has(error.code)
      ? '/acesso/sessao-expirada'
      : '/entrar'
  }
  if (error.status === 400 && error.code === 'invalid_auth_flow') {
    return '/acesso/fluxo-invalido'
  }
  return '/acesso/servico-indisponivel'
}

interface SessionGateProps {
  children: ReactNode
}

export function SessionGate({ children }: SessionGateProps) {
  const [session, setSession] = useState<SessionResponse | null>(null)
  const [redirectTo, setRedirectTo] = useState<string | null>(null)
  const location = useLocation()
  const navigate = useNavigate()

  useEffect(() => {
    let controller: AbortController | null = null
    let active = true
    const validate = (hideProtectedContent: boolean) => {
      controller?.abort()
      controller = new AbortController()
      if (hideProtectedContent) {
        setSession(null)
        setRedirectTo(null)
      }
      getSession(controller.signal)
        .then((response) => {
          if (!active) return
          if (response.role !== PLATFORM_OPERATOR_ROLE) {
            setRedirectTo('/acesso/negado')
            return
          }
          setRedirectTo(null)
          setSession(response)
        })
        .catch((error: unknown) => {
          if (!active || (error instanceof DOMException && error.name === 'AbortError')) {
            return
          }
          setRedirectTo(destinationFor(error))
        })
    }
    validate(false)
    const revalidateVisibleSession = () => {
      if (document.visibilityState === 'visible') validate(true)
    }
    const revalidateRestoredPage = (event: PageTransitionEvent) => {
      if (event.persisted) validate(true)
    }
    document.addEventListener('visibilitychange', revalidateVisibleSession)
    window.addEventListener('focus', revalidateVisibleSession)
    window.addEventListener('pageshow', revalidateRestoredPage)
    return () => {
      active = false
      controller?.abort()
      document.removeEventListener('visibilitychange', revalidateVisibleSession)
      window.removeEventListener('focus', revalidateVisibleSession)
      window.removeEventListener('pageshow', revalidateRestoredPage)
    }
  }, [])

  useEffect(() => {
    const synchronizeIdleDeadline = (event: Event) => {
      if (!(event instanceof CustomEvent)) return
      const candidate = event.detail?.idleExpiresAt
      if (typeof candidate !== 'string') return
      const candidateTime = Date.parse(candidate)
      if (!Number.isFinite(candidateTime)) return
      setSession((current) => {
        if (!current) return current
        const currentTime = Date.parse(current.session.idle_expires_at)
        const absoluteTime = Date.parse(current.session.absolute_expires_at)
        if (candidateTime <= currentTime || candidateTime > absoluteTime) {
          return current
        }
        return {
          ...current,
          session: { ...current.session, idle_expires_at: candidate },
        }
      })
    }
    window.addEventListener(SESSION_IDLE_DEADLINE_EVENT, synchronizeIdleDeadline)
    return () => {
      window.removeEventListener(SESSION_IDLE_DEADLINE_EVENT, synchronizeIdleDeadline)
    }
  }, [])

  useEffect(() => {
    if (!session) return
    const expiresAt = Math.min(
      Date.parse(session.session.idle_expires_at),
      Date.parse(session.session.absolute_expires_at),
    )
    const remaining = expiresAt - Date.now()
    if (remaining <= 0) {
      navigate('/acesso/sessao-expirada', { replace: true })
      return
    }
    const timer = window.setTimeout(
      () => navigate('/acesso/sessao-expirada', { replace: true }),
      Math.min(remaining, 2_147_483_647),
    )
    return () => window.clearTimeout(timer)
  }, [navigate, session])

  if (redirectTo) {
    return (
      <Navigate
        replace
        state={{ from: location.pathname }}
        to={redirectTo}
      />
    )
  }
  if (!session) {
    return (
      <FullPageStatus
        description="Estamos confirmando sua sessão com segurança."
        title="Validando acesso"
      />
    )
  }
  return (
    <SessionContext.Provider value={session}>
      {children}
    </SessionContext.Provider>
  )
}

export function useSession(): SessionResponse {
  const session = useContext(SessionContext)
  if (!session) throw new Error('useSession deve ser usado após SessionGate')
  return session
}

export function redirectForProtectedRequest(error: unknown): string | null {
  if (!(error instanceof ApiError)) return null
  if (error.status === 403) return '/acesso/negado'
  if (error.status === 401) return '/acesso/sessao-expirada'
  return null
}
