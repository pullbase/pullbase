import { useEffect, useState, useRef, type ReactNode } from 'react'
import { Link as RouterLink, useLocation } from 'react-router-dom'
import { useAuth } from '../../contexts/auth-context'
import { Button } from '../ui/button'
import { Avatar, AvatarFallback } from '../ui/avatar'
import { Badge } from '../ui/badge'
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from '../ui/dropdown-menu'
import {
  Server,
  Settings,
  GitBranch,
  LogOut,
  User,
  Users,
  BarChart3,
  Loader2,
  Sun,
  Moon,
} from 'lucide-react'
import { cn } from '../../lib/utils'
import { toast } from 'sonner'

interface AppLayoutProps {
  children: ReactNode
}

const navigation = [
  {
    name: 'Dashboard',
    href: '/ui/dashboard',
    icon: BarChart3,
  },
  {
    name: 'Environments',
    href: '/ui/environments',
    icon: GitBranch,
  },
  {
    name: 'Servers',
    href: '/ui/servers',
    icon: Server,
  },
]

export function AppLayout({ children }: AppLayoutProps) {
  const { user, logout } = useAuth()
  const location = useLocation()
  const role = (user?.role || 'user').toLowerCase()
  const roleLabel = role.charAt(0).toUpperCase() + role.slice(1)
  const isAdmin = role === 'admin'
  const navItems = isAdmin
    ? [...navigation, { name: 'Users', href: '/ui/users', icon: Users }]
    : navigation
  const [loadingCount, setLoadingCount] = useState(0)
  const loadingToastRef = useRef<string | number | null>(null)
  const [theme, setTheme] = useState<'light' | 'dark'>(() => {
    if (typeof window === 'undefined') {
      return 'light'
    }
    const stored = localStorage.getItem('pullbase-theme')
    if (stored === 'dark' || stored === 'light') {
      return stored
    }
    return window.matchMedia('(prefers-color-scheme: dark)').matches ? 'dark' : 'light'
  })

  useEffect(() => {
    const root = document.documentElement
    if (theme === 'dark') {
      root.classList.add('dark')
    } else {
      root.classList.remove('dark')
    }
    localStorage.setItem('pullbase-theme', theme)
  }, [theme])

  useEffect(() => {
    const handleLoading = (event: Event) => {
      const customEvent = event as CustomEvent<{ count: number }>
      setLoadingCount(customEvent.detail?.count ?? 0)
    }

    window.addEventListener('pullbase:loading', handleLoading as EventListener)
    return () => {
      window.removeEventListener('pullbase:loading', handleLoading as EventListener)
    }
  }, [])

  useEffect(() => {
    let timer: ReturnType<typeof setTimeout> | null = null
    if (loadingCount > 0) {
      timer = setTimeout(() => {
        if (loadingCount > 0 && !loadingToastRef.current) {
          loadingToastRef.current = toast.loading('Syncing data…')
        }
      }, 1200)
    } else if (loadingToastRef.current) {
      toast.dismiss(loadingToastRef.current)
      loadingToastRef.current = null
    }

    return () => {
      if (timer) {
        clearTimeout(timer)
      }
    }
  }, [loadingCount])

  useEffect(() => {
    return () => {
      if (loadingToastRef.current) {
        toast.dismiss(loadingToastRef.current)
        loadingToastRef.current = null
      }
    }
  }, [])

  const toggleTheme = () => {
    setTheme((prev) => (prev === 'dark' ? 'light' : 'dark'))
  }

  const logoSrc = theme === 'dark' ? '/ui/logo-nav-white.svg' : '/ui/logo-nav-dark.svg'

  return (
    <div className="min-h-screen bg-background">
      {/* Navigation */}
      <nav className="bg-card shadow-sm border-b border-border">
        <div className="max-w-7xl mx-auto px-4 sm:px-6 lg:px-8">
          <div className="flex justify-between h-16">
            <div className="flex">
              {/* Logo */}
              <div className="flex-shrink-0 flex items-center">
                <img src={logoSrc} alt="PullBase" className="h-8 w-auto" />
              </div>

              {/* Navigation links */}
              <div className="hidden sm:ml-6 sm:flex sm:space-x-8">
                {navItems.map((item) => {
                  const Icon = item.icon
                  const isActive = location.pathname.startsWith(item.href)

                  return (
                    <RouterLink
                      key={item.name}
                      to={item.href}
                      className={cn(
                        'inline-flex items-center px-1 pt-1 border-b-2 text-sm font-medium',
                        isActive
                          ? 'border-primary text-primary'
                          : 'border-transparent text-muted-foreground hover:border-border hover:text-foreground'
                      )}
                    >
                      <Icon className="w-4 h-4 mr-2" />
                      {item.name}
                    </RouterLink>
                  )
                })}
              </div>
            </div>

            {/* User menu */}
            <div className="flex items-center gap-4">
              <Button
                variant="ghost"
                size="icon"
                onClick={toggleTheme}
                className="h-8 w-8"
              >
                {theme === 'dark' ? (
                  <Sun className="h-4 w-4" />
                ) : (
                  <Moon className="h-4 w-4" />
                )}
                <span className="sr-only">Toggle theme</span>
              </Button>

              {loadingCount > 0 && (
                <div className="flex items-center gap-2 text-xs text-muted-foreground">
                  <Loader2 className="w-4 h-4 animate-spin text-primary" />
                  <span>Syncing…</span>
                </div>
              )}

              <Badge variant="secondary" className="uppercase tracking-wide text-xs px-2 py-1">
                {roleLabel}
              </Badge>

              <DropdownMenu>
                <DropdownMenuTrigger asChild>
                  <Button variant="ghost" className="relative h-8 w-8 rounded-full">
                    <Avatar className="h-8 w-8">
                      <AvatarFallback className="bg-primary text-primary-foreground">
                        {user?.username?.charAt(0).toUpperCase() || 'U'}
                      </AvatarFallback>
                    </Avatar>
                  </Button>
                </DropdownMenuTrigger>
                <DropdownMenuContent className="w-56" align="end" forceMount>
                  <DropdownMenuItem>
                    <User className="mr-2 h-4 w-4" />
                    <span>{user?.username}</span>
                  </DropdownMenuItem>
                  <DropdownMenuItem>
                    <Settings className="mr-2 h-4 w-4" />
                    <span className="text-sm text-muted-foreground">Role: {roleLabel}</span>
                  </DropdownMenuItem>
                  <DropdownMenuItem onClick={logout}>
                    <LogOut className="mr-2 h-4 w-4" />
                    <span>Log out</span>
                  </DropdownMenuItem>
                </DropdownMenuContent>
              </DropdownMenu>
            </div>
          </div>
        </div>

        {/* Mobile menu */}
        <div className="sm:hidden">
          <div className="pt-2 pb-3 space-y-1">
            {navItems.map((item) => {
              const Icon = item.icon
              const isActive = location.pathname.startsWith(item.href)

              return (
                <RouterLink
                  key={item.name}
                  to={item.href}
                  className={cn(
                    'block pl-3 pr-4 py-2 border-l-4 text-base font-medium',
                    isActive
                      ? 'bg-accent border-primary text-primary'
                      : 'border-transparent text-muted-foreground hover:bg-accent hover:border-border hover:text-foreground'
                  )}
                >
                  <Icon className="w-4 h-4 inline mr-2" />
                  {item.name}
                </RouterLink>
              )
            })}
          </div>
        </div>
      </nav>

      {/* Main content */}
      <main className="py-8">
        <div className="max-w-7xl mx-auto px-4 sm:px-6 lg:px-8">
          {children}
        </div>
      </main>
    </div>
  )
}
