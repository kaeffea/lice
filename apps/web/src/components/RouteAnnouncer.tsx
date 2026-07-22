import { useEffect, useState } from 'react'
import { useLocation } from 'react-router-dom'

function pageName(pathname: string): string {
  if (pathname === '/entrar') return 'Entrar'
  if (pathname === '/controle') return 'Visão geral'
  if (pathname === '/controle/auditoria') return 'Auditoria'
  if (pathname.startsWith('/controle/auditoria/')) return 'Detalhes do evento'
  if (pathname.startsWith('/acesso/')) return 'Informação de acesso'
  return 'Página não encontrada'
}

export function RouteAnnouncer() {
  const { pathname } = useLocation()
  const [announcement, setAnnouncement] = useState('')

  useEffect(() => {
    const name = pageName(pathname)
    document.title = `${name} | LICE`
    setAnnouncement(`Página carregada: ${name}`)
  }, [pathname])

  return (
    <div aria-atomic="true" aria-live="polite" className="sr-only">
      {announcement}
    </div>
  )
}
