import { useEffect, useState } from 'react'
import { useNavigate } from 'react-router-dom'
import { useAuth } from '../contexts/auth-context'
import { Button } from '../components/ui/button'
import { Input } from '../components/ui/input'
import { Label } from '../components/ui/label'
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '../components/ui/card'
import { Alert, AlertDescription, AlertTitle } from '../components/ui/alert'
import { Loader2, Lock, Copy, Info } from 'lucide-react'
import { authApi, type BootstrapStatus } from '../lib/api'
import { toast } from 'sonner'

export default function LoginPage() {
  const [username, setUsername] = useState('')
  const [password, setPassword] = useState('')
  const [isLoading, setIsLoading] = useState(false)
  const [bootstrapStatus, setBootstrapStatus] = useState<BootstrapStatus | null>(null)
  const [isBootstrapStatusLoading, setIsBootstrapStatusLoading] = useState(true)
  const [errorMessage, setErrorMessage] = useState<string | null>(null)
  const [isDarkMode, setIsDarkMode] = useState(() =>
    typeof document !== 'undefined' ? document.documentElement.classList.contains('dark') : false
  )
  const { login } = useAuth()
  const navigate = useNavigate()

  useEffect(() => {
    let mounted = true
    let observer: MutationObserver | null = null

    if (typeof window !== 'undefined') {
      const root = document.documentElement
      const updateTheme = () => setIsDarkMode(root.classList.contains('dark'))
      updateTheme()
      observer = new MutationObserver(updateTheme)
      observer.observe(root, { attributes: true, attributeFilter: ['class'] })
    }

    const fetchStatus = async () => {
      try {
        setIsBootstrapStatusLoading(true)
        const status = await authApi.getBootstrapStatus()
        if (mounted) {
          setBootstrapStatus(status)
        }
      } catch (error) {
        console.error('Failed to fetch bootstrap status', error)
      } finally {
        if (mounted) {
          setIsBootstrapStatusLoading(false)
        }
      }
    }

    fetchStatus()

    return () => {
      mounted = false
      observer?.disconnect()
    }
  }, [])

  const handleCopy = async (value: string, label: string) => {
    try {
      await navigator.clipboard.writeText(value)
      toast.success(`${label} copied to clipboard`)
    } catch (error) {
      console.error('Failed to copy', error)
      toast.error('Failed to copy to clipboard')
    }
  }

  const serverOrigin = typeof window !== 'undefined' ? window.location.origin : ''
  const wizardCommand = `pullbasectl bootstrap wizard --server-url ${serverOrigin}`

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault()
    setIsLoading(true)
    setErrorMessage(null)

    const result = await login({ username, password })

    if (result.success) {
      navigate('/ui/dashboard')
    } else {
      setErrorMessage(result.error || 'Invalid username or password')
    }
    
    setIsLoading(false)
  }

  return (
    <div className="min-h-screen flex items-center justify-center bg-background py-12 px-4 sm:px-6 lg:px-8">
      <div className="max-w-md w-full space-y-8">
        <div className="text-center">
          <img
            src={isDarkMode ? '/ui/logo-nav-white.svg' : '/ui/logo-nav-dark.svg'}
            alt="PullBase"
            className="h-12 w-auto mx-auto mb-4"
          />
          <p className="mt-2 text-sm text-muted-foreground">
            GitOps for Servers
          </p>
        </div>

        {!isBootstrapStatusLoading && bootstrapStatus?.bootstrap_enabled && (
          <Alert className="border-primary/30 bg-primary/5 text-left">
            <div className="flex flex-col gap-3">
              <AlertTitle className="flex items-center gap-2 text-primary">
                <Info className="h-4 w-4" />
                Complete first-time setup
              </AlertTitle>
              <AlertDescription className="space-y-4 text-sm text-primary/90">
                <p>
                  Pullbase hasn&apos;t been initialized yet. Use the CLI wizard to create your admin account
                  and register your GitHub App.
                </p>
                <div className="space-y-2">
                  <div className="flex items-center justify-between gap-2">
                    <span className="font-medium text-primary">Bootstrap wizard command</span>
                    <Button
                      type="button"
                      variant="outline"
                      size="sm"
                      className="gap-1"
                      onClick={() => handleCopy(wizardCommand, 'Command')}
                    >
                      <Copy className="h-3 w-3" />
                      Copy
                    </Button>
                  </div>
                  <pre className="bg-background border border-primary/20 rounded-md p-3 text-sm overflow-x-auto text-primary">
                    {wizardCommand}
                  </pre>
                </div>
                {bootstrapStatus.secret_path && (
                  <div className="space-y-2">
                    <div className="flex items-center justify-between gap-2">
                      <span className="font-medium text-primary">Bootstrap secret path</span>
                      <Button
                        type="button"
                        variant="outline"
                        size="sm"
                        className="gap-1"
                        onClick={() => handleCopy(bootstrapStatus.secret_path!, 'Secret path')}
                      >
                        <Copy className="h-3 w-3" />
                        Copy
                      </Button>
                    </div>
                    <code className="bg-background border border-primary/20 rounded-md px-3 py-2 text-sm block text-primary">
                      {bootstrapStatus.secret_path}
                    </code>
                    <p className="text-xs text-primary/80">
                      Run <code className="font-mono">docker compose exec central-server cat {bootstrapStatus.secret_path}</code> to print the secret.
                    </p>
                  </div>
                )}
              </AlertDescription>
            </div>
          </Alert>
        )}

        <Card className="border-border">
          <CardHeader>
            <CardTitle className="text-2xl text-center text-foreground">Sign in</CardTitle>
            <CardDescription className="text-center">
              Enter your credentials to access the dashboard
            </CardDescription>
          </CardHeader>
          <CardContent>
            {errorMessage && (
              <Alert className="mb-4 border-destructive/40 bg-destructive/10 text-destructive">
                <AlertTitle>Login failed</AlertTitle>
                <AlertDescription className="text-destructive">
                  {errorMessage}
                </AlertDescription>
              </Alert>
            )}
            <form onSubmit={handleSubmit} className="space-y-6">
              <div className="space-y-2">
                <Label htmlFor="username">Username</Label>
                <Input
                  id="username"
                  type="text"
                  value={username}
                  onChange={(e) => setUsername(e.target.value)}
                  required
                  disabled={isLoading}
                  placeholder="Enter your username"
                  className="appearance-none rounded-md relative block w-full"
                />
              </div>

              <div className="space-y-2">
                <Label htmlFor="password">Password</Label>
                <Input
                  id="password"
                  type="password"
                  value={password}
                  onChange={(e) => setPassword(e.target.value)}
                  required
                  disabled={isLoading}
                  placeholder="Enter your password"
                  allowPasswordToggle
                />
              </div>

              <Button
                type="submit"
                className="w-full bg-primary hover:bg-primary/90 text-primary-foreground"
                disabled={isLoading || !username || !password}
              >
                {isLoading ? (
                  <>
                    <Loader2 className="mr-2 h-4 w-4 animate-spin" />
                    Signing in...
                  </>
                ) : (
                  <>
                    <Lock className="mr-2 h-4 w-4" />
                    Sign in
                  </>
                )}
              </Button>
            </form>
          </CardContent>
        </Card>
      </div>
    </div>
  )
}
