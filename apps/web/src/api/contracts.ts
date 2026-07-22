export const PLATFORM_OPERATOR_ROLE = 'platform_operator' as const

export interface Principal {
  id: string
  display_name: string
}

export interface SessionInfo {
  started_at: string
  idle_expires_at: string
  absolute_expires_at: string
}

export interface SessionResponse {
  principal: Principal
  role: string
  session: SessionInfo
  csrf_token: string
}

export type AuditResult = 'success' | 'denied' | 'failure'

export interface AuditActor {
  id: string
  display_name: string
}

export interface AuditEvent {
  id: string
  event_type: string
  occurred_at: string
  actor: AuditActor | null
  role: string | null
  context: string
  result: AuditResult
  reason_code: string | null
  correlation_id: string
  source: string
}

export interface AuditEventPage {
  items: AuditEvent[]
  next_cursor: string | null
}

export interface AuditFilters {
  period: '24h' | '7d' | '30d'
  event_type?: string
  result?: AuditResult
  query?: string
  cursor?: string
  limit?: number
}
