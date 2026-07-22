import type { AuditResult } from '../api/contracts'

const RESULT_LABELS: Record<AuditResult, string> = {
  success: 'Concluído',
  denied: 'Negado',
  failure: 'Falhou',
}

export function AuditResultBadge({ result }: { result: AuditResult }) {
  return (
    <span className={`badge badge--${result}`}>
      <span aria-hidden="true" className="badge__dot" />
      {RESULT_LABELS[result]}
    </span>
  )
}
