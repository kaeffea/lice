import { Navigate, Route, Routes } from 'react-router-dom'
import { SessionGate } from './auth/session'
import { ControlShell } from './components/ControlShell'
import { RouteAnnouncer } from './components/RouteAnnouncer'
import { AccessPage } from './pages/AccessPage'
import { AuditDetailPage } from './pages/AuditDetailPage'
import { AuditPage } from './pages/AuditPage'
import { ControlHomePage } from './pages/ControlHomePage'
import { EntryPage } from './pages/EntryPage'

export function App() {
  return (
    <>
      <RouteAnnouncer />
      <Routes>
        <Route element={<Navigate replace to="/controle" />} path="/" />
        <Route element={<EntryPage />} path="/entrar" />
        <Route element={<AccessPage />} path="/acesso/:reason" />
        <Route
          element={
            <SessionGate>
              <ControlShell />
            </SessionGate>
          }
          path="/controle"
        >
          <Route element={<ControlHomePage />} index />
          <Route element={<AuditPage />} path="auditoria" />
          <Route element={<AuditDetailPage />} path="auditoria/:eventId" />
        </Route>
        <Route element={<Navigate replace to="/acesso/nao-encontrado" />} path="*" />
      </Routes>
    </>
  )
}
