import { Link } from 'react-router-dom'

interface BrandProps {
  compact?: boolean
  linked?: boolean
}

export function Brand({ compact = false, linked = false }: BrandProps) {
  const content = (
    <>
      <span aria-hidden="true" className="brand__mark">
        <span />
        <span />
        <span />
      </span>
      <span className="brand__copy">
        <strong>LICE</strong>
        {!compact && <small>Lócus Integrado de Campus e Ensino</small>}
      </span>
    </>
  )
  if (linked) {
    return (
      <Link aria-label="LICE — visão geral" className="brand" to="/controle">
        {content}
      </Link>
    )
  }
  return <div className="brand">{content}</div>
}
