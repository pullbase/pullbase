import { useState, useEffect, useMemo } from 'react'
import { Link } from 'react-router-dom'
import {
  serversApi,
  environmentsApi,
  authApi,
  metricsApi,
  type Server,
  type Environment,
  type EnvironmentHealthEntry,
  type BootstrapStatus,
  type DriftMetricsResponse,
  type ReconciliationMetricsResponse,
  type AgentConnectivityResponse
} from '../lib/api'
import { Button } from '../components/ui/button'
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '../components/ui/card'
import { Badge } from '../components/ui/badge'
import { Skeleton } from '../components/ui/skeleton'
import {
  LineChart,
  Line,
  XAxis,
  YAxis,
  CartesianGrid,
  Tooltip,
  ResponsiveContainer,
  AreaChart,
  Area
} from 'recharts'
import {
  Activity,
  AlertTriangle,
  CheckCircle2,
  Clock,
  GitBranch,
  Server as ServerIcon,
  AlertCircle,
  XCircle,
  ShieldCheck,
  RefreshCw,
  KeyRound,
  ArrowUpRight,
  GitCommit,
  type LucideIcon
} from 'lucide-react'
import { formatDistanceToNow } from 'date-fns'
import { useAuth } from '../contexts/auth-context'
import ExpiringTokensBadge from '../components/ExpiringTokensBadge'

interface DashboardStats {
  totalServers: number
  healthyServers: number
  driftedServers: number
  errorServers: number
  totalEnvironments: number
  activeEnvironments: number
}

export default function DashboardPage() {
  const [stats, setStats] = useState<DashboardStats | null>(null)
  const [recentServers, setRecentServers] = useState<Server[]>([])
  const [environments, setEnvironments] = useState<Environment[]>([])
  const [environmentHealth, setEnvironmentHealth] = useState<EnvironmentHealthEntry[]>([])
  const [isLoading, setIsLoading] = useState(true)
  const [servers, setServers] = useState<Server[]>([])
  const [bootstrapStatus, setBootstrapStatus] = useState<BootstrapStatus | null>(null)
  const [driftMetrics, setDriftMetrics] = useState<DriftMetricsResponse | null>(null)
  const [reconciliationMetrics, setReconciliationMetrics] = useState<ReconciliationMetricsResponse | null>(null)
  const [_connectivityMetrics, setConnectivityMetrics] = useState<AgentConnectivityResponse | null>(null)
  const { user } = useAuth()
  const isAdmin = user?.role === 'admin'

  const loadDashboardData = async () => {
    try {
      setIsLoading(true)

      const [driftData, reconData, connData] = await Promise.all([
        metricsApi.getDriftMetrics(7).catch(() => null),
        metricsApi.getReconciliationMetrics(7).catch(() => null),
        metricsApi.getAgentConnectivity().catch(() => null)
      ])

      if (driftData) setDriftMetrics(driftData)
      if (reconData) setReconciliationMetrics(reconData)
      if (connData) setConnectivityMetrics(connData)

      // Load servers for stats and recent activity
      const serversData = await serversApi.list(1, 50, 'recent')
      const servers = Array.isArray(serversData) ? serversData : (serversData?.data || [])
      setServers(servers)

      // Load environments
      const environmentsData = await environmentsApi.list(1, 20, 'recent')
      const envs = Array.isArray(environmentsData) ? environmentsData : (environmentsData?.data || [])

      // Load environment health
      try {
        const healthResponse = await environmentsApi.health()
        if (healthResponse && Array.isArray(healthResponse.environments)) {
          setEnvironmentHealth(healthResponse.environments)
        } else {
          setEnvironmentHealth([])
        }
      } catch (healthError) {
        console.warn('Failed to load environment health:', healthError)
        setEnvironmentHealth([])
      }

      // Calculate stats
      const healthyServers = servers.filter(s => s.last_status === 'Applied' && !s.last_is_drifted).length
      const driftedServers = servers.filter(s => s.last_is_drifted).length
      const errorServers = servers.filter(s => s.last_status === 'Failed').length
      const activeEnvironments = envs.filter(e => e.deployed_commit).length

      setStats({
        totalServers: servers.length,
        healthyServers,
        driftedServers,
        errorServers,
        totalEnvironments: envs.length,
        activeEnvironments
      })

      // Get recent servers (sorted by last update)
      const recent = servers
        .filter(s => s.last_timestamp)
        .sort((a, b) => new Date(b.last_timestamp!).getTime() - new Date(a.last_timestamp!).getTime())
        .slice(0, 10)

      setRecentServers(recent)
      setEnvironments(envs)

    } catch (error) {
      console.error('Failed to load dashboard data:', error)
    } finally {
      setIsLoading(false)
    }
  }

  useEffect(() => {
    loadDashboardData()
  }, [])

  useEffect(() => {
    if (!isAdmin) {
      setBootstrapStatus(null)
      return
    }
    authApi
      .getBootstrapStatus()
      .then((status) => setBootstrapStatus(status))
      .catch(() => setBootstrapStatus(null))
  }, [isAdmin])

  type StatusMeta = {
    icon: LucideIcon
    tone: string
    label: string
    description: string
    footnote?: string
  }

  const formatRelativeTime = (value?: string | null) => {
    if (!value) return null
    try {
      return formatDistanceToNow(new Date(value), { addSuffix: true })
    } catch {
      return null
    }
  }

  const formatDurationValue = (value?: string | number | null) => {
    if (value === undefined || value === null) return null
    if (typeof value === 'number') {
      if (value === 0) return '0s'
      const seconds = Math.round(value / 1e9)
      if (seconds < 60) return `${seconds}s`
      const minutes = Math.floor(seconds / 60)
      const remainingSeconds = seconds % 60
      if (minutes < 60) {
        return remainingSeconds ? `${minutes}m ${remainingSeconds}s` : `${minutes}m`
      }
      const hours = Math.floor(minutes / 60)
      const remainingMinutes = minutes % 60
      return `${hours}h${remainingMinutes ? ` ${remainingMinutes}m` : ''}`
    }
    if (typeof value === 'string') {
      return value
        .replace(/([a-zA-Z])(?=\d)/g, '$1 ')
        .replace(/\s+/g, ' ')
        .trim()
    }
    return null
  }

  const getProviderLabel = (provider?: string) => {
    if (!provider) return 'Unknown provider'
    const normalized = provider.toLowerCase()
    if (normalized === 'github') return 'GitHub'
    if (normalized === 'gitlab') return 'GitLab'
    return provider.charAt(0).toUpperCase() + provider.slice(1)
  }

  const getWebhookMeta = (entry: EnvironmentHealthEntry): StatusMeta => {
    const status = entry.webhook_status?.status?.toLowerCase() || 'unknown'
    const retryCount = entry.webhook_status?.retry_count ?? 0
    const lastWebhook = formatRelativeTime(entry.last_webhook_at)

    switch (status) {
      case 'active':
      case 'healthy':
        return {
          icon: ShieldCheck,
          tone: 'bg-emerald-500/10 text-emerald-500',
          label: 'Webhook active',
          description: lastWebhook ? `Last event ${lastWebhook}` : 'Awaiting first webhook event.',
          footnote: retryCount > 0 ? `Recent retries: ${retryCount}` : undefined
        }
      case 'pending':
        return {
          icon: Clock,
          tone: 'bg-amber-500/10 text-amber-600',
          label: 'Awaiting webhook',
          description: lastWebhook ? `Last event ${lastWebhook}` : 'No webhook events received yet.',
          footnote: retryCount > 0 ? `Retry attempts: ${retryCount}` : undefined
        }
      case 'error':
        return {
          icon: AlertTriangle,
          tone: 'bg-red-500/10 text-red-500',
          label: 'Webhook errors',
          description: lastWebhook ? `Last success ${lastWebhook}` : 'No successful webhook deliveries recorded.',
          footnote: retryCount > 0 ? `Active retries: ${retryCount}` : 'Check GitHub App configuration.'
        }
      default:
        return {
          icon: RefreshCw,
          tone: 'bg-muted text-muted-foreground',
          label: 'Status unknown',
          description: lastWebhook ? `Last event ${lastWebhook}` : 'No webhook updates recorded yet.',
          footnote: retryCount > 0 ? `Retry attempts: ${retryCount}` : undefined
        }
    }
  }

  const getTokenMeta = (entry: EnvironmentHealthEntry): StatusMeta => {
    const now = Date.now()
    let nextAllowedDate: Date | null = null
    if (entry.git_token_next_allowed) {
      const parsed = new Date(entry.git_token_next_allowed)
      if (!Number.isNaN(parsed.getTime())) {
        nextAllowedDate = parsed
      }
    }

    const cooldownActive = nextAllowedDate ? nextAllowedDate.getTime() > now : false
    const cooldownLabel = formatDurationValue(entry.git_token_cooldown)
    const nextAllowedRelative = entry.git_token_next_allowed ? formatRelativeTime(entry.git_token_next_allowed) : null
    const sortedHistory = [...entry.git_token_history].sort(
      (a, b) => new Date(b.timestamp).getTime() - new Date(a.timestamp).getTime()
    )
    const lastAttempt = sortedHistory[0]
    const lastAttemptRelative = lastAttempt?.timestamp ? formatRelativeTime(lastAttempt.timestamp) : null
    const blockedCount = entry.git_token_history.filter(item => !item.allowed).length

    if (cooldownActive) {
      return {
        icon: AlertTriangle,
        tone: 'bg-amber-500/10 text-amber-600',
        label: 'Token issuance cooling down',
        description: nextAllowedRelative ? `Next token ${nextAllowedRelative}` : 'Next token issuance pending cooldown.',
        footnote: cooldownLabel ? `Cooldown window: ${cooldownLabel}` : undefined
      }
    }

    if (lastAttempt && !lastAttempt.allowed) {
      return {
        icon: AlertCircle,
        tone: 'bg-red-500/10 text-red-500',
        label: 'Latest token request blocked',
        description: lastAttemptRelative ? `Attempt ${lastAttemptRelative}` : 'Recent token request failed.',
        footnote: lastAttempt.message
      }
    }

    if (lastAttempt && lastAttempt.allowed) {
      return {
        icon: KeyRound,
        tone: 'bg-emerald-500/10 text-emerald-500',
        label: 'Token issuance healthy',
        description: lastAttemptRelative ? `Last issued ${lastAttemptRelative}` : 'Tokens issued recently.',
        footnote: blockedCount > 0 ? `${blockedCount} blocked attempt${blockedCount === 1 ? '' : 's'} recorded` : undefined
      }
    }

    return {
      icon: Clock,
      tone: 'bg-muted text-muted-foreground',
      label: 'Awaiting agent activity',
      description: 'No token requests observed yet.'
    }
  }

  const getStatusBadge = (status?: string, isDrifted?: boolean) => {
    if (!status) {
      return (
        <Badge variant="secondary" className="gap-1">
          <Clock className="w-3 h-3" />
          Unknown
        </Badge>
      )
    }

    if (status === 'Applied') {
      if (isDrifted) {
        return (
          <Badge variant="destructive" className="gap-1">
            <AlertCircle className="w-3 h-3" />
            Drifted
          </Badge>
        )
      }
      return (
        <Badge className="bg-green-500 hover:bg-green-600 gap-1">
          <CheckCircle2 className="w-3 h-3" />
          Applied
        </Badge>
      )
    }

    if (status === 'Failed') {
      return (
        <Badge variant="destructive" className="gap-1">
          <XCircle className="w-3 h-3" />
          Failed
        </Badge>
      )
    }

    return (
      <Badge variant="secondary" className="gap-1">
        <Clock className="w-3 h-3" />
        {status}
      </Badge>
    )
  }

  const formatLastUpdate = (timestamp?: string) => {
    if (!timestamp) return 'Never'
    try {
      return formatDistanceToNow(new Date(timestamp), { addSuffix: true })
    } catch {
      return 'Invalid date'
    }
  }

  const getHealthPercentage = () => {
    if (!stats || stats.totalServers === 0) return 0
    return Math.round((stats.healthyServers / stats.totalServers) * 100)
  }

  const healthSummary = environmentHealth.reduce(
    (acc, entry) => {
      const status = entry.webhook_status?.status?.toLowerCase() || 'unknown'
      if (status === 'active' || status === 'healthy') {
        acc.active += 1
      } else if (status === 'pending') {
        acc.pending += 1
      } else if (status === 'error') {
        acc.error += 1
      } else {
        acc.unknown += 1
      }
      return acc
    },
    { active: 0, pending: 0, error: 0, unknown: 0 }
  )

  const getEnvironmentStatusRank = (entry: EnvironmentHealthEntry) => {
    const status = entry.webhook_status?.status?.toLowerCase() || 'unknown'
    if (status === 'error') return 0
    if (status === 'pending') return 1
    if (status === 'active' || status === 'healthy') return 2
    return 3
  }

  const sortedEnvironmentHealth = environmentHealth
    .slice()
    .sort((a, b) => {
      const rankDiff = getEnvironmentStatusRank(a) - getEnvironmentStatusRank(b)
      if (rankDiff !== 0) {
        return rankDiff
      }
      return a.environment_name.localeCompare(b.environment_name)
    })

  const webhookIssues = useMemo(
    () => environmentHealth.filter((entry) => {
      const status = entry.webhook_status?.status?.toLowerCase()
      return status === 'error' || status === 'pending'
    }).length,
    [environmentHealth]
  )

  const tokenNeeds = useMemo(
    () => environmentHealth.filter((entry) => {
      if (!entry.git_token_history || entry.git_token_history.length === 0) return true
      return entry.git_token_history.every((attempt) => !attempt.allowed)
    }).length,
    [environmentHealth]
  )

  const recentWindow = useMemo(() => Date.now() - 24 * 60 * 60 * 1000, [])

  const recentApplied = useMemo(
    () =>
      servers.filter(
        (server) =>
          server.last_status === 'Applied' &&
          server.last_timestamp &&
          new Date(server.last_timestamp).getTime() >= recentWindow
      ).length,
    [servers, recentWindow]
  )

  const recentFailed = useMemo(
    () =>
      servers.filter(
        (server) =>
          server.last_status === 'Failed' &&
          server.last_timestamp &&
          new Date(server.last_timestamp).getTime() >= recentWindow
      ).length,
    [servers, recentWindow]
  )

  const staleThresholdMs = 5 * 60 * 1000
  const staleServers = servers.filter((server) => {
    if (!server.last_timestamp) return true
    const lastSeen = new Date(server.last_timestamp).getTime()
    return Date.now() - lastSeen > staleThresholdMs
  }).length
  const healthyAgents = Math.max(0, servers.length - staleServers)

  const pendingDeployments = environments.filter((env) => {
    const status = env.webhook_status?.status?.toLowerCase()
    return !env.deployed_commit || status === 'pending' || status === 'error'
  }).length

  const bootstrapEnabled = bootstrapStatus?.bootstrap_enabled ?? false

  return (
    <div className="space-y-6">
      {/* Header */}
      <div className="flex justify-between items-center">
        <div>
          <h1 className="text-3xl font-bold text-foreground">Dashboard</h1>
          <p className="text-muted-foreground mt-1">
            Overview of your GitOps configuration management infrastructure
          </p>
        </div>
        <div className="flex gap-2 items-center">
          <ExpiringTokensBadge />
          <Link to="/ui/environments">
            <Button variant="outline" className="gap-2">
              <GitBranch className="w-4 h-4" />
              Manage Environments
            </Button>
          </Link>
          <Link to="/ui/servers">
            <Button className="gap-2">
              <ServerIcon className="w-4 h-4" />
              View All Servers
            </Button>
          </Link>
        </div>
      </div>

      {/* Stats Cards */}
      {isLoading ? (
        <div className="grid gap-4 md:grid-cols-2 xl:grid-cols-3 2xl:grid-cols-4">
          {[...Array(6)].map((_, i) => (
            <Card key={i}>
              <CardHeader className="flex flex-row items-center justify-between space-y-0 pb-2">
                <Skeleton className="h-4 w-24" />
                <Skeleton className="h-4 w-4" />
              </CardHeader>
              <CardContent>
                <Skeleton className="h-8 w-16 mb-1" />
                <Skeleton className="h-3 w-32" />
              </CardContent>
            </Card>
          ))}
        </div>
      ) : (
        <div className="space-y-6">
          <div className="grid gap-4 md:grid-cols-2 xl:grid-cols-3 2xl:grid-cols-4">
            <Card>
              <CardHeader className="flex flex-row items-center justify-between space-y-0 pb-2">
                <CardTitle className="text-sm font-medium">Total Servers</CardTitle>
                <ServerIcon className="h-4 w-4 text-muted-foreground" />
              </CardHeader>
              <CardContent>
                <div className="text-2xl font-bold">{stats?.totalServers || 0}</div>
                <p className="text-xs text-muted-foreground">
                  {stats?.activeEnvironments || 0} active environments
                </p>
              </CardContent>
            </Card>

            <Card>
              <CardHeader className="flex flex-row items-center justify-between space-y-0 pb-2">
                <CardTitle className="text-sm font-medium">Health Status</CardTitle>
                <Activity className="h-4 w-4 text-muted-foreground" />
              </CardHeader>
              <CardContent>
                <div className="text-2xl font-bold">{getHealthPercentage()}%</div>
                <p className="text-xs text-muted-foreground">
                  {stats?.healthyServers || 0} of {stats?.totalServers || 0} healthy
                </p>
              </CardContent>
            </Card>

            <Card>
              <CardHeader className="flex flex-row items-center justify-between space-y-0 pb-2">
                <CardTitle className="text-sm font-medium">Drifted</CardTitle>
                <AlertTriangle className="h-4 w-4 text-muted-foreground" />
              </CardHeader>
              <CardContent>
                <div className="text-2xl font-bold">{stats?.driftedServers || 0}</div>
                <p className="text-xs text-muted-foreground">
                  Servers with configuration drift
                </p>
              </CardContent>
            </Card>

            <Card>
              <CardHeader className="flex flex-row items-center justify-between space-y-0 pb-2">
                <CardTitle className="text-sm font-medium">Errors</CardTitle>
                <XCircle className="h-4 w-4 text-muted-foreground" />
              </CardHeader>
              <CardContent>
                <div className="text-2xl font-bold">{stats?.errorServers || 0}</div>
                <p className="text-xs text-muted-foreground">
                  Failed configuration attempts
                </p>
              </CardContent>
            </Card>

            <Card>
              <CardHeader className="flex flex-row items-center justify-between space-y-0 pb-2">
                <CardTitle className="text-sm font-medium">Webhook Reliability</CardTitle>
                <AlertTriangle className="h-4 w-4 text-muted-foreground" />
              </CardHeader>
              <CardContent>
                <div className="text-2xl font-bold">{webhookIssues}</div>
                <p className="text-xs text-muted-foreground">
                  Environment{webhookIssues === 1 ? '' : 's'} reporting webhook issues
                </p>
              </CardContent>
            </Card>

            <Card>
              <CardHeader className="flex flex-row items-center justify-between space-y-0 pb-2">
                <CardTitle className="text-sm font-medium">Token Posture</CardTitle>
                <KeyRound className="h-4 w-4 text-muted-foreground" />
              </CardHeader>
              <CardContent>
                <div className="text-2xl font-bold">{tokenNeeds}</div>
                <p className="text-xs text-muted-foreground">
                  Environment{tokenNeeds === 1 ? '' : 's'} without recent token issuance
                </p>
              </CardContent>
            </Card>

            <Card>
              <CardHeader className="flex flex-row items-center justify-between space-y-0 pb-2">
                <CardTitle className="text-sm font-medium">Agent Connectivity</CardTitle>
                <Activity className="h-4 w-4 text-muted-foreground" />
              </CardHeader>
              <CardContent>
                <div className="text-2xl font-bold">{healthyAgents}/{servers.length}</div>
                <p className="text-xs text-muted-foreground">
                  {staleServers} agent{staleServers === 1 ? '' : 's'} offline &gt; 5 minutes
                </p>
              </CardContent>
            </Card>

            <Card>
              <CardHeader className="flex flex-row items-center justify-between space-y-0 pb-2">
                <CardTitle className="text-sm font-medium">Git Sync Health</CardTitle>
                <GitCommit className="h-4 w-4 text-muted-foreground" />
              </CardHeader>
              <CardContent>
                <div className="text-2xl font-bold">{pendingDeployments}</div>
                <p className="text-xs text-muted-foreground">
                  Environment{pendingDeployments === 1 ? '' : 's'} awaiting deployment
                </p>
              </CardContent>
            </Card>

            {isAdmin && (
              <Card>
                <CardHeader className="flex flex-row items-center justify-between space-y-0 pb-2">
                  <CardTitle className="text-sm font-medium">Bootstrap Status</CardTitle>
                  <ShieldCheck className={`h-4 w-4 ${bootstrapEnabled ? 'text-emerald-500' : 'text-muted-foreground'}`} />
                </CardHeader>
                <CardContent>
                  <div className="text-2xl font-bold">{bootstrapEnabled ? 'Enabled' : 'Disabled'}</div>
                  <p className="text-xs text-muted-foreground">
                    {bootstrapEnabled
                      ? 'Disable bootstrap after onboarding to lock down access.'
                      : 'Bootstrap closed. Use CLI to manage additional admins.'}
                  </p>
                  {typeof bootstrapStatus?.admin_count === 'number' && (
                    <p className="text-xs text-muted-foreground mt-1">
                      Active admins: {bootstrapStatus?.admin_count}
                    </p>
                  )}
                </CardContent>
              </Card>
            )}
          </div>

          <div className="grid gap-6 md:grid-cols-2">
            <Card>
              <CardHeader>
                <CardTitle className="text-sm font-medium">Drift Events (7 Days)</CardTitle>
                <CardDescription>
                  Number of drift events detected over time
                </CardDescription>
              </CardHeader>
              <CardContent>
                <div className="h-[200px] w-full">
                  {driftMetrics?.time_series && driftMetrics.time_series.length > 0 ? (
                    <ResponsiveContainer width="100%" height="100%">
                      <LineChart data={driftMetrics.time_series}>
                        <CartesianGrid strokeDasharray="3 3" vertical={false} />
                        <XAxis 
                          dataKey="timestamp" 
                          tickFormatter={(value) => new Date(value).toLocaleDateString(undefined, { month: 'short', day: 'numeric' })}
                          tick={{ fontSize: 12, fill: '#888888' }}
                          tickLine={false}
                          axisLine={false}
                        />
                        <YAxis 
                          tick={{ fontSize: 12, fill: '#888888' }}
                          tickLine={false}
                          axisLine={false}
                          allowDecimals={false}
                        />
                        <Tooltip 
                          contentStyle={{ borderRadius: '6px', border: '1px solid #e2e8f0', boxShadow: '0 4px 6px -1px rgb(0 0 0 / 0.1)' }}
                          labelFormatter={(label) => new Date(label).toLocaleDateString()}
                        />
                        <Line 
                          type="monotone" 
                          dataKey="value" 
                          stroke="#ef4444" 
                          strokeWidth={2}
                          dot={false}
                          activeDot={{ r: 4, strokeWidth: 0 }}
                        />
                      </LineChart>
                    </ResponsiveContainer>
                  ) : (
                    <div className="flex h-full items-center justify-center text-sm text-muted-foreground">
                      No drift data available
                    </div>
                  )}
                </div>
              </CardContent>
            </Card>

            <Card>
              <CardHeader>
                <CardTitle className="text-sm font-medium">Reconciliation Success Rate</CardTitle>
                <CardDescription>
                  Successful vs failed applications over time
                </CardDescription>
              </CardHeader>
              <CardContent>
                <div className="h-[200px] w-full">
                  {reconciliationMetrics?.time_series && reconciliationMetrics.time_series.length > 0 ? (
                    <ResponsiveContainer width="100%" height="100%">
                      <AreaChart data={reconciliationMetrics.time_series}>
                        <defs>
                          <linearGradient id="colorValue" x1="0" y1="0" x2="0" y2="1">
                            <stop offset="5%" stopColor="#10b981" stopOpacity={0.1}/>
                            <stop offset="95%" stopColor="#10b981" stopOpacity={0}/>
                          </linearGradient>
                        </defs>
                        <CartesianGrid strokeDasharray="3 3" vertical={false} />
                        <XAxis 
                          dataKey="timestamp" 
                          tickFormatter={(value) => new Date(value).toLocaleDateString(undefined, { month: 'short', day: 'numeric' })}
                          tick={{ fontSize: 12, fill: '#888888' }}
                          tickLine={false}
                          axisLine={false}
                        />
                        <YAxis 
                          tick={{ fontSize: 12, fill: '#888888' }}
                          tickLine={false}
                          axisLine={false}
                          unit="%"
                        />
                        <Tooltip 
                          contentStyle={{ borderRadius: '6px', border: '1px solid #e2e8f0', boxShadow: '0 4px 6px -1px rgb(0 0 0 / 0.1)' }}
                          labelFormatter={(label) => new Date(label).toLocaleDateString()}
                          formatter={(value) => [`${value ?? 0}%`, 'Success Rate']}
                        />
                        <Area 
                          type="monotone" 
                          dataKey="value" 
                          stroke="#10b981" 
                          fillOpacity={1} 
                          fill="url(#colorValue)" 
                        />
                      </AreaChart>
                    </ResponsiveContainer>
                  ) : (
                    <div className="flex h-full items-center justify-center text-sm text-muted-foreground">
                      No reconciliation data available
                    </div>
                  )}
                </div>
              </CardContent>
            </Card>
          </div>
        </div>
      )}

      {/* Recent Activity */}
      <div className="grid gap-6 md:grid-cols-2">
        <Card>
          <CardHeader>
            <CardTitle className="flex items-center gap-2">
              <Activity className="w-5 h-5" />
              Recent Server Activity
            </CardTitle>
            <CardDescription>
              Latest configuration enforcement and drift detection results
            </CardDescription>
          </CardHeader>
          <CardContent>
            {!isLoading && (
              <div className="mb-4 flex flex-wrap items-center gap-3 text-xs text-muted-foreground">
                <span className="flex items-center gap-1 text-emerald-600">
                  <CheckCircle2 className="w-3 h-3" /> {recentApplied} applied
                </span>
                <span className="flex items-center gap-1 text-red-600">
                  <XCircle className="w-3 h-3" /> {recentFailed} failed
                </span>
              </div>
            )}
            {isLoading ? (
              <div className="space-y-4">
                {[...Array(5)].map((_, i) => (
                  <div key={i} className="flex items-center space-x-4">
                    <Skeleton className="h-4 w-[100px]" />
                    <Skeleton className="h-4 w-[120px]" />
                    <Skeleton className="h-4 w-[80px]" />
                  </div>
                ))}
              </div>
            ) : recentServers.length === 0 ? (
              <div className="text-center py-8">
                <ServerIcon className="w-8 h-8 text-muted-foreground mx-auto mb-2" />
                <p className="text-muted-foreground">No server activity found</p>
                <Link to="/ui/servers">
                  <Button variant="outline" size="sm" className="mt-2">
                    Add Server
                  </Button>
                </Link>
              </div>
            ) : (
              <div className="space-y-4">
                {recentServers.map((server) => (
                  <div key={server.id} className="flex items-center justify-between">
                    <div className="flex items-center gap-3">
                      <ServerIcon className="w-4 h-4 text-muted-foreground" />
                      <div>
                        <Link to={`/ui/servers/${server.id}`} className="font-medium hover:underline">
                          {server.name}
                        </Link>
                        <p className="text-sm text-muted-foreground">
                          {formatLastUpdate(server.last_timestamp)}
                        </p>
                      </div>
                    </div>
                    <div className="flex items-center gap-2">
                      {getStatusBadge(server.last_status, server.last_is_drifted)}
                    </div>
                  </div>
                ))}
                <div className="pt-2">
                  <Link to="/ui/servers">
                    <Button variant="outline" size="sm" className="w-full">
                      View All Servers
                    </Button>
                  </Link>
                </div>
              </div>
            )}
          </CardContent>
        </Card>

        <Card>
          <CardHeader>
            <CardTitle className="flex items-center gap-2">
              <GitBranch className="w-5 h-5" />
              Environments Overview
            </CardTitle>
            <CardDescription>
              GitOps Configuration Environments (dev, staging, prod)
            </CardDescription>
          </CardHeader>
          <CardContent>
            {isLoading ? (
              <div className="space-y-4">
                {[...Array(5)].map((_, i) => (
                  <div key={i} className="flex items-center space-x-4">
                    <Skeleton className="h-4 w-[100px]" />
                    <Skeleton className="h-4 w-[120px]" />
                    <Skeleton className="h-4 w-[80px]" />
                  </div>
                ))}
              </div>
            ) : environments.length === 0 ? (
              <div className="text-center py-8">
                <GitBranch className="w-8 h-8 text-muted-foreground mx-auto mb-2" />
                <p className="text-muted-foreground">No environments configured</p>
                <Link to="/ui/environments/new">
                  <Button variant="outline" size="sm" className="mt-2">
                    Create Environment
                  </Button>
                </Link>
              </div>
            ) : (
              <div className="space-y-4">
                {environments.slice(0, 10).map((env) => {
                  const webhookStatus = (env.webhook_status?.status || env.status || '').toLowerCase()
                  const webhookActive = webhookStatus === 'active' || webhookStatus === 'healthy'

                  return (
                    <div key={env.id} className="flex items-center justify-between">
                      <div className="flex items-center gap-3">
                        <GitBranch className="w-4 h-4 text-muted-foreground" />
                        <div>
                          <Link to={`/ui/environments`} className="font-medium hover:underline">
                            {env.name}
                          </Link>
                          <p className="text-sm text-muted-foreground">
                            {env.repo_url?.replace('https://github.com/', '') || 'Repository not set'} •{' '}
                            {env.deployed_commit ? `${env.deployed_commit.substring(0, 8)}` : 'Not deployed'}
                          </p>
                        </div>
                      </div>
                      <div className="flex items-center gap-2">
                        <Badge variant={webhookActive ? 'default' : 'secondary'}>
                          {webhookActive ? 'Active' : 'Inactive'}
                        </Badge>
                      </div>
                    </div>
                  )
                })}
                <div className="pt-2">
                  <Link to="/ui/environments">
                    <Button variant="outline" size="sm" className="w-full">
                      Manage Environments
                    </Button>
                  </Link>
                </div>
              </div>
            )}
          </CardContent>
        </Card>
      </div>

      {/* Environment Health */}
      <Card>
        <CardHeader>
          <CardTitle className="flex items-center gap-2">
            <ShieldCheck className="w-5 h-5" />
            Environment Health
          </CardTitle>
          <CardDescription>
            Monitor webhook delivery and Git token issuance across environments.
          </CardDescription>
        </CardHeader>
        <CardContent className="space-y-4">
          {!isLoading && environmentHealth.length > 0 && (
            <div className="flex flex-wrap items-center gap-4 text-sm">
              <span className="flex items-center gap-1 text-emerald-500">
                <ShieldCheck className="w-4 h-4" />
                {healthSummary.active} healthy
              </span>
              <span className="flex items-center gap-1 text-amber-600">
                <Clock className="w-4 h-4" />
                {healthSummary.pending} pending
              </span>
              <span className="flex items-center gap-1 text-red-500">
                <AlertTriangle className="w-4 h-4" />
                {healthSummary.error} attention
              </span>
              {healthSummary.unknown > 0 && (
                <span className="flex items-center gap-1 text-muted-foreground">
                  <RefreshCw className="w-4 h-4" />
                  {healthSummary.unknown} unknown
                </span>
              )}
            </div>
          )}

          {isLoading ? (
            <div className="grid gap-4 md:grid-cols-2">
              {[...Array(2)].map((_, i) => (
                <div key={i} className="rounded-lg border border-border/40 bg-muted/30 p-4 shadow-sm space-y-3">
                  <Skeleton className="h-5 w-32" />
                  <Skeleton className="h-4 w-3/4" />
                  <Skeleton className="h-4 w-1/2" />
                  <Skeleton className="h-4 w-full" />
                </div>
              ))}
            </div>
          ) : sortedEnvironmentHealth.length === 0 ? (
            <div className="text-center py-10 text-muted-foreground">
              <ShieldCheck className="w-10 h-10 mx-auto mb-3 opacity-70" />
              <p>No environments registered yet.</p>
              <Link to="/ui/environments/new">
                <Button variant="outline" size="sm" className="mt-3">
                  Create Environment
                </Button>
              </Link>
            </div>
          ) : (
            <div className="grid gap-4 lg:grid-cols-2">
              {sortedEnvironmentHealth.map((entry) => {
                const webhookMeta = getWebhookMeta(entry)
                const tokenMeta = getTokenMeta(entry)
                const WebhookIcon = webhookMeta.icon
                const TokenIcon = tokenMeta.icon
                return (
                  <div
                    key={entry.environment_id}
                    className="rounded-lg border border-border/40 bg-card/70 p-4 shadow-sm space-y-4"
                  >
                    <div className="flex items-start justify-between gap-3">
                      <div>
                        <Link
                          to={`/ui/environments/${entry.environment_id}`}
                          className="text-base font-semibold hover:underline"
                        >
                          {entry.environment_name}
                        </Link>
                        <p className="text-sm text-muted-foreground">
                          {getProviderLabel(entry.provider)}
                        </p>
                      </div>
                      <Badge variant="outline" className="uppercase">
                        {entry.provider}
                      </Badge>
                    </div>

                    <div className="grid gap-4 sm:grid-cols-2">
                      <div className="flex items-start gap-3">
                        <div className={`flex h-9 w-9 items-center justify-center rounded-full ${webhookMeta.tone}`}>
                          <WebhookIcon className="h-4 w-4" />
                        </div>
                        <div>
                          <p className="font-medium text-foreground">{webhookMeta.label}</p>
                          <p className="text-sm text-muted-foreground">{webhookMeta.description}</p>
                          {webhookMeta.footnote && (
                            <p className="mt-1 text-xs text-muted-foreground break-words">
                              {webhookMeta.footnote}
                            </p>
                          )}
                        </div>
                      </div>

                      <div className="flex items-start gap-3">
                        <div className={`flex h-9 w-9 items-center justify-center rounded-full ${tokenMeta.tone}`}>
                          <TokenIcon className="h-4 w-4" />
                        </div>
                        <div>
                          <p className="font-medium text-foreground">{tokenMeta.label}</p>
                          <p className="text-sm text-muted-foreground">{tokenMeta.description}</p>
                          {tokenMeta.footnote && (
                            <p className="mt-1 text-xs text-muted-foreground break-words">
                              {tokenMeta.footnote}
                            </p>
                          )}
                        </div>
                      </div>
                    </div>

                    <div className="flex flex-wrap items-center justify-between gap-3 text-xs text-muted-foreground">
                      <span>
                        Last deploy:{' '}
                        {entry.deployed_commit ? entry.deployed_commit.substring(0, 8) : 'Not deployed'}
                      </span>
                      <Link
                        to={`/ui/environments/${entry.environment_id}`}
                        className="flex items-center gap-1 font-medium text-primary hover:underline"
                      >
                        View details
                        <ArrowUpRight className="w-3 h-3" />
                      </Link>
                    </div>
                  </div>
                )
              })}
            </div>
          )}
        </CardContent>
      </Card>
    </div>
  )
}
