const EVENT_LABELS: Record<string, string> = {
  'security.session_started': 'Sessão iniciada',
  'security.login_rejected': 'Entrada rejeitada',
  'security.access_denied': 'Acesso negado',
  'security.session_expired': 'Sessão expirada',
  'security.session_ended': 'Sessão encerrada',
}

const REASON_LABELS: Record<string, string> = {
  invalid_credentials: 'Credenciais inválidas',
  account_disabled: 'Conta desativada',
  insufficient_role: 'Perfil sem permissão',
  idle_timeout: 'Tempo de inatividade excedido',
  absolute_timeout: 'Duração máxima da sessão excedida',
}

export function auditEventLabel(eventType: string): string {
  return EVENT_LABELS[eventType] ?? eventType
}

export function auditReasonLabel(reasonCode: string | null): string {
  if (!reasonCode) return 'Não informado'
  return REASON_LABELS[reasonCode] ?? reasonCode
}

export function formatDateTime(value: string): string {
  return new Intl.DateTimeFormat('pt-BR', {
    dateStyle: 'short',
    timeStyle: 'medium',
    timeZone: 'America/Maceio',
  }).format(new Date(value))
}

export function formatRole(role: string | null): string {
  if (role === 'platform_operator') return 'Operação da plataforma'
  return role ?? 'Sistema'
}
