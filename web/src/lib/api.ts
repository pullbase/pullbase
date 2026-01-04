import axios, { AxiosError } from 'axios'
import type { AxiosInstance } from 'axios'
import { toast } from 'sonner'

const API_BASE_URL =
  import.meta.env.MODE === 'development'
    ? import.meta.env.VITE_API_URL || 'http://localhost:8080'
    : `${window.location.protocol}//${window.location.host}`;

let activeRequests = 0
const dispatchLoadingEvent = (count: number) => {
  if (typeof window !== 'undefined') {
    window.dispatchEvent(new CustomEvent('pullbase:loading', { detail: { count } }))
  }
}

// Create axios instance
const api: AxiosInstance = axios.create({
  baseURL: API_BASE_URL,
  timeout: 10000,
  headers: {
    'Content-Type': 'application/json',
  }
})

// Request interceptor to add auth token
api.interceptors.request.use(
  (config) => {
    activeRequests += 1
    dispatchLoadingEvent(activeRequests)
    // Add auth token if available
    if (typeof window !== 'undefined') {
      const token = localStorage.getItem('auth_token')
      if (token) {
        config.headers.Authorization = `Bearer ${token}`
      }
    }
    return config
  },
  (error) => {
    activeRequests = Math.max(0, activeRequests - 1)
    dispatchLoadingEvent(activeRequests)
    return Promise.reject(error)
  }
)

api.interceptors.response.use(
  (response) => {
    activeRequests = Math.max(0, activeRequests - 1)
    dispatchLoadingEvent(activeRequests)
    return response
  },
  (error: AxiosError) => {
    activeRequests = Math.max(0, activeRequests - 1)
    dispatchLoadingEvent(activeRequests)
    console.log('API Error:', error.response?.status, error.config?.url, error.message)
    const requestUrl = error.config?.url ?? ''
    const isAuthLogin = requestUrl.includes('/api/v1/auth/login')

    if (error.response?.status === 401 && !isAuthLogin) {
      if (typeof window !== 'undefined') {
        console.log('Unauthorized, redirecting to login')
        localStorage.removeItem('auth_token')
        window.location.href = '/ui/login'
      }
    } else if (error.response && error.response.status >= 500) {
      toast.error('Server error. Please try again later.')
    } else if (error.code === 'NETWORK_ERROR') {
      toast.error('Network error. Please check your connection.')
    }
    
    return Promise.reject(error)
  }
)

// Types
export interface LoginRequest {
  username: string
  password: string
}

export interface UserSummary {
  id: number
  username: string
  role: string
}

export interface UsersListResponse {
  users: UserSummary[]
  total: number
  limit: number
  offset: number
  role?: string
}

export interface LoginResponse {
  access_token: string
  user: UserSummary
}

export interface BootstrapStatus {
  bootstrap_enabled: boolean
  secret_path?: string
  admin_count?: number
}

export interface Server {
  id: string
  name: string
  environment_id?: number
  environment_name?: string
  auto_reconcile: boolean
  last_status?: string
  last_is_drifted?: boolean
  last_timestamp?: string
  last_commit_hash?: string
  last_agent_version?: string
}

export interface ServerStatusHistory {
  id: string
  status: string
  commit_hash: string
  is_drifted: boolean
  error_message?: string
  timestamp: string
}

export interface AgentToken {
  id: number
  server_id: string
  description: string
  created_at: string
  expires_at?: string
  last_used_at?: string
  is_active: boolean
  created_by_user_id?: number
}

export interface CreateServerResponse {
  id: string
  name: string
  environment_id?: number
  auto_reconcile: boolean
  agent_token: string
  installation_info: {
    instructions: string
    example_cmd: string
  }
}

export interface CreateTokenRequest {
  description?: string
  expires_in?: number 
}

export interface CreateTokenResponse {
  id: number
  token: string
  description: string
  server_id: string
  created_at: string
  expires_at?: string
  installation_info: {
    instructions: string
    example_cmd: string
  }
}

export interface Environment {
  id: number
  name: string
  repo_url: string
  branch: string
  deploy_path: string
  provider: string
  installation_id: number
  app_slug?: string | null
  repository_id?: number | null
  notification_webhook_url?: string | null
  status?: string
  auto_reconcile: boolean
  created_at: string
  updated_at: string
  last_webhook_at?: string
  deployed_commit?: string
  webhook_status?: {
    status: string
    last_webhook: string
    retry_count: number
    error: string
  }
}

export interface EnvironmentHealthEntry {
  environment_id: number
  environment_name: string
  provider: string
  webhook_status: {
    status?: string
    retry_count?: number
    error?: string
  }
  deployed_commit?: string | null
  last_webhook_at?: string
  git_token_cooldown?: string | number | null
  git_token_next_allowed?: string
  git_token_history: Array<{
    timestamp: string
    allowed: boolean
    message: string
  }>
}

export interface CommitInfo {
  hash: string
  applied_at: string
  message: string
}

export interface CommitsResponse {
  commits: CommitInfo[]
  limit: number
}

export interface RollbackEvent {
  id: number
  environment_id: number
  from_commit: string
  to_commit: string
  initiated_by: string
  status: 'pending' | 'in_progress' | 'completed' | 'failed'
  reason: string
  created_at: string
  completed_at?: string
  error_message?: string
}

export interface RollbacksResponse {
  rollbacks: RollbackEvent[]
  limit: number
  offset: number
}

export interface PaginationResponse<T> {
  data: T[]
  total: number
  page: number
  limit: number
  total_pages: number
}

export interface ExpiringTokensResponse {
  tokens: Array<{
    id: number
    server_id: string
    description: string
    expires_at: string
    server_name: string
    environment_name: string
    days_until_expiry: number
  }>
  count: number
  days_checked: number
}

export interface DriftDetailsResponse {
  server_id: string
  server_name: string
  has_drift: boolean
  drift_details?: {
    packages?: Array<{
      name: string
      expected_state: string
      actual_state: string
      expected_version?: string
      actual_version?: string
    }>
    services?: Array<{
      name: string
      expected_state: string
      actual_state: string
      expected_enabled?: boolean
      actual_enabled?: boolean
    }>
    files?: Array<{
      path: string
      expected_mode?: string
      actual_mode?: string
      content_differs: boolean
      expected_content?: string
      actual_content?: string
    }>
  }
  detected_at?: string
}

export interface DriftMetricsResponse {
  period: string
  total_events: number
  time_series: Array<{ timestamp: string; value: number }>
}

export interface ReconciliationMetricsResponse {
  period: string
  total_applied: number
  total_failed: number
  success_rate: number
  time_series: Array<{ timestamp: string; value: number }>
}

export interface AgentConnectivityResponse {
  total_agents: number
  online_agents: number
  offline_agents: number
  stale_threshold: string
  agent_statuses: Array<{
    server_id: string
    server_name: string
    last_seen?: string
    is_online: boolean
    status?: string
  }>
}

export const metricsApi = {
  getDriftMetrics: async (days = 7): Promise<DriftMetricsResponse> => {
    const response = await api.get(`/api/v1/metrics/drift?days=${days}`)
    return response.data
  },

  getReconciliationMetrics: async (days = 7): Promise<ReconciliationMetricsResponse> => {
    const response = await api.get(`/api/v1/metrics/reconciliation?days=${days}`)
    return response.data
  },

  getAgentConnectivity: async (): Promise<AgentConnectivityResponse> => {
    const response = await api.get('/api/v1/metrics/connectivity')
    return response.data
  }
}

// Auth API
export const authApi = {
  login: async (credentials: LoginRequest): Promise<LoginResponse> => {
    const response = await api.post('/api/v1/auth/login', credentials)
    return response.data
  },

  getCurrentUser: async () => {
    const response = await api.get('/api/v1/auth/me')
    return response.data
  },

  getBootstrapStatus: async (): Promise<BootstrapStatus> => {
    const response = await api.get('/api/v1/bootstrap/status')
    return response.data
  }
}

// Servers API
export const serversApi = {
  list: async (page = 1, limit = 10, sort?: string): Promise<PaginationResponse<Server>> => {
    const params = new URLSearchParams({ page: String(page), limit: String(limit) })
    if (sort) {
      params.set('sort', sort)
    }
    const response = await api.get(`/api/v1/servers?${params.toString()}`)
    if (Array.isArray(response.data)) {
      return {
        data: response.data,
        total: response.data.length,
        page,
        limit,
        total_pages: 1
      }
    }
    return response.data
  },

  get: async (id: string): Promise<Server | null> => {
    const response = await api.get(`/api/v1/servers/${id}`)
    const data = response.data
    if (!data || typeof data !== 'object' || !('id' in data)) {
      return null
    }
    return data
  },

  create: async (server: { id: string; name: string; environment_id: number }): Promise<CreateServerResponse> => {
    const response = await api.post('/api/v1/servers', server)
    return response.data
  },

  update: async (id: string, server: Partial<Server>): Promise<Server> => {
    const response = await api.put(`/api/v1/servers/${id}`, server)
    return response.data
  },

  delete: async (id: string): Promise<void> => {
    await api.delete(`/api/v1/servers/${id}`)
  },

  getStatusHistory: async (id: string, page = 1, limit = 20): Promise<PaginationResponse<ServerStatusHistory>> => {
    const response = await api.get(`/api/v1/servers/${id}/status/history?page=${page}&limit=${limit}`)
    return response.data
  },

  toggleAutoReconcile: async (id: string): Promise<{ auto_reconcile: boolean }> => {
    const response = await api.post(`/api/v1/servers/${id}/toggle-auto-reconcile`)
    return response.data
  },

  listTokens: async (serverId: string): Promise<AgentToken[]> => {
    const response = await api.get(`/api/v1/servers/${serverId}/tokens`)
    return response.data
  },

  createToken: async (serverId: string, tokenRequest: CreateTokenRequest): Promise<CreateTokenResponse> => {
    const response = await api.post(`/api/v1/servers/${serverId}/tokens`, tokenRequest)
    return response.data
  },

  deactivateToken: async (serverId: string, tokenId: number): Promise<{ message: string }> => {
    const response = await api.delete(`/api/v1/servers/${serverId}/tokens/${tokenId}`)
    return response.data
  },

  getInstallInstructions: async (serverId: string): Promise<{
    server_id: string
    server_name: string
    instructions: string
    example_cmd?: string
    has_tokens: boolean
    active_tokens?: number
    latest_token_created?: string
  }> => {
    const response = await api.get(`/api/v1/servers/${serverId}/install`)
    return response.data
  },

  getDrift: async (serverId: string): Promise<DriftDetailsResponse> => {
    const response = await api.get(`/api/v1/servers/${serverId}/drift`)
    return response.data
  },

  getExpiringTokens: async (days = 7): Promise<ExpiringTokensResponse> => {
    const response = await api.get(`/api/v1/tokens/expiring?days=${days}`)
    return response.data
  }
}

// Environments API
export const environmentsApi = {
  list: async (page = 1, limit = 10, sort?: string): Promise<PaginationResponse<Environment>> => {
    const params = new URLSearchParams({ page: String(page), limit: String(limit) })
    if (sort) {
      params.set('sort', sort)
    }
    const response = await api.get(`/api/v1/environments?${params.toString()}`)
    if ('environments' in response.data) {
      return {
        data: response.data.environments,
        total: response.data.environments.length,
        page,
        limit,
        total_pages: 1
      }
    }
    if (response.data && Array.isArray(response.data.data)) {
      return response.data
    }
    // fallback for empty or malformed
    return {
      data: [],
      total: 0,
      page,
      limit,
      total_pages: 1
    }
  },

  get: async (id: number): Promise<Environment> => {
    const response = await api.get(`/api/v1/environments/${id}`)
    return response.data
  },

  health: async (): Promise<{ environments: EnvironmentHealthEntry[] }> => {
    const response = await api.get('/api/v1/environments/health')
    return response.data
  },

  create: async (environment: {
    name: string
    repo_url: string
    branch: string
    deploy_path: string
    installation_id: number
    app_slug?: string
    repository_id?: number
    notification_webhook_url?: string
    webhook_secret?: string
    auto_reconcile: boolean
  }): Promise<Environment> => {
    const response = await api.post('/api/v1/environments', {
      ...environment,
      provider: 'github'
    })
    return response.data
  },

  update: async (id: number, environment: {
    name: string
    repo_url: string
    branch: string
    deploy_path: string
    installation_id: number
    app_slug?: string
    repository_id?: number
    notification_webhook_url?: string
    webhook_secret?: string
    auto_reconcile: boolean
  }): Promise<Environment> => {
    const response = await api.put(`/api/v1/environments/${id}`, environment)
    return response.data
  },

  delete: async (id: number): Promise<void> => {
    await api.delete(`/api/v1/environments/${id}`)
  },

  getCommits: async (id: number): Promise<CommitsResponse> => {
    const response = await api.get(`/api/v1/environments/${id}/commits`)
    return response.data
  },

  initiateRollback: async (id: number, data: {
    to_commit: string
    reason?: string
  }): Promise<{
    rollback_id: number
    status: string
    message: string
  }> => {
    const response = await api.post(`/api/v1/environments/${id}/rollback`, data)
    return response.data
  },

  getRollbacks: async (id: number): Promise<RollbacksResponse> => {
    const response = await api.get(`/api/v1/environments/${id}/rollbacks`)
    return response.data
  },

  toggleAutoReconcile: async (id: number): Promise<{ auto_reconcile: boolean; message: string }> => {
    const response = await api.post(`/api/v1/environments/${id}/toggle-auto-reconcile`)
    return response.data
  }
}

export const usersApi = {
  create: async (payload: {
    username: string
    password: string
    role: string
  }): Promise<UserSummary> => {
    const response = await api.post('/api/v1/users', payload)
    return response.data.user as UserSummary
  },

  list: async (params?: { limit?: number; offset?: number; role?: string }): Promise<UsersListResponse> => {
    const searchParams = new URLSearchParams()
    if (params?.limit !== undefined) {
      searchParams.set('limit', String(params.limit))
    }
    if (params?.offset !== undefined) {
      searchParams.set('offset', String(params.offset))
    }
    if (params?.role) {
      searchParams.set('role', params.role)
    }
    const query = searchParams.toString()
    const response = await api.get(`/api/v1/users${query ? `?${query}` : ''}`)
    return response.data as UsersListResponse
  },

  delete: async (id: number, confirmUsername: string): Promise<void> => {
    await api.delete(`/api/v1/users/${id}`, {
      data: { confirm_username: confirmUsername }
    })
  }
}

export default api 
