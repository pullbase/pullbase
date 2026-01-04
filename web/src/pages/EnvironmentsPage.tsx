import { useState, useEffect, useRef, useCallback, useMemo } from 'react'
import { environmentsApi, type Environment, type PaginationResponse } from '../lib/api'
import { Button } from '../components/ui/button'
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '../components/ui/card'
import { Badge } from '../components/ui/badge'
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from '../components/ui/table'
import { Skeleton } from '../components/ui/skeleton'
import { Input } from '../components/ui/input'
import { 
  GitBranch, 
  ExternalLink, 
  Settings,
  Trash2,
  Plus,
  RotateCcw,
  GitCommit,
  Clock,
  Webhook
} from 'lucide-react'
import { formatDistanceToNow } from 'date-fns'
import { toast } from 'sonner'
import {
  AlertDialog,
  AlertDialogAction,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle,
  AlertDialogTrigger,
} from '../components/ui/alert-dialog'
import { useNavigate } from 'react-router-dom'
import { useAuth } from '../contexts/auth-context'

export default function EnvironmentsPage() {
  const [environmentsData, setEnvironmentsData] = useState<PaginationResponse<Environment> | null>(null)
  const [isLoading, setIsLoading] = useState(true)
  const [currentPage, setCurrentPage] = useState(1)
  const [isAutoRefreshEnabled, setIsAutoRefreshEnabled] = useState(true)
  const [isRefreshing, setIsRefreshing] = useState(false)
  const intervalRef = useRef<number | null>(null)
  const [searchTerm, setSearchTerm] = useState('')
  const [statusFilter, setStatusFilter] = useState<'all' | 'auto_on' | 'auto_off' | 'webhook_error'>('all')
  const [sortKey, setSortKey] = useState<'recent' | 'name' | 'webhook'>('recent')
  const [rollbackDialog, setRollbackDialog] = useState<{
    open: boolean
    environment: Environment | null
    commits: Array<{
      hash?: string
      id?: string
      message?: string
      author?: string
      timestamp?: string
    }>
    isLoadingCommits: boolean
    selectedCommit: string
    reason: string
    isRollingBack: boolean
  }>({
    open: false,
    environment: null,
    commits: [],
    isLoadingCommits: false,
    selectedCommit: '',
    reason: '',
    isRollingBack: false
  })
  const navigate = useNavigate()
  const { user } = useAuth()
  const isViewer = user?.role === 'viewer'

  const loadEnvironments = useCallback(async (page = 1) => {
    try {
      setIsLoading(true)
      const data = await environmentsApi.list(page, 10, sortKey)
      setEnvironmentsData(data)
      setCurrentPage(page)
    } catch (error) {
      console.error('Failed to load environments:', error)
      toast.error('Failed to load environments')
    } finally {
      setIsLoading(false)
    }
  }, [sortKey])

  const refreshEnvironments = useCallback(async () => {
    try {
      setIsRefreshing(true)
      const data = await environmentsApi.list(currentPage, 10, sortKey)
      setEnvironmentsData(data)
    } catch (error) {
      console.error('Failed to refresh environments:', error)
    } finally {
      setIsRefreshing(false)
    }
  }, [currentPage, sortKey])

  // Manual refresh function
  const handleManualRefresh = async () => {
    await refreshEnvironments()
    toast.success('Environments refreshed successfully')
  }

  const handleDeleteEnvironment = async (id: number) => {
    try {
      await environmentsApi.delete(id)
      toast.success('Environment deleted successfully')
      loadEnvironments(currentPage)
    } catch (error) {
      console.error('Failed to delete environment:', error)
      toast.error('Failed to delete environment')
    }
  }

  const handleOpenRollback = async (environment: Environment) => {
    setRollbackDialog(prev => ({
      ...prev,
      open: true,
      environment,
      isLoadingCommits: true,
      selectedCommit: '',
      reason: ''
    }))

    try {
      const response = await environmentsApi.getCommits(environment.id)
      setRollbackDialog(prev => ({
        ...prev,
        commits: response.commits.map((commit) => ({
          hash: commit.hash,
          id: commit.hash, 
          message: commit.message,
          author: 'System',
          timestamp: commit.applied_at 
        })),
        isLoadingCommits: false
      }))
    } catch (error) {
      console.error('Failed to load commits:', error)
      toast.error('Failed to load commit history')
      setRollbackDialog(prev => ({
        ...prev,
        commits: [],
        isLoadingCommits: false
      }))
    }
  }

  const handleRollback = async () => {
    if (!rollbackDialog.environment || !rollbackDialog.selectedCommit) {
      return
    }

    setRollbackDialog(prev => ({ ...prev, isRollingBack: true }))

    try {
      await environmentsApi.initiateRollback(rollbackDialog.environment.id, {
        to_commit: rollbackDialog.selectedCommit,
        reason: rollbackDialog.reason || 'Rollback initiated from UI'
      })

      toast.success(
        <div className="space-y-1">
          <div className="font-medium">Rollback initiated successfully!</div>
          <div className="text-sm text-muted-foreground">
            Environment target updated to commit {rollbackDialog.selectedCommit.substring(0, 8)}
          </div>
          <div className="text-xs text-muted-foreground">
            Agents will apply this change during their next check-in
          </div>
        </div>
      )
      setRollbackDialog({
        open: false,
        environment: null,
        commits: [],
        isLoadingCommits: false,
        selectedCommit: '',
        reason: '',
        isRollingBack: false
      })
      
      // Refresh environments to show updated deployed_commit
      loadEnvironments(currentPage)
    } catch (error: unknown) {
      console.error('Failed to initiate rollback:', error)
      
      let errorMessage = 'Failed to initiate rollback'
      
      if (error && typeof error === 'object' && 'response' in error) {
        const axiosError = error as { response?: { data?: { error?: string } } }
        if (axiosError.response?.data?.error) {
          errorMessage = axiosError.response.data.error
        }
      }
      
      toast.error(
        <div className="space-y-1">
          <div className="font-medium">Rollback Failed</div>
          <div className="text-sm text-red-600 whitespace-pre-wrap">
            {errorMessage}
          </div>
        </div>
      )
    } finally {
      setRollbackDialog(prev => ({ ...prev, isRollingBack: false }))
    }
  }

  useEffect(() => {
    loadEnvironments()
  }, [loadEnvironments])

  // Auto-refresh effect
  useEffect(() => {
    if (!isAutoRefreshEnabled || isLoading) return

    intervalRef.current = window.setInterval(() => {
      if (!document.hidden) {
        refreshEnvironments()
      }
    }, 30000)

    return () => {
      if (intervalRef.current) {
        window.clearInterval(intervalRef.current)
      }
    }
  }, [currentPage, isAutoRefreshEnabled, isLoading, refreshEnvironments])

  useEffect(() => {
    return () => {
      if (intervalRef.current) {
        window.clearInterval(intervalRef.current)
      }
    }
  }, [])

  const getProviderBadge = () => (
    <Badge className="bg-gray-900 hover:bg-gray-800 text-white">GitHub</Badge>
  )

  const formatTimestamp = (timestamp?: string) => {
    if (!timestamp) return 'Never'
    try {
      return formatDistanceToNow(new Date(timestamp), { addSuffix: true })
    } catch {
      return 'Invalid date'
    }
  }

  const filteredEnvironments = useMemo(() => {
    const list = environmentsData?.data ?? []
    const term = searchTerm.trim().toLowerCase()

    const filtered = list.filter((env) => {
      const matchesSearch =
        term.length === 0 ||
        env.name.toLowerCase().includes(term) ||
        env.repo_url.toLowerCase().includes(term)

      let matchesStatus = true
      if (statusFilter === 'auto_on') {
        matchesStatus = !!env.auto_reconcile
      } else if (statusFilter === 'auto_off') {
        matchesStatus = !env.auto_reconcile
      } else if (statusFilter === 'webhook_error') {
        const status = env.webhook_status?.status?.toLowerCase()
        matchesStatus = status === 'error' || status === 'pending'
      }

      return matchesSearch && matchesStatus
    })

    const sorted = [...filtered]
    if (sortKey === 'name') {
      sorted.sort((a, b) => a.name.localeCompare(b.name))
    } else if (sortKey === 'webhook') {
      sorted.sort((a, b) => {
        const aTime = a.webhook_status?.last_webhook ? new Date(a.webhook_status.last_webhook).getTime() : 0
        const bTime = b.webhook_status?.last_webhook ? new Date(b.webhook_status.last_webhook).getTime() : 0
        return bTime - aTime
      })
    } else {
      sorted.sort((a, b) => {
        const aTime = a.updated_at ? new Date(a.updated_at).getTime() : 0
        const bTime = b.updated_at ? new Date(b.updated_at).getTime() : 0
        return bTime - aTime
      })
    }

    return sorted
  }, [environmentsData, searchTerm, statusFilter, sortKey])

  const hasNoEnvironments =
    !environmentsData || !Array.isArray(environmentsData.data) || environmentsData.data.length === 0
  const hasNoFilteredEnvironments = filteredEnvironments.length === 0

  return (
      <div className="space-y-6">
        {/* Header */}
        <div className="flex justify-between items-center">
          <div>
            <h1 className="text-3xl font-bold text-foreground">Environments</h1>
            <p className="text-muted-foreground mt-1">
              Manage GitOps configuration environments with continuous drift detection
            </p>
            {isViewer && (
              <p className="text-xs text-muted-foreground mt-2">
                Viewer role is read-only. Ask an administrator to make changes.
              </p>
            )}
          </div>
          <div className="flex items-center gap-2">
            <Button
              variant="outline"
              onClick={handleManualRefresh}
              disabled={isRefreshing}
              className="gap-2"
            >
              <RotateCcw className={`w-4 h-4 ${isRefreshing ? 'animate-spin' : ''}`} />
              Refresh
            </Button>

            <Button
              variant="outline"
              onClick={() => setIsAutoRefreshEnabled(!isAutoRefreshEnabled)}
              className={`gap-2 ${isAutoRefreshEnabled ? 'bg-green-50 border-green-200' : ''}`}
              title={isAutoRefreshEnabled ? 'Auto-refresh enabled (30s)' : 'Auto-refresh disabled'}
            >
              <Clock className="w-4 h-4" />
              Auto-refresh: {isAutoRefreshEnabled ? 'On' : 'Off'}
              {isRefreshing && isAutoRefreshEnabled && (
                <span className="text-xs text-green-600">Updating...</span>
              )}
            </Button>

            <Button
              className="gap-2"
              onClick={() => navigate('/ui/environments/new')}
              disabled={isViewer}
              title={isViewer ? 'Viewer role cannot create environments' : undefined}
            >
              <Plus className="w-4 h-4" />
              Add Environment
            </Button>
          </div>
        </div>

        {/* Stats Cards */}
        <div className="grid gap-6 md:grid-cols-3">
          <Card>
            <CardHeader className="flex flex-row items-center justify-between space-y-0 pb-2">
              <CardTitle className="text-sm font-medium">Total Environments</CardTitle>
              <GitBranch className="h-4 w-4 text-muted-foreground" />
            </CardHeader>
            <CardContent>
              <div className="text-2xl font-bold">{environmentsData?.total || 0}</div>
            </CardContent>
          </Card>

          <Card>
            <CardHeader className="flex flex-row items-center justify-between space-y-0 pb-2">
              <CardTitle className="text-sm font-medium">Auto-Reconcile Enabled</CardTitle>
              <Settings className="h-4 w-4 text-muted-foreground" />
            </CardHeader>
            <CardContent>
              <div className="text-2xl font-bold">
                {environmentsData?.data?.filter(env => env.auto_reconcile).length || 0}
              </div>
            </CardContent>
          </Card>

          <Card>
            <CardHeader className="flex flex-row items-center justify-between space-y-0 pb-2">
            <CardTitle className="text-sm font-medium">Webhook Active</CardTitle>
            <Webhook className="h-4 w-4 text-muted-foreground" />
            </CardHeader>
            <CardContent>
              <div className="text-2xl font-bold">
                {environmentsData?.data?.filter(env => env.webhook_status?.status === 'active').length || 0}
              </div>
            </CardContent>
          </Card>
        </div>

        {/* Environments Table */}
        <Card>
          <CardHeader>
            <CardTitle className="flex items-center gap-2">
              <GitBranch className="w-5 h-5" />
              Environment List
              {isRefreshing && (
                <div className="flex items-center gap-1 text-sm text-green-600">
                  <RotateCcw className="w-3 h-3 animate-spin" />
                  <span>Refreshing...</span>
                </div>
              )}
            </CardTitle>
            <CardDescription className="space-y-2">
              <span>
                {filteredEnvironments.length} of {environmentsData?.total || 0} environment
                {(environmentsData?.total || 0) !== 1 ? 's' : ''}
              </span>
              {isAutoRefreshEnabled && (
                <span className="block text-xs text-green-600">
                  Auto-refresh every 30 seconds
                </span>
              )}
            </CardDescription>
          </CardHeader>
          <CardContent>
            <div className="flex flex-col gap-3 md:flex-row md:items-center md:justify-between mb-4">
              <Input
                placeholder="Search by name or repository..."
                value={searchTerm}
                onChange={(e) => setSearchTerm(e.target.value)}
                className="md:w-64"
              />
              <div className="flex flex-col gap-2 sm:flex-row sm:items-center sm:gap-3">
                <div className="flex items-center gap-2">
                  <label className="text-xs uppercase text-muted-foreground">Status</label>
                  <select
                    value={statusFilter}
                    onChange={(e) => setStatusFilter(e.target.value as typeof statusFilter)}
                    className="h-9 rounded-md border border-input bg-background px-2 text-sm"
                  >
                    <option value="all">All</option>
                    <option value="auto_on">Auto-reconcile enabled</option>
                    <option value="auto_off">Auto-reconcile disabled</option>
                    <option value="webhook_error">Webhook issues</option>
                  </select>
                </div>
                <div className="flex items-center gap-2">
                  <label className="text-xs uppercase text-muted-foreground">Sort</label>
                  <select
                    value={sortKey}
                    onChange={(e) => setSortKey(e.target.value as typeof sortKey)}
                    className="h-9 rounded-md border border-input bg-background px-2 text-sm"
                  >
                    <option value="recent">Recently updated</option>
                    <option value="name">Name (A–Z)</option>
                    <option value="webhook">Last webhook</option>
                  </select>
                </div>
              </div>
            </div>
            {isLoading ? (
              <div className="space-y-4">
                {[...Array(5)].map((_, i) => (
                  <div key={i} className="flex items-center space-x-4">
                    <Skeleton className="h-4 w-[150px]" />
                    <Skeleton className="h-4 w-[200px]" />
                    <Skeleton className="h-4 w-[100px]" />
                    <Skeleton className="h-4 w-[80px]" />
                    <Skeleton className="h-4 w-[120px]" />
                  </div>
                ))}
              </div>
            ) : (
              <>
                <Table>
                  <TableHeader>
                    <TableRow>
                      <TableHead>Environment</TableHead>
                      <TableHead>Repository</TableHead>
                      <TableHead>Installation ID</TableHead>
                      <TableHead>Provider</TableHead>
                      <TableHead>Auto Reconcile</TableHead>
                      <TableHead>Webhook Status</TableHead>
                      <TableHead>Last Webhook</TableHead>
                      <TableHead>Deployed Commit</TableHead>
                      <TableHead></TableHead>
                    </TableRow>
                  </TableHeader>
                  <TableBody>
                    {hasNoEnvironments ? (
                      <TableRow>
                        <TableCell colSpan={9} className="text-center py-8">
                          <div className="flex flex-col items-center gap-2">
                            <GitBranch className="w-8 h-8 text-muted-foreground" />
                            <p className="text-muted-foreground">No environments found</p>
                            <p className="text-sm text-muted-foreground">
                            Add your first environment to get started with GitOps configuration management
                            </p>
                          </div>
                        </TableCell>
                      </TableRow>
                    ) : hasNoFilteredEnvironments ? (
                      <TableRow>
                        <TableCell colSpan={9} className="py-12">
                          <div className="flex flex-col items-center gap-3 text-center">
                            <GitBranch className="w-8 h-8 text-muted-foreground" />
                            <p className="text-muted-foreground">No environments match the current filters.</p>
                            <p className="text-sm text-muted-foreground">
                              Adjust your search or filter criteria to see more results.
                            </p>
                          </div>
                        </TableCell>
                      </TableRow>
                    ) : (
                      filteredEnvironments.map((environment) => (
                        <TableRow
                          key={environment.id}
                          className="transition-colors hover:bg-muted/60 dark:hover:bg-muted/20"
                        >
                          <TableCell className="font-medium">
                            <div className="flex items-center gap-2">
                              <GitBranch className="w-4 h-4 text-muted-foreground" />
                              {environment.name}
                            </div>
                          </TableCell>
                          <TableCell>
                            <div className="flex items-center gap-2">
                              <span className="font-mono text-sm text-foreground">
                                {environment.repo_url?.replace('https://github.com/', '') || 'N/A'}
                              </span>
                              {environment.repo_url && (
                                <Button variant="ghost" size="sm" asChild>
                                  <a href={environment.repo_url} target="_blank" rel="noopener noreferrer">
                                    <ExternalLink className="w-3 h-3" />
                                  </a>
                                </Button>
                              )}
                            </div>
                            <div className="text-xs text-muted-foreground mt-1">
                              Branch: <span className="font-mono">{environment.branch}</span> · Config: <span className="font-mono">{environment.deploy_path}</span>
                            </div>
                          </TableCell>
                          <TableCell className="text-sm text-muted-foreground">
                            {environment.installation_id}
                          </TableCell>
                          <TableCell>
                            {getProviderBadge()}
                          </TableCell>
                          <TableCell>
                            <Badge variant={environment.auto_reconcile ? 'default' : 'secondary'} className="dark:bg-muted/30 dark:text-muted-foreground">
                              {environment.auto_reconcile ? 'Enabled' : 'Disabled'}
                            </Badge>
                          </TableCell>
                          <TableCell>
                            <Badge variant={environment.webhook_status?.status === 'active' ? 'default' : 'secondary'} className="dark:bg-muted/30 dark:text-muted-foreground">
                              {environment.webhook_status?.status === 'active' ? 'Active' : 'Inactive'}
                            </Badge>
                          </TableCell>
                          <TableCell className="text-sm text-muted-foreground">
                            {formatTimestamp(environment.webhook_status?.last_webhook)}
                          </TableCell>
                          <TableCell>
                            {environment.deployed_commit ? (
                              <code className="bg-muted px-2 py-1 rounded text-sm text-foreground">
                                {environment.deployed_commit.substring(0, 8)}
                              </code>
                            ) : (
                              <span className="text-muted-foreground">-</span>
                            )}
                          </TableCell>
                          <TableCell>
                            <div className="flex items-center gap-1">
                              {!isViewer && (
                                <Button
                                  variant="ghost"
                                  size="sm"
                                  className="gap-1"
                                  onClick={() => handleOpenRollback(environment)}
                                  disabled={!environment.deployed_commit}
                                >
                                  <RotateCcw className="w-3 h-3" />
                                  Rollback
                                </Button>
                              )}

                              <Button
                                variant="ghost"
                                size="sm"
                                className="gap-1"
                                onClick={() => navigate(`/ui/environments/${environment.id}`)}
                                title={isViewer ? 'View environment details (read-only)' : undefined}
                              >
                                <Settings className="w-3 h-3" />
                                {isViewer ? 'View' : 'Settings'}
                              </Button>

                              {!isViewer && (
                                <AlertDialog>
                                  <AlertDialogTrigger asChild>
                                    <Button variant="ghost" size="sm" className="gap-1 text-red-600 hover:text-red-700">
                                      <Trash2 className="w-3 h-3" />
                                    </Button>
                                  </AlertDialogTrigger>
                                  <AlertDialogContent>
                                    <AlertDialogHeader>
                                      <AlertDialogTitle>Delete Environment</AlertDialogTitle>
                                      <AlertDialogDescription className="text-sm text-muted-foreground leading-relaxed">
                                        Are you sure you want to delete &ldquo;{environment.name}&rdquo;? This action cannot be undone and will remove the environment configuration and stop monitoring its Git repository.
                                      </AlertDialogDescription>
                                    </AlertDialogHeader>
                                    <AlertDialogFooter>
                                      <AlertDialogCancel>Cancel</AlertDialogCancel>
                                      <AlertDialogAction
                                        onClick={() => handleDeleteEnvironment(environment.id)}
                                        className="bg-red-600 hover:bg-red-700"
                                      >
                                        Delete
                                      </AlertDialogAction>
                                    </AlertDialogFooter>
                                  </AlertDialogContent>
                                </AlertDialog>
                              )}
                            </div>
                          </TableCell>
                        </TableRow>
                      ))
                    )}
                  </TableBody>
                </Table>

                {/* Pagination */}
                    {environmentsData && environmentsData.total_pages > 1 && (
                  <div className="flex items-center justify-between mt-4">
                    <p className="text-sm text-muted-foreground">
                      Showing {filteredEnvironments.length} of {environmentsData.total} environments
                    </p>
                    <div className="flex items-center gap-2">
                      <Button
                        variant="outline"
                        size="sm"
                      onClick={() => loadEnvironments(currentPage - 1)}
                        disabled={currentPage <= 1 || isLoading}
                      >
                        Previous
                      </Button>
                      <span className="text-sm text-muted-foreground">
                        Page {currentPage} of {environmentsData.total_pages}
                      </span>
                      <Button
                        variant="outline"
                        size="sm"
                      onClick={() => loadEnvironments(currentPage + 1)}
                        disabled={currentPage >= environmentsData.total_pages || isLoading}
                      >
                        Next
                      </Button>
                    </div>
                  </div>
                )}
              </>
            )}
          </CardContent>
        </Card>

        {/* Rollback Dialog */}
        <AlertDialog
          open={rollbackDialog.open}
          onOpenChange={(open: boolean) =>
            setRollbackDialog(prev => ({ ...prev, open }))
          }
        >
          <AlertDialogContent className="max-w-2xl">
            <AlertDialogHeader>
              <AlertDialogTitle className="flex items-center gap-2">
                <RotateCcw className="w-5 h-5" />
                Rollback Environment: {rollbackDialog.environment?.name}
              </AlertDialogTitle>
              <AlertDialogDescription className="text-sm text-muted-foreground leading-relaxed">
                Select a previous commit to rollback to. This updates the environment's desired state and agents will automatically apply the change.
              </AlertDialogDescription>
            </AlertDialogHeader>
            
            <div className="space-y-4">
              {rollbackDialog.isLoadingCommits ? (
                <div className="space-y-2">
                  <Skeleton className="h-4 w-full" />
                  <Skeleton className="h-4 w-full" />
                  <Skeleton className="h-4 w-full" />
                </div>
              ) : (
                <>
                  <div>
                    <label className="text-sm font-medium">Select Commit</label>
                    <div className="mt-2 space-y-2 max-h-48 overflow-y-auto border rounded p-2">
                      {rollbackDialog.commits.length === 0 ? (
                        <p className="text-sm text-muted-foreground">No commit history available</p>
                      ) : (
                                                 rollbackDialog.commits.map((commit, index: number) => (
                          <div
                            key={commit.hash || index}
                            className={`p-3 border rounded cursor-pointer transition-colors ${
                              rollbackDialog.selectedCommit === (commit.hash || commit.id) 
                                ? 'border-blue-500 bg-blue-50' 
                                : 'border-gray-200 hover:border-gray-300'
                            }`}
                            onClick={() => setRollbackDialog(prev => ({ 
                              ...prev, 
                              selectedCommit: commit.hash || commit.id || ''
                            }))}
                          >
                            <div className="flex items-center gap-2">
                              <GitCommit className="w-4 h-4 text-muted-foreground" />
                              <code className="text-sm bg-muted px-2 py-1 rounded">
                                {(commit.hash || commit.id)?.substring(0, 8)}
                              </code>
                              <span className="text-sm text-muted-foreground">
                                {commit.author} • {commit.timestamp ? 
                                  formatDistanceToNow(new Date(commit.timestamp), { addSuffix: true }) : 
                                  'Unknown time'
                                }
                              </span>
                            </div>
                            {commit.message && (
                              <p className="text-sm mt-1 text-gray-700">{commit.message}</p>
                            )}
                          </div>
                        ))
                      )}
                    </div>
                  </div>
                  
                  <div>
                    <label className="text-sm font-medium">Reason (Optional)</label>
                    <Input
                      placeholder="Enter reason for rollback..."
                      value={rollbackDialog.reason}
                      onChange={(e) => setRollbackDialog(prev => ({ 
                        ...prev, 
                        reason: e.target.value 
                      }))}
                      className="mt-2"
                    />
                  </div>
                </>
              )}
            </div>

            <AlertDialogFooter>
              <AlertDialogCancel disabled={rollbackDialog.isRollingBack}>
                Cancel
              </AlertDialogCancel>
              <AlertDialogAction
                onClick={handleRollback}
                disabled={!rollbackDialog.selectedCommit || rollbackDialog.isRollingBack}
                className="bg-orange-600 hover:bg-orange-700"
              >
                {rollbackDialog.isRollingBack ? 'Rolling back...' : 'Initiate Rollback'}
              </AlertDialogAction>
            </AlertDialogFooter>
          </AlertDialogContent>
        </AlertDialog>
      </div>
  )
} 
