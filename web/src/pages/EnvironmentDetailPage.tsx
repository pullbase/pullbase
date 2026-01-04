import { useState, useEffect } from 'react'
import { useParams, useNavigate } from 'react-router-dom'
import { environmentsApi, type Environment, type CommitInfo, type RollbackEvent } from '../lib/api'
import { Button } from '../components/ui/button'
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '../components/ui/card'
import { Badge } from '../components/ui/badge'
import { Input } from '../components/ui/input'
import { Label } from '../components/ui/label'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
  DialogTrigger,
} from '../components/ui/dialog'
import {
  ArrowLeft,
  GitBranch,
  Settings,
  ExternalLink,
  Webhook,
  GitCommit,
  Save,
  ShieldCheck,
  AlertTriangle,
  Clock,
  RotateCcw,
  History
} from 'lucide-react'
import { formatDistanceToNow } from 'date-fns'
import { toast } from 'sonner'
import { cn } from '../lib/utils'
import { useAuth } from '../contexts/auth-context'
import { ValidationStatusBadge } from '../components/ValidationStatusBadge'

export default function EnvironmentDetailPage() {
  const params = useParams()
  const navigate = useNavigate()
  const { user } = useAuth()
  const isViewer = user?.role === 'viewer'
  const canEdit = !isViewer
  const environmentId = params.id as string

  const [environment, setEnvironment] = useState<Environment | null>(null)
  const [isLoading, setIsLoading] = useState(true)
  const [isEditing, setIsEditing] = useState(false)
  const [isSaving, setIsSaving] = useState(false)
  
  const [isRollbackOpen, setIsRollbackOpen] = useState(false)
  const [commits, setCommits] = useState<CommitInfo[]>([])
  const [selectedCommit, setSelectedCommit] = useState('')
  const [rollbackReason, setRollbackReason] = useState('')
  const [rollbackHistory, setRollbackHistory] = useState<RollbackEvent[]>([])
  const [isRollbackLoading, setIsRollbackLoading] = useState(false)
  const [isLoadingCommits, setIsLoadingCommits] = useState(false)

  const [editForm, setEditForm] = useState({
    name: '',
    repo_url: '',
    branch: '',
    deploy_path: '',
    installation_id: '',
    app_slug: '',
    repository_id: '',
    notification_webhook_url: '',
    webhook_secret: '',
    auto_reconcile: false
  })

  useEffect(() => {
    if (isViewer && isEditing) {
      setIsEditing(false)
    }
  }, [isViewer, isEditing])

  const loadCommits = async () => {
    if (!environmentId) return
    try {
      setIsLoadingCommits(true)
      const data = await environmentsApi.getCommits(parseInt(environmentId))
      setCommits(data.commits ?? [])
    } catch (error) {
      console.error('Failed to load commits:', error)
      toast.error('Failed to load available commits')
    } finally {
      setIsLoadingCommits(false)
    }
  }

  const loadRollbackHistory = async () => {
    if (!environmentId) return
    try {
      const data = await environmentsApi.getRollbacks(parseInt(environmentId))
      setRollbackHistory(data.rollbacks ?? [])
    } catch (error) {
      console.error('Failed to load rollback history:', error)
    }
  }

  const handleRollback = async () => {
    if (!selectedCommit) {
      toast.error('Please select a commit to rollback to')
      return
    }

    try {
      setIsRollbackLoading(true)
      await environmentsApi.initiateRollback(parseInt(environmentId), {
        to_commit: selectedCommit,
        reason: rollbackReason || undefined
      })
      toast.success('Rollback initiated successfully')
      setIsRollbackOpen(false)
      loadEnvironment()
      loadRollbackHistory()
      setRollbackReason('')
      setSelectedCommit('')
    } catch (error) {
      console.error('Rollback failed:', error)
      toast.error('Failed to initiate rollback')
    } finally {
      setIsRollbackLoading(false)
    }
  }

  const loadEnvironment = async () => {
    // Validate environment ID
    if (!environmentId) {
      toast.error('Invalid environment ID')
      navigate('/ui/environments')
      return
    }

    const parsedId = parseInt(environmentId, 10)
    if (isNaN(parsedId) || parsedId <= 0) {
      toast.error('Invalid environment ID format')
      navigate('/ui/environments')
      return
    }

    try {
      setIsLoading(true)
      const data = await environmentsApi.get(parsedId)
      setEnvironment(data)
      setEditForm({
        name: data.name || '',
        repo_url: data.repo_url || '',
        branch: data.branch || 'main',
        deploy_path: data.deploy_path || 'config.yaml',
        installation_id: data.installation_id?.toString() || '',
        app_slug: data.app_slug || '',
        repository_id: data.repository_id?.toString() || '',
        notification_webhook_url: data.notification_webhook_url || '',
        webhook_secret: '',
        auto_reconcile: data.auto_reconcile || false
      })
      
      loadRollbackHistory()
    } catch (error: unknown) {
      console.error('EnvironmentDetailPage - API error:', error)
      if (
        error &&
        typeof error === 'object' &&
        'response' in error &&
        error.response &&
        typeof error.response === 'object' &&
        'status' in error.response &&
        (error.response as { status?: number }).status === 404
      ) {
        toast.error('Environment not found')
        navigate('/ui/environments')
      } else {
        toast.error('Failed to load environment details')
      }
    } finally {
      setIsLoading(false)
    }
  }

  const handleToggleAutoReconcile = async () => {
    if (!environment) return
    if (isViewer) {
      toast.error('Viewer role cannot modify auto-reconcile')
      return
    }
    
    try {
      const result = await environmentsApi.toggleAutoReconcile(environment.id)
      setEnvironment({ ...environment, auto_reconcile: result.auto_reconcile })
      setEditForm(prev => ({ ...prev, auto_reconcile: result.auto_reconcile }))
      toast.success(result.message)
    } catch (error) {
      console.error('Failed to toggle auto-reconcile:', error)
      toast.error('Failed to toggle auto-reconcile')
    }
  }

  const handleSave = async () => {
    if (!environment || isViewer) {
      toast.error('Viewer role cannot update environments')
      return
    }
    
    try {
      setIsSaving(true)
      const installationId = parseInt(editForm.installation_id, 10)
      if (isNaN(installationId) || installationId <= 0) {
        throw new Error('Installation ID must be a positive number')
      }
      await environmentsApi.update(environment.id, {
        name: editForm.name,
        repo_url: editForm.repo_url,
        branch: editForm.branch || 'main',
        deploy_path: editForm.deploy_path || 'config.yaml',
        installation_id: installationId,
        app_slug: editForm.app_slug || undefined,
        repository_id: editForm.repository_id ? parseInt(editForm.repository_id, 10) || undefined : undefined,
        notification_webhook_url: editForm.notification_webhook_url || undefined,
        webhook_secret: editForm.webhook_secret,
        auto_reconcile: editForm.auto_reconcile
      })
      toast.success('Environment updated successfully')
      setIsEditing(false)
      loadEnvironment()
    } catch (error) {
      console.error('Failed to update environment:', error)
      toast.error('Failed to update environment')
    } finally {
      setIsSaving(false)
    }
  }

  const handleInputChange = (e: React.ChangeEvent<HTMLInputElement>) => {
    if (isViewer) {
      toast.error('Viewer role cannot modify settings')
      return
    }

    const { name, value, type, checked } = e.target
    setEditForm(prev => ({
      ...prev,
      [name]: type === 'checkbox' ? checked : value
    }))
  }

  const formatTimestamp = (timestamp?: string) => {
    if (!timestamp) return 'Never'
    try {
      return formatDistanceToNow(new Date(timestamp), { addSuffix: true })
    } catch {
      return 'Invalid date'
    }
  }

  const getProviderBadge = () => (
    <Badge className="bg-gray-900 hover:bg-gray-800 text-white">GitHub</Badge>
  )

  useEffect(() => {
    loadEnvironment()
  }, [environmentId]) // eslint-disable-line react-hooks/exhaustive-deps

  if (isLoading) {
    return (
      <div className="space-y-6">
        <div className="flex items-center gap-4">
          <div className="h-8 w-8 bg-gray-200 rounded animate-pulse"></div>
          <div className="h-8 w-32 bg-gray-200 rounded animate-pulse"></div>
        </div>
        <div className="grid gap-6 md:grid-cols-2">
          <div className="h-64 bg-gray-200 rounded animate-pulse"></div>
          <div className="h-64 bg-gray-200 rounded animate-pulse"></div>
        </div>
      </div>
    )
  }

  if (!environment) {
    return (
      <div className="text-center py-12">
        <h1 className="text-2xl font-bold text-foreground">Environment Not Found</h1>
        <p className="text-muted-foreground mt-2">The environment you&apos;re looking for doesn&apos;t exist or could not be loaded.</p>
        <Button className="mt-4" onClick={() => navigate('/ui/environments')}>
          Back to Environments
        </Button>
      </div>
    )
  }

  const webhookStatus = environment.webhook_status
  const integrationStatus = (webhookStatus?.status || (environment.installation_id ? 'inactive' : 'not configured')).toLowerCase()
  const statusStyles: Record<string, string> = {
    active: 'bg-emerald-100 text-emerald-700 border border-emerald-200',
    error: 'bg-red-100 text-red-700 border border-red-200',
    fallback: 'bg-amber-100 text-amber-800 border border-amber-200',
    pending: 'bg-blue-100 text-blue-700 border border-blue-200',
  }

  const statusLabel = webhookStatus?.status
    ? webhookStatus.status.charAt(0).toUpperCase() + webhookStatus.status.slice(1)
    : environment.installation_id
      ? 'Inactive'
      : 'Not configured'

  const statusBadgeClass = cn(
    'px-2 py-1 rounded-full text-xs font-medium capitalize',
    statusStyles[integrationStatus] || 'bg-muted text-muted-foreground border border-border'
  )
  const lastWebhookDisplay = formatTimestamp(environment.webhook_status?.last_webhook)
  const deployedBadgeLabel = environment.deployed_commit
    ? `Deployed ${environment.deployed_commit.substring(0, 8)}`
    : 'No deployment yet'

  return (
    <div className="space-y-6">
      {/* Header */}
      <div className="flex items-center justify-between">
        <div className="flex items-center gap-4">
          <Button variant="ghost" size="sm" onClick={() => navigate('/ui/environments')}>
            <ArrowLeft className="w-4 h-4" />
          </Button>
          <div>
            <h1 className="text-3xl font-bold text-foreground">{environment.name}</h1>
            <p className="text-muted-foreground mt-1">Environment configuration and settings</p>
          </div>
        </div>
        <div className="flex gap-2">
          {!isEditing && !isViewer && (
            <Dialog open={isRollbackOpen} onOpenChange={(open) => {
              setIsRollbackOpen(open)
              if (open) loadCommits()
            }}>
              <DialogTrigger asChild>
                <Button variant="outline" className="gap-2 border-amber-200 hover:bg-amber-50 text-amber-700">
                  <RotateCcw className="w-4 h-4" />
                  Rollback
                </Button>
              </DialogTrigger>
              <DialogContent className="sm:max-w-[500px]">
                <DialogHeader>
                  <DialogTitle>Rollback Environment</DialogTitle>
                  <DialogDescription>
                    Revert this environment to a previous commit. This will trigger a deployment.
                  </DialogDescription>
                </DialogHeader>
                
                <div className="space-y-4 py-4">
                  <div className="space-y-2">
                    <Label htmlFor="commit">Select Commit</Label>
                    <select
                      id="commit"
                      className="flex h-10 w-full rounded-md border border-input bg-background px-3 py-2 text-sm ring-offset-background focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring focus-visible:ring-offset-2 disabled:cursor-not-allowed disabled:opacity-50"
                      value={selectedCommit}
                      onChange={(e) => setSelectedCommit(e.target.value)}
                      disabled={isLoadingCommits}
                    >
                      <option value="" disabled>Select a commit...</option>
                      {commits.map((commit) => (
                        <option key={commit.hash} value={commit.hash}>
                          {commit.hash.substring(0, 8)} - {commit.message.substring(0, 40)}{commit.message.length > 40 ? '...' : ''} ({formatTimestamp(commit.applied_at)})
                        </option>
                      ))}
                    </select>
                    {isLoadingCommits && <p className="text-xs text-muted-foreground">Loading commits...</p>}
                  </div>
                  
                  <div className="space-y-2">
                    <Label htmlFor="reason">Reason (Optional)</Label>
                    <Input
                      id="reason"
                      placeholder="e.g., Fix critical bug in v2.1"
                      value={rollbackReason}
                      onChange={(e) => setRollbackReason(e.target.value)}
                    />
                  </div>

                  <div className="rounded-md bg-amber-50 p-4 border border-amber-200">
                    <div className="flex">
                      <div className="flex-shrink-0">
                        <AlertTriangle className="h-5 w-5 text-amber-600" aria-hidden="true" />
                      </div>
                      <div className="ml-3">
                        <h3 className="text-sm font-medium text-amber-800">Impact Warning</h3>
                        <div className="mt-2 text-sm text-amber-700">
                          <p>
                            Rolling back will force all servers in this environment to apply the selected configuration. 
                            This action cannot be undone, but you can roll forward by deploying a newer commit later.
                          </p>
                        </div>
                      </div>
                    </div>
                  </div>
                </div>

                <DialogFooter>
                  <Button variant="outline" onClick={() => setIsRollbackOpen(false)}>Cancel</Button>
                  <Button 
                    onClick={handleRollback} 
                    disabled={!selectedCommit || isRollbackLoading}
                    className="gap-2 bg-amber-600 hover:bg-amber-700"
                  >
                    {isRollbackLoading ? (
                      <>
                        <RotateCcw className="w-4 h-4 animate-spin" />
                        Rolling back...
                      </>
                    ) : (
                      <>
                        <RotateCcw className="w-4 h-4" />
                        Confirm Rollback
                      </>
                    )}
                  </Button>
                </DialogFooter>
              </DialogContent>
            </Dialog>
          )}

          {isEditing ? (
            <>
              <Button variant="outline" onClick={() => setIsEditing(false)} disabled={isSaving}>
                Cancel
              </Button>
              <Button onClick={handleSave} disabled={isSaving} className="gap-2">
                <Save className="w-4 h-4" />
                {isSaving ? 'Saving...' : 'Save Changes'}
              </Button>
            </>
          ) : (
            <Button
              onClick={() => {
                if (isViewer) {
                  toast.error('Viewer role has read-only access to environments')
                  return
                }
                setIsEditing(true)
              }}
              className="gap-2"
              disabled={isViewer}
              variant={isViewer ? 'outline' : undefined}
              title={isViewer ? 'Viewer role cannot edit settings' : undefined}
            >
              <Settings className="w-4 h-4" />
              {isViewer ? 'View Only' : 'Edit Settings'}
            </Button>
          )}
        </div>
      </div>
      <div className="flex flex-wrap items-center gap-2 text-xs sm:text-sm text-muted-foreground">
        <Badge variant={environment.auto_reconcile ? 'default' : 'secondary'} className="flex items-center gap-2">
          <ShieldCheck className="w-3 h-3" />
          Auto-reconcile: {environment.auto_reconcile ? 'Enabled' : 'Disabled'}
        </Badge>
        <Badge variant="secondary" className="flex items-center gap-2">
          <Clock className="w-3 h-3" />
          Last webhook: {lastWebhookDisplay}
        </Badge>
        <Badge variant="outline" className="font-mono">
          {deployedBadgeLabel}
        </Badge>
        <ValidationStatusBadge status="unknown" />
      </div>

      {/* Environment Overview */}
      <div className="grid gap-6 md:grid-cols-2 xl:grid-cols-3">
        <Card>
          <CardHeader>
            <CardTitle className="flex items-center gap-2">
              <GitBranch className="w-5 h-5" />
              Repository Configuration
            </CardTitle>
          </CardHeader>
          <CardContent className="space-y-4">
            {isEditing ? (
              <>
                <div>
                  <Label htmlFor="name">Environment Name</Label>
                  <Input
                    id="name"
                    name="name"
                    value={editForm.name}
                    onChange={handleInputChange}
                    disabled={isSaving}
                  />
                </div>
                
                <div>
                  <Label htmlFor="repo_url">Repository URL</Label>
                  <Input
                    id="repo_url"
                    name="repo_url"
                    value={editForm.repo_url}
                    onChange={handleInputChange}
                    disabled={isSaving}
                  />
                </div>
                <div>
                  <Label htmlFor="branch">Branch</Label>
                  <Input
                    id="branch"
                    name="branch"
                    value={editForm.branch}
                    onChange={handleInputChange}
                    disabled={isSaving}
                  />
                </div>
                <div>
                  <Label htmlFor="deploy_path">Config Path</Label>
                  <Input
                    id="deploy_path"
                    name="deploy_path"
                    value={editForm.deploy_path}
                    onChange={handleInputChange}
                    disabled={isSaving}
                  />
                </div>
                
              </>
            ) : (
              <>
                <div>
                  <label className="text-sm font-medium text-foreground">Repository</label>
                  <div className="flex items-center gap-2 mt-1">
                    <GitBranch className="w-4 h-4 text-muted-foreground" />
                    <span className="font-mono text-sm">
                      {environment.repo_url || 'N/A'}
                    </span>
                    {environment.repo_url && (
                      <Button variant="ghost" size="sm" asChild>
                        <a href={environment.repo_url} target="_blank" rel="noopener noreferrer">
                          <ExternalLink className="w-3 h-3" />
                        </a>
                      </Button>
                    )}
                  </div>
                </div>

                <div className="grid grid-cols-2 gap-4">
                  <div>
                    <label className="text-sm font-medium text-foreground">Branch</label>
                    <div className="mt-1 font-mono text-sm text-muted-foreground">
                      {environment.branch}
                    </div>
                  </div>
                  <div>
                    <label className="text-sm font-medium text-foreground">Config Path</label>
                    <div className="mt-1 flex items-center gap-2">
                      <div className="font-mono text-sm text-muted-foreground">
                        {environment.deploy_path}
                      </div>
                      {environment.repo_url && (
                        <Button variant="ghost" size="sm" className="h-6 gap-1 px-2 text-xs" asChild>
                          <a
                            href={`${environment.repo_url}/blob/${environment.branch}/${environment.deploy_path}`}
                            target="_blank"
                            rel="noopener noreferrer"
                          >
                            View Config
                            <ExternalLink className="w-3 h-3" />
                          </a>
                        </Button>
                      )}
                    </div>
                  </div>
                </div>

                {environment.notification_webhook_url && (
                  <div>
                    <label className="text-sm font-medium text-foreground">Notification Webhook URL</label>
                    <div className="mt-1 font-mono text-sm text-muted-foreground break-all">
                      {environment.notification_webhook_url}
                    </div>
                  </div>
                )}

                <div>
                  <label className="text-sm font-medium text-foreground">Provider</label>
                  <div className="mt-1">
                    {getProviderBadge()}
                  </div>
                </div>

                <div>
                  <label className="text-sm font-medium text-foreground">Installation ID</label>
                  <div className="mt-1 text-sm text-muted-foreground">
                    {environment.installation_id}
                  </div>
                </div>
                {environment.app_slug && (
                  <div>
                    <label className="text-sm font-medium text-foreground">App Slug</label>
                    <div className="mt-1 text-sm text-muted-foreground">
                      {environment.app_slug}
                    </div>
                  </div>
                )}
                {environment.repository_id && (
                  <div>
                    <label className="text-sm font-medium text-foreground">Repository ID</label>
                    <div className="mt-1 text-sm text-muted-foreground">
                      {environment.repository_id}
                    </div>
                  </div>
                )}

                <div>
                  <label className="text-sm font-medium text-foreground">Auto Reconcile</label>
                  <div className="mt-1">
                    <Badge variant={environment.auto_reconcile ? 'default' : 'secondary'}>
                      {environment.auto_reconcile ? 'Enabled' : 'Disabled'}
                    </Badge>
                  </div>
                </div>
              </>
            )}
          </CardContent>
        </Card>

        <Card className="xl:col-span-1">
          <CardHeader>
            <CardTitle className="flex items-center gap-2">
              <Webhook className="w-5 h-5" />
              Deployment Status
            </CardTitle>
          </CardHeader>
          <CardContent className="space-y-4">
            <div>
              <label className="text-sm font-medium text-foreground">Last Webhook</label>
              <div className="mt-1 text-sm text-muted-foreground">
                {formatTimestamp(environment.webhook_status?.last_webhook)}
              </div>
            </div>

            {environment.deployed_commit && (
              <div>
                <label className="text-sm font-medium text-foreground">Deployed Commit</label>
                <div className="flex items-center gap-2 mt-1">
                  <GitCommit className="w-4 h-4 text-muted-foreground" />
                  <code className="bg-muted px-2 py-1 rounded text-sm">
                    {environment.deployed_commit.substring(0, 8)}
                  </code>
                  {environment.repo_url && (
                    <Button variant="ghost" size="sm" asChild>
                      <a 
                        href={`${environment.repo_url}/commit/${environment.deployed_commit}`} 
                        target="_blank" 
                        rel="noopener noreferrer"
                      >
                        <ExternalLink className="w-3 h-3" />
                      </a>
                    </Button>
                  )}
                </div>
              </div>
            )}

            <div>
              <label className="text-sm font-medium text-foreground">Webhook URL</label>
              <div className="mt-1">
                <code className="bg-muted px-2 py-1 rounded text-sm break-all">
                  {window.location.origin}/webhooks/github
                </code>
              </div>
            </div>
          </CardContent>
        </Card>

        <Card>
          <CardHeader>
            <CardTitle className="flex items-center gap-2">
              <ShieldCheck className="w-5 h-5" />
              GitHub App Integration
            </CardTitle>
            <CardDescription>
              Connection details for the GitHub App associated with this environment
            </CardDescription>
          </CardHeader>
          <CardContent className="space-y-4">
            <div className="flex items-start justify-between gap-4">
              <div>
                <p className="text-sm font-medium text-foreground">Integration status</p>
                <p className="text-sm text-muted-foreground mt-1">
                  {integrationStatus === 'active'
                    ? 'Webhook events are flowing successfully.'
                    : integrationStatus === 'error'
                      ? 'The controller is retrying webhook registration.'
                      : integrationStatus === 'fallback'
                        ? 'Webhook registration failed; monitor retry attempts.'
                        : 'Complete the bootstrap wizard to finish configuring this environment.'}
                </p>
              </div>
              <span className={statusBadgeClass}>{statusLabel}</span>
            </div>

            <div className="grid grid-cols-1 sm:grid-cols-2 gap-4">
              <div>
                <p className="text-xs uppercase text-muted-foreground tracking-wide">Installation ID</p>
                <p className="mt-1 text-sm font-mono text-foreground">
                  {environment.installation_id || '—'}
                </p>
              </div>
              <div>
                <p className="text-xs uppercase text-muted-foreground tracking-wide">App slug</p>
                <p className="mt-1 text-sm font-mono text-foreground">
                  {environment.app_slug || '—'}
                </p>
              </div>
              <div>
                <p className="text-xs uppercase text-muted-foreground tracking-wide">Repository ID</p>
                <p className="mt-1 text-sm font-mono text-foreground">
                  {environment.repository_id || '—'}
                </p>
              </div>
              <div>
                <p className="text-xs uppercase text-muted-foreground tracking-wide">Last webhook</p>
                <p className="mt-1 text-sm text-foreground">
                  {webhookStatus?.last_webhook ? formatTimestamp(webhookStatus.last_webhook) : 'Never'}
                </p>
              </div>
            </div>

            {(webhookStatus?.retry_count ?? 0) > 0 && (
              <div className="flex items-center gap-2 rounded border border-amber-200 bg-amber-50 px-3 py-2">
                <AlertTriangle className="w-4 h-4 text-amber-600" />
                <p className="text-sm text-amber-800">
                  Webhook retries: {webhookStatus?.retry_count}. Consider re-running the bootstrap wizard or checking GitHub App permissions.
                </p>
              </div>
            )}

            {webhookStatus?.error && (
              <div className="rounded border border-red-200 bg-red-50 px-3 py-2 text-sm text-red-700">
                {webhookStatus.error}
              </div>
            )}
          </CardContent>
        </Card>
      </div>

      {/* Security Settings */}
      {canEdit && isEditing && (
        <Card>
          <CardHeader>
            <CardTitle className="flex items-center gap-2">
              <Settings className="w-5 h-5" />
              GitHub App Configuration
            </CardTitle>
            <CardDescription>
              Update installation details or webhook secret. Leave the secret blank to keep the existing value.
            </CardDescription>
          </CardHeader>
          <CardContent className="space-y-4">
            <div className="grid gap-4 md:grid-cols-2">
              <div className="space-y-2">
                <Label htmlFor="installation_id">GitHub Installation ID</Label>
                <Input
                  id="installation_id"
                  name="installation_id"
                  type="number"
                  value={editForm.installation_id}
                  onChange={handleInputChange}
                  disabled={isSaving}
                  placeholder="12345678"
                />
              </div>
              <div className="space-y-2">
                <Label htmlFor="app_slug">GitHub App Slug</Label>
                <Input
                  id="app_slug"
                  name="app_slug"
                  value={editForm.app_slug}
                  onChange={handleInputChange}
                  disabled={isSaving}
                  placeholder="Optional"
                />
              </div>
              <div className="space-y-2">
                <Label htmlFor="repository_id">Repository ID</Label>
                <Input
                  id="repository_id"
                  name="repository_id"
                  type="number"
                  value={editForm.repository_id}
                  onChange={handleInputChange}
                  disabled={isSaving}
                  placeholder="Optional"
                />
              </div>
              <div className="space-y-2">
                <Label htmlFor="webhook_secret">Webhook Secret</Label>
                <Input
                  id="webhook_secret"
                  name="webhook_secret"
                  type="password"
                  value={editForm.webhook_secret}
                  onChange={handleInputChange}
                  disabled={isSaving}
                  placeholder="Leave blank to keep existing"
                />
              </div>
              <div className="space-y-2">
                <Label htmlFor="notification_webhook_url">Notification Webhook URL</Label>
                <Input
                  id="notification_webhook_url"
                  name="notification_webhook_url"
                  type="url"
                  value={editForm.notification_webhook_url}
                  onChange={handleInputChange}
                  disabled={isSaving}
                  placeholder="https://example.com/webhook"
                />
                <p className="text-xs text-muted-foreground">
                  Receive notifications for drift, errors, and agent disconnections
                </p>
              </div>
            </div>

            <div className="flex items-center gap-2">
              <input
                id="auto_reconcile"
                name="auto_reconcile"
                type="checkbox"
                checked={editForm.auto_reconcile}
                onChange={handleToggleAutoReconcile}
                disabled={isSaving || isViewer}
                className="h-4 w-4"
              />
              <Label htmlFor="auto_reconcile">Enable Auto-Reconcile</Label>
            </div>

            <div className="pt-4 border-t">
              <Button onClick={handleSave} disabled={isSaving} className="gap-2">
                <Save className="w-4 h-4" />
                {isSaving ? 'Saving...' : 'Save Changes'}
              </Button>
            </div>
          </CardContent>
        </Card>
      )}
      
      <Card>
        <CardHeader>
          <CardTitle className="flex items-center gap-2">
            <History className="w-5 h-5" />
            Rollback History
          </CardTitle>
          <CardDescription>
            History of rollback operations performed on this environment
          </CardDescription>
        </CardHeader>
        <CardContent>
          {rollbackHistory.length === 0 ? (
            <div className="text-center py-8 text-muted-foreground">
              No rollbacks recorded
            </div>
          ) : (
            <div className="space-y-4">
              {rollbackHistory.map((rollback) => (
                <div key={rollback.id} className="flex items-center justify-between border-b pb-4 last:border-0 last:pb-0">
                  <div className="space-y-1">
                    <div className="flex items-center gap-2">
                      <span className="font-medium text-sm">
                        {rollback.from_commit.substring(0, 8)} <span className="text-muted-foreground">→</span> {rollback.to_commit.substring(0, 8)}
                      </span>
                      <Badge variant={
                        rollback.status === 'completed' ? 'default' : 
                        rollback.status === 'failed' ? 'destructive' : 'secondary'
                      } className="capitalize">
                        {rollback.status.replace('_', ' ')}
                      </Badge>
                    </div>
                    <p className="text-xs text-muted-foreground">
                      Initiated by {rollback.initiated_by} • {formatTimestamp(rollback.created_at)}
                    </p>
                    {rollback.reason && (
                      <p className="text-sm text-muted-foreground italic">"{rollback.reason}"</p>
                    )}
                  </div>
                  {rollback.error_message && (
                    <div className="text-sm text-red-600 max-w-[300px] truncate" title={rollback.error_message}>
                      Error: {rollback.error_message}
                    </div>
                  )}
                </div>
              ))}
            </div>
          )}
        </CardContent>
      </Card>
    </div>
  )
} 
