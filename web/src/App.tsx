import { BrowserRouter as Router, Routes, Route, Navigate } from 'react-router-dom'
import { AuthProvider, useAuth } from './contexts/auth-context'
import { Toaster } from 'sonner'
import { AppLayout } from './components/layout/app-layout'
import './index.css'

// Import pages
import LoginPage from './pages/LoginPage'
import DashboardPage from './pages/DashboardPage'
import ServersPage from './pages/ServersPage'
import ServerDetailPage from './pages/ServerDetailPage'
import EnvironmentsPage from './pages/EnvironmentsPage'
import EnvironmentDetailPage from './pages/EnvironmentDetailPage'
import NewEnvironmentPage from './pages/NewEnvironmentPage'
import UsersPage from './pages/UsersPage'

// Protected route component
function ProtectedRoute({ children }: { children: React.ReactNode }) {
  const { isAuthenticated, isLoading } = useAuth()

  if (isLoading) {
    return (
      <div className="min-h-screen flex items-center justify-center bg-background">
        <div className="text-center">
          <div className="animate-spin rounded-full h-8 w-8 border-b-2 border-primary mx-auto"></div>
          <p className="text-sm text-muted-foreground mt-2">Loading…</p>
        </div>
      </div>
    )
  }

  if (!isAuthenticated) {
    return <Navigate to="/ui/login" replace />
  }

  return <AppLayout>{children}</AppLayout>
}

function AppRoutes() {
  return (
    <Routes>
      <Route path="/ui/login" element={<LoginPage />} />
      <Route path="/ui" element={<Navigate to="/ui/dashboard" replace />} />
      <Route path="/ui/dashboard" element={
        <ProtectedRoute>
          <DashboardPage />
        </ProtectedRoute>
      } />
      <Route path="/ui/servers" element={
        <ProtectedRoute>
          <ServersPage />
        </ProtectedRoute>
      } />
      <Route path="/ui/servers/:id" element={
        <ProtectedRoute>
          <ServerDetailPage />
        </ProtectedRoute>
      } />
      <Route path="/ui/environments" element={
        <ProtectedRoute>
          <EnvironmentsPage />
        </ProtectedRoute>
      } />
      <Route path="/ui/environments/new" element={
        <ProtectedRoute>
          <NewEnvironmentPage />
        </ProtectedRoute>
      } />
      <Route path="/ui/environments/:id" element={
        <ProtectedRoute>
          <EnvironmentDetailPage />
        </ProtectedRoute>
      } />
      <Route path="/ui/users" element={
        <ProtectedRoute>
          <UsersPage />
        </ProtectedRoute>
      } />
      <Route path="/" element={<Navigate to="/ui/dashboard" replace />} />
      <Route path="*" element={<Navigate to="/ui/dashboard" replace />} />
    </Routes>
  )
}

function App() {
  return (
    <AuthProvider>
      <Router>
        <AppRoutes />
        <Toaster position="top-right" />
      </Router>
    </AuthProvider>
  )
}

export default App
