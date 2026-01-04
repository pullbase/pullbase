import { useEffect } from 'react'
import { useNavigate, useLocation } from 'react-router-dom'
import { useAuth } from '../contexts/auth-context'
import { Loader2 } from 'lucide-react'

export default function HomePage() {
  const { isAuthenticated, isLoading } = useAuth()
  const navigate = useNavigate()
  const location = useLocation()

  useEffect(() => {
    if (!isLoading && location.pathname === '/ui') {
      if (isAuthenticated) {
        navigate('/ui/servers')
      } else {
        navigate('/ui/login')
      }
    }
  }, [isAuthenticated, isLoading, navigate, location.pathname])

  if (location.pathname !== '/ui') {
    return null
  }

  return (
    <div className="min-h-screen flex items-center justify-center bg-background">
      <div className="flex flex-col items-center gap-4">
        <img
          src="/ui/logo-nav-dark.svg"
          alt="PullBase"
          className="h-12 w-auto"
        />
        <p className="text-sm text-muted-foreground">GitOps for Servers</p>
        <Loader2 className="h-8 w-8 animate-spin text-primary" />
        <p className="text-sm text-muted-foreground">Loading...</p>
      </div>
    </div>
  )
} 