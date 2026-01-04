import { createContext, useContext, useEffect, useState } from 'react'
import type { ReactNode } from 'react'
import { authApi, type UserSummary } from '../lib/api'
import { toast } from 'sonner'

type User = UserSummary

interface LoginRequest {
  username: string
  password: string
}

interface LoginResponse {
  access_token: string
  user: User
}

interface AuthContextType {
  user: User | null
  isLoading: boolean
  isAuthenticated: boolean
  login: (credentials: LoginRequest) => Promise<{ success: boolean; error?: string }>
  logout: () => void
}

const AuthContext = createContext<AuthContextType | undefined>(undefined)

export function AuthProvider({ children }: { children: ReactNode }) {
  const [user, setUser] = useState<User | null>(null)
  const [isLoading, setIsLoading] = useState(true)

  const isAuthenticated = !!user

  useEffect(() => {
    const token = localStorage.getItem('auth_token')
    if (token) {
      authApi.getCurrentUser()
        .then((userData) => {
          setUser(userData as User)
          console.log('User loaded', userData)
        })
        .catch((err) => {
          console.log('Auth error', err)
          localStorage.removeItem('auth_token')
        })
        .finally(() => {
          setIsLoading(false)
          console.log('Auth loading finished')
        })
    } else {
      setIsLoading(false)
      console.log('No token, loading finished')
    }
  }, [])

  const login = async (credentials: LoginRequest): Promise<{ success: boolean; error?: string }> => {
    try {
      setIsLoading(true)
      const response: LoginResponse = await authApi.login(credentials)
      
      localStorage.setItem('auth_token', response.access_token)

      const isSecure = window.location.protocol === 'https:';
      document.cookie = `session_token=${response.access_token}; Path=/; SameSite=Lax;${isSecure ? ' Secure;' : ''}`;

      setUser(response.user)
      
      toast.success('Login successful')
      return { success: true }
    } catch (error: unknown) {
      const errorMessage =
        (error as { response?: { data?: { error?: string } } }).response?.data?.error ||
        'Login failed'
      toast.error(errorMessage)
      return { success: false, error: errorMessage }
    } finally {
      setIsLoading(false)
    }
  }

  const logout = () => {
    localStorage.removeItem('auth_token')
    setUser(null)
    toast.success('Logged out successfully')
    window.location.href = '/ui/login'
  }

  return (
    <AuthContext.Provider
      value={{
        user,
        isLoading,
        isAuthenticated,
        login,
        logout,
      }}
    >
      {children}
    </AuthContext.Provider>
  )
}

export function useAuth() {
  const context = useContext(AuthContext)
  if (context === undefined) {
    throw new Error('useAuth must be used within an AuthProvider')
  }
  return context
} 
