import type {
  AuditActor,
  AuditEvent,
  AuditEventPage,
  AuditFilters,
  AuditResult,
  Principal,
  SessionInfo,
  SessionResponse,
} from './contracts'

export class ApiError extends Error {
  constructor(
    public readonly status: number,
    public readonly code: string | null,
    message = 'A requisição não pôde ser concluída.',
  ) {
    super(message)
    this.name = 'ApiError'
  }
}

export const SESSION_IDLE_DEADLINE_EVENT = 'lice:session-idle-deadline'
const SESSION_IDLE_DEADLINE_HEADER = 'X-Lice-Session-Idle-Expires-At'

const isRecord = (value: unknown): value is Record<string, unknown> =>
  typeof value === 'object' && value !== null && !Array.isArray(value)

function stringField(
  record: Record<string, unknown>,
  field: string,
  maximumLength = 200,
): string {
  const value = record[field]
  if (typeof value !== 'string' || value.length === 0 || value.length > maximumLength) {
    throw new ApiError(502, 'invalid_upstream_payload')
  }
  return value
}

function nullableStringField(
  record: Record<string, unknown>,
  field: string,
  maximumLength = 200,
): string | null {
  const value = record[field]
  if (value === null) return null
  if (typeof value !== 'string' || value.length > maximumLength) {
    throw new ApiError(502, 'invalid_upstream_payload')
  }
  return value
}

function parsePrincipal(value: unknown): Principal {
  if (!isRecord(value)) throw new ApiError(502, 'invalid_upstream_payload')
  return {
    id: stringField(value, 'id', 128),
    display_name: stringField(value, 'display_name', 160),
  }
}

function parseSessionInfo(value: unknown): SessionInfo {
  if (!isRecord(value)) throw new ApiError(502, 'invalid_upstream_payload')
  const session = {
    started_at: stringField(value, 'started_at', 40),
    idle_expires_at: stringField(value, 'idle_expires_at', 40),
    absolute_expires_at: stringField(value, 'absolute_expires_at', 40),
  }
  for (const timestamp of Object.values(session)) {
    if (!Number.isFinite(Date.parse(timestamp))) {
      throw new ApiError(502, 'invalid_upstream_payload')
    }
  }
  return session
}

function parseSession(value: unknown): SessionResponse {
  if (!isRecord(value)) throw new ApiError(502, 'invalid_upstream_payload')
  return {
    principal: parsePrincipal(value.principal),
    role: stringField(value, 'role', 80),
    session: parseSessionInfo(value.session),
    csrf_token: stringField(value, 'csrf_token', 512),
  }
}

function parseActor(value: unknown): AuditActor | null {
  if (value === null) return null
  if (!isRecord(value)) throw new ApiError(502, 'invalid_upstream_payload')
  return {
    id: stringField(value, 'id', 128),
    display_name: stringField(value, 'display_name', 160),
  }
}

function parseResult(value: unknown): AuditResult {
  if (value === 'success' || value === 'denied' || value === 'failure') return value
  throw new ApiError(502, 'invalid_upstream_payload')
}

function parseAuditEvent(value: unknown): AuditEvent {
  if (!isRecord(value)) throw new ApiError(502, 'invalid_upstream_payload')
  const occurredAt = stringField(value, 'occurred_at', 40)
  if (!Number.isFinite(Date.parse(occurredAt))) {
    throw new ApiError(502, 'invalid_upstream_payload')
  }
  return {
    id: stringField(value, 'id', 128),
    event_type: stringField(value, 'event_type', 120),
    occurred_at: occurredAt,
    actor: parseActor(value.actor),
    role: nullableStringField(value, 'role', 80),
    context: stringField(value, 'context', 160),
    result: parseResult(value.result),
    reason_code: nullableStringField(value, 'reason_code', 120),
    correlation_id: stringField(value, 'correlation_id', 128),
    source: stringField(value, 'source', 120),
  }
}

function parseAuditPage(value: unknown): AuditEventPage {
  if (!isRecord(value) || !Array.isArray(value.items)) {
    throw new ApiError(502, 'invalid_upstream_payload')
  }
  const nextCursor = value.next_cursor
  if (nextCursor !== null && typeof nextCursor !== 'string') {
    throw new ApiError(502, 'invalid_upstream_payload')
  }
  return {
    items: value.items.map(parseAuditEvent),
    next_cursor: nextCursor,
  }
}

async function errorFromResponse(response: Response): Promise<ApiError> {
  let code: string | null = null
  try {
    const body: unknown = await response.clone().json()
    if (isRecord(body) && typeof body.code === 'string' && body.code.length <= 120) {
      code = body.code
    }
  } catch {
    // Error bodies are deliberately ignored unless they match the small safe contract.
  }
  return new ApiError(response.status, code)
}

async function getJson(
  path: string,
  signal?: AbortSignal,
): Promise<unknown> {
  let response: Response
  try {
    response = await fetch(path, {
      method: 'GET',
      credentials: 'same-origin',
      cache: 'no-store',
      headers: { Accept: 'application/json' },
      signal,
    })
  } catch (error) {
    if (error instanceof DOMException && error.name === 'AbortError') throw error
    throw new ApiError(0, 'network_unavailable')
  }
  if (!response.ok) throw await errorFromResponse(response)
  const idleDeadline = response.headers.get(SESSION_IDLE_DEADLINE_HEADER)
  if (idleDeadline && Number.isFinite(Date.parse(idleDeadline))) {
    window.dispatchEvent(
      new CustomEvent(SESSION_IDLE_DEADLINE_EVENT, {
        detail: { idleExpiresAt: idleDeadline },
      }),
    )
  }
  try {
    return await response.json()
  } catch {
    throw new ApiError(502, 'invalid_upstream_payload')
  }
}

export async function getSession(signal?: AbortSignal): Promise<SessionResponse> {
  return parseSession(await getJson('/api/v1/session', signal))
}

export async function listAuditEvents(
  filters: AuditFilters,
  signal?: AbortSignal,
): Promise<AuditEventPage> {
  const query = new URLSearchParams({ period: filters.period })
  if (filters.event_type) query.set('event_type', filters.event_type)
  if (filters.result) query.set('result', filters.result)
  if (filters.query) query.set('q', filters.query.slice(0, 120))
  if (filters.cursor) query.set('cursor', filters.cursor)
  if (filters.limit) query.set('limit', String(filters.limit))
  return parseAuditPage(
    await getJson(`/api/v1/platform/audit-events?${query.toString()}`, signal),
  )
}

export async function getAuditEvent(
  eventId: string,
  signal?: AbortSignal,
): Promise<AuditEvent> {
  return parseAuditEvent(
    await getJson(
      `/api/v1/platform/audit-events/${encodeURIComponent(eventId)}`,
      signal,
    ),
  )
}
