import { NavLink, Navigate, Outlet, Route, Routes } from 'react-router-dom'
import { useAuth } from './auth/AuthContext'
import { HomePage } from './pages/HomePage'
import { LoginPage } from './pages/LoginPage'
import { MetricsPage } from './pages/MetricsPage'
import { ModelsPage } from './pages/ModelsPage'
import { SignupPage } from './pages/SignupPage'

function ProtectedRoute() {
  const { authenticated } = useAuth()
  return authenticated ? <Outlet /> : <Navigate to="/login" replace />
}

function AppLayout() {
  const { logout } = useAuth()
  return (
    <div className="app-shell">
      <header className="topbar">
        <NavLink className="brand" to="/">LLM Gateway</NavLink>
        <nav aria-label="Main navigation">
          <NavLink to="/" end>Home</NavLink>
          <NavLink to="/models">Models</NavLink>
          <NavLink to="/metrics">Metrics</NavLink>
          <button className="link-button" onClick={logout}>Logout</button>
        </nav>
      </header>
      <main className="page"><Outlet /></main>
    </div>
  )
}

export default function App() {
  return (
    <Routes>
      <Route path="/login" element={<LoginPage />} />
      <Route path="/signup" element={<SignupPage />} />
      <Route element={<ProtectedRoute />}>
        <Route element={<AppLayout />}>
          <Route index element={<HomePage />} />
          <Route path="/models" element={<ModelsPage />} />
          <Route path="/metrics" element={<MetricsPage />} />
        </Route>
      </Route>
      <Route path="*" element={<Navigate to="/" replace />} />
    </Routes>
  )
}
