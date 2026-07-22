import { Brand } from './Brand'

interface FullPageStatusProps {
  title: string
  description: string
}

export function FullPageStatus({ title, description }: FullPageStatusProps) {
  return (
    <main className="status-page" id="conteudo-principal">
      <div aria-live="polite" className="status-card" role="status">
        <Brand />
        <span aria-hidden="true" className="spinner" />
        <h1>{title}</h1>
        <p>{description}</p>
      </div>
    </main>
  )
}
