import { useState, useEffect, useRef } from 'react'
import { useParams, useNavigate } from 'react-router-dom'
import { serversApi, type Server, type ServerStatusHistory, type PaginationResponse, type DriftDetailsResponse } from '../lib/api'
import { Button } from '../components/ui/button'
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '../components/ui/card'
import { Badge } from '../components/ui/badge'
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from '../components/ui/table'
import { Skeleton } from '../components/ui/skeleton'
import { Tabs, TabsContent, TabsList, TabsTrigger } from '../components/ui/tabs'
import { SmartTooltip } from '../components/SmartTooltip'
import { 
  ArrowLeft,
  Server as ServerIcon,
  Clock, 
  CheckCircle,
  AlertCircle,
  XCircle,
  RotateCcw, 
  Settings,
  GitCommit,
  Trash2,
  FileDiff,
  Package,
  Layers,
  ChevronRight,
  ChevronDown
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
import ServerTokenManagement from '../components/ServerTokenManagement'
import { useAuth } from '../contexts/auth-context'

export default function ServerDetailPage() {
  const params = useParams()
  const navigate = useNavigate()
  const serverId = params.id as string
  const { user } = useAuth()
  const isViewer = user?.role === 'viewer'

  const [server, setServer] = useState<Server | null>(null)
  const [statusHistory, setStatusHistory] = useState<PaginationResponse<ServerStatusHistory> | null>(null)
  const [driftDetails, setDriftDetails] = useState<DriftDetailsResponse | null>(null)
  const [isLoading, setIsLoading] = useState(true)
  const [isHistoryLoading, setIsHistoryLoading] = useState(true)
  const [isToggling, setIsToggling] = useState(false)
  const [currentHistoryPage, setCurrentHistoryPage] = useState(1)
  const [isAutoRefreshEnabled, setIsAutoRefreshEnabled] = useState(true)
  const [isRefreshing, setIsRefreshing] = useState(false)
  const [expandedFiles, setExpandedFiles] = useState<Record<string, boolean>>({})
  const intervalRef = useRef<number | null>(null)

  const loadServer = async () => {
    try {
      setIsLoading(true)
      const data = await serversApi.get(serverId)
      setServer(data)
      
      if (data?.last_is_drifted) {
        loadDriftDetails()
      }
    } catch (error: unknown) {
      console.error('Failed to load server:', error)
      if (
        error &&
        typeof error === 'object' &&
        'response' in error &&
        error.response &&
        typeof error.response === 'object' &&
        'status' in error.response &&
        (error.response as { status?: number }).status === 404
      ) {
        toast.error('Server not found')
        navigate('/ui/servers')
      } else {
        toast.error('Failed to load server details')
      }
    } finally {
      setIsLoading(false)
    }
  }

  const loadDriftDetails = async () => {
    try {
      const data = await serversApi.getDrift(serverId)
      setDriftDetails(data)
    } catch (error) {
      console.error('Failed to load drift details:', error)
    }
  }

  const loadStatusHistory = async (page = 1) => {
    try {
      setIsHistoryLoading(true)
      const data = await serversApi.getStatusHistory(serverId, page, 20)
      setStatusHistory(data)
      setCurrentHistoryPage(page)
    } catch (error) {
      console.error('Failed to load status history:', error)
      toast.error('Failed to load status history')
    } finally {
      setIsHistoryLoading(false)
    }
  }

  // Silent refresh for auto-refresh (doesn't show loading state)
  const refreshData = async () => {
    try {
      setIsRefreshing(true)
      const [serverData, historyData] = await Promise.all([
        serversApi.get(serverId),
        serversApi.getStatusHistory(serverId, currentHistoryPage, 20)
      ])
      setServer(serverData)
      setStatusHistory(historyData)
      
      if (serverData?.last_is_drifted) {
        loadDriftDetails()
      } else {
        setDriftDetails(null)
      }
    } catch (error) {
      console.error('Failed to refresh data:', error)
      // Don't show toast for auto-refresh errors to avoid spam
    } finally {
      setIsRefreshing(false)
    }
  }

  // Manual refresh function
  const handleManualRefresh = async () => {
    await refreshData()
    toast.success('Data refreshed successfully')
  }

  const handleToggleAutoReconcile = async () => {
    if (!server) return
    if (isViewer) {
      toast.error('Viewer role cannot modify auto-reconcile')
      return
    }

    try {
      setIsToggling(true)
      const result = await serversApi.toggleAutoReconcile(serverId)
      setServer({ ...server, auto_reconcile: result.auto_reconcile })
      toast.success(`Auto-reconcile ${result.auto_reconcile ? 'enabled' : 'disabled'}`)
    } catch (error) {
      console.error('Failed to toggle auto-reconcile:', error)
      toast.error('Failed to toggle auto-reconcile')
    } finally {
      setIsToggling(false)
    }
  }

  const handleDeleteServer = async () => {
    if (isViewer) {
      toast.error('Viewer role cannot delete servers')
      return
    }
    try {
      await serversApi.delete(serverId)
      toast.success('Server deleted successfully')
      navigate('/ui/servers')
    } catch (error) {
      console.error('Failed to delete server:', error)
      toast.error('Failed to delete server')
    }
  }

  useEffect(() => {
    loadServer()
    loadStatusHistory()
  }, [serverId]) // eslint-disable-line react-hooks/exhaustive-deps

  useEffect(() => {
    if (!isAutoRefreshEnabled || isLoading) return

    intervalRef.current = window.setInterval(() => {
      if (!document.hidden) {
        refreshData()
      }
    }, 30000)

    return () => {
      if (intervalRef.current) {
        window.clearInterval(intervalRef.current)
      }
    }
  }, [serverId, isAutoRefreshEnabled, isLoading, currentHistoryPage]) // eslint-disable-line react-hooks/exhaustive-deps

  useEffect(() => {
    return () => {
      if (intervalRef.current) {
        window.clearInterval(intervalRef.current)
      }
    }
  }, [])

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
          <CheckCircle className="w-3 h-3" />
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

  const formatTimestamp = (timestamp?: string) => {
    if (!timestamp) return 'Never'
    try {
      return formatDistanceToNow(new Date(timestamp), { addSuffix: true })
    } catch {
      return 'Invalid date'
    }
  }

  const toggleFileExpand = (path: string) => {
    setExpandedFiles(prev => ({
      ...prev,
      [path]: !prev[path]
    }))
  }

  if (isLoading) {
    return (
      <div className="space-y-6">
        <div className="flex items-center gap-4">
          <Skeleton className="h-8 w-8" />
          <Skeleton className="h-8 w-32" />
        </div>
        <div className="grid gap-6 md:grid-cols-2">
          <Skeleton className="h-64" />
          <Skeleton className="h-64" />
        </div>
      </div>
    )
  }

  if (!server || Object.keys(server).length === 0) {
    return (
      <div className="text-center py-12">
        <h1 className="text-2xl font-bold text-foreground">Server Not Found</h1>
        <p className="text-muted-foreground mt-2">The server you&apos;re looking for doesn&apos;t exist or could not be loaded.</p>
        <Button className="mt-4" onClick={() => navigate('/ui/servers')}>
          Back to Servers
        </Button>
      </div>
    )
  }

  return (
    <div className="space-y-6">
      {/* Header */}
      <div className="flex items-center justify-between">
        <div className="flex items-center gap-4">
          <Button 
            variant="outline" 
            size="sm"
            onClick={() => navigate('/ui/servers')}
            className="gap-2"
          >
            <ArrowLeft className="w-4 h-4" />
            Back
          </Button>
          <div>
            <h1 className="text-3xl font-bold text-foreground flex items-center gap-2">
              <ServerIcon className="w-8 h-8" />
              {server.name || server.id}
            </h1>
            <p className="text-muted-foreground mt-1">Server Configuration</p>
          </div>
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
            variant="outline"
            onClick={handleToggleAutoReconcile}
            disabled={isViewer || isToggling}
            className="gap-2"
            title={isViewer ? 'Viewer role cannot change auto-reconcile' : undefined}
          >
            <RotateCcw className="w-4 h-4" />
            {server.auto_reconcile ? 'Disable' : 'Enable'} Auto-Reconcile
          </Button>

          {!isViewer && (
            <AlertDialog>
              <AlertDialogTrigger asChild>
                <Button variant="destructive" className="gap-2">
                  <Trash2 className="w-4 h-4" />
                  Delete
                </Button>
              </AlertDialogTrigger>
              <AlertDialogContent>
                <AlertDialogHeader>
                  <AlertDialogTitle>Delete Server</AlertDialogTitle>
                  <AlertDialogDescription className="text-sm text-muted-foreground leading-relaxed">
                    Are you sure you want to delete this server? This action cannot be undone.
                  </AlertDialogDescription>
                </AlertDialogHeader>
                <AlertDialogFooter>
                  <AlertDialogCancel>Cancel</AlertDialogCancel>
                  <AlertDialogAction onClick={handleDeleteServer}>
                    Delete
                  </AlertDialogAction>
                </AlertDialogFooter>
              </AlertDialogContent>
            </AlertDialog>
          )}
        </div>
      </div>

      <div className="flex flex-wrap items-center gap-2 text-xs sm:text-sm text-muted-foreground">
        <Badge variant="outline" className="flex items-center gap-2">
          <Clock className="w-3 h-3" />
          Last update: {formatTimestamp(server.last_timestamp)}
        </Badge>
        <div className="flex items-center gap-2">
          {getStatusBadge(server.last_status, server.last_is_drifted)}
        </div>
        <Badge variant={server.auto_reconcile ? 'default' : 'secondary'}>
          Auto-reconcile: {server.auto_reconcile ? 'Enabled' : 'Disabled'}
        </Badge>
      </div>

      {/* Server Overview */}
      <div className="grid gap-6 md:grid-cols-2">
        <Card>
          <CardHeader>
            <CardTitle className="flex items-center gap-2">
              <Settings className="w-5 h-5" />
              Configuration
            </CardTitle>
          </CardHeader>
          <CardContent className="space-y-4">
            <div>
              <label className="text-sm font-medium text-foreground">Environment</label>
              <div className="mt-1">
                {server.environment_id ? (
                  <Badge variant="outline">Environment ID: {server.environment_id}</Badge>
                ) : (
                  <span className="text-sm text-muted-foreground">Not assigned to environment</span>
                )}
              </div>
            </div>

            <div>
              <label className="text-sm font-medium text-foreground">Configuration Source</label>
              <div className="mt-1 text-sm text-muted-foreground">
                {server.environment_id 
                  ? 'Inherited from assigned environment'
                  : 'No environment configuration'
                }
              </div>
            </div>

            <div>
              <label className="text-sm font-medium text-foreground">Auto Reconcile</label>
              <div className="mt-1">
                <Badge variant={server.auto_reconcile ? 'default' : 'secondary'}>
                  {server.auto_reconcile ? 'Enabled' : 'Disabled'}
                </Badge>
              </div>
            </div>

          </CardContent>
        </Card>

        <Card>
          <CardHeader>
            <CardTitle className="flex items-center gap-2">
              <Clock className="w-5 h-5" />
              Current Status
            </CardTitle>
          </CardHeader>
          <CardContent className="space-y-4">
            <div>
              <label className="text-sm font-medium text-foreground">Status</label>
              <div className="mt-1">
                {getStatusBadge(server.last_status, server.last_is_drifted)}
              </div>
            </div>

            <div>
              <label className="text-sm font-medium text-foreground">Last Update</label>
              <div className="mt-1 text-sm text-muted-foreground">
                {formatTimestamp(server.last_timestamp)}
              </div>
            </div>

            <div>
              <label className="text-sm font-medium text-foreground">Agent Version</label>
              <div className="mt-1 text-sm text-muted-foreground">
                {server.last_agent_version ? (
                  <Badge variant="outline" className="font-mono text-xs">
                    {server.last_agent_version}
                  </Badge>
                ) : (
                  <span className="text-muted-foreground">Unknown</span>
                )}
              </div>
            </div>

              <div>
              <label className="text-sm font-medium text-foreground">Applied Commit</label>
                <div className="flex items-center gap-2 mt-1">
                  <GitCommit className="w-4 h-4 text-muted-foreground" />
                {server.last_commit_hash ? (
                  <code className="bg-muted px-2 py-1 rounded text-sm">
                    {server.last_commit_hash.substring(0, 8)}
                  </code>
                ) : (
                  <span className="text-sm text-muted-foreground">
                    No commits applied yet
                  </span>
                )}
              </div>
            </div>
          </CardContent>
        </Card>
      </div>

        {server.last_is_drifted && driftDetails && driftDetails.drift_details && (
          <Card className="border-red-200 bg-red-50/10">
            <CardHeader>
              <CardTitle className="flex items-center gap-2 text-red-700">
                <FileDiff className="w-5 h-5" />
                Drift Detected
              </CardTitle>
              <CardDescription>
                Configuration drift detected at {formatTimestamp(driftDetails.detected_at)}.
                The following resources differ from the desired state.
              </CardDescription>
            </CardHeader>
            <CardContent>
              <Tabs defaultValue="packages" className="w-full">
                <TabsList className="grid w-full grid-cols-3">
                  <TabsTrigger value="packages" className="gap-2">
                    <Package className="w-4 h-4" />
                    Packages ({driftDetails.drift_details.packages?.length || 0})
                  </TabsTrigger>
                  <TabsTrigger value="services" className="gap-2">
                    <Layers className="w-4 h-4" />
                    Services ({driftDetails.drift_details.services?.length || 0})
                  </TabsTrigger>
                  <TabsTrigger value="files" className="gap-2">
                    <FileDiff className="w-4 h-4" />
                    Files ({driftDetails.drift_details.files?.length || 0})
                  </TabsTrigger>
                </TabsList>
                
                <TabsContent value="packages" className="mt-4">
                  {(!driftDetails.drift_details.packages || driftDetails.drift_details.packages.length === 0) ? (
                    <div className="text-center py-8 text-muted-foreground bg-muted/20 rounded-md">
                      No package drift detected
                    </div>
                  ) : (
                    <Table>
                      <TableHeader>
                        <TableRow>
                          <TableHead>Package</TableHead>
                          <TableHead>Expected</TableHead>
                          <TableHead>Actual</TableHead>
                          <TableHead>Version Gap</TableHead>
                        </TableRow>
                      </TableHeader>
                      <TableBody>
                        {driftDetails.drift_details.packages.map((pkg, idx) => (
                          <TableRow key={idx}>
                            <TableCell className="font-medium">{pkg.name}</TableCell>
                            <TableCell>
                              <Badge variant="outline" className="bg-green-50 text-green-700 border-green-200">
                                {pkg.expected_state}
                              </Badge>
                            </TableCell>
                            <TableCell>
                              <Badge variant="outline" className="bg-red-50 text-red-700 border-red-200">
                                {pkg.actual_state}
                              </Badge>
                            </TableCell>
                            <TableCell className="font-mono text-sm">
                              {pkg.expected_version && pkg.actual_version ? (
                                <div className="flex flex-col gap-1">
                                  <span className="text-green-600">Want: {pkg.expected_version}</span>
                                  <span className="text-red-600">Have: {pkg.actual_version}</span>
                                </div>
                              ) : '-'}
                            </TableCell>
                          </TableRow>
                        ))}
                      </TableBody>
                    </Table>
                  )}
                </TabsContent>
                
                <TabsContent value="services" className="mt-4">
                  {(!driftDetails.drift_details.services || driftDetails.drift_details.services.length === 0) ? (
                    <div className="text-center py-8 text-muted-foreground bg-muted/20 rounded-md">
                      No service drift detected
                    </div>
                  ) : (
                    <Table>
                      <TableHeader>
                        <TableRow>
                          <TableHead>Service</TableHead>
                          <TableHead>Expected State</TableHead>
                          <TableHead>Actual State</TableHead>
                          <TableHead>Enabled</TableHead>
                        </TableRow>
                      </TableHeader>
                      <TableBody>
                        {driftDetails.drift_details.services.map((svc, idx) => (
                          <TableRow key={idx}>
                            <TableCell className="font-medium">{svc.name}</TableCell>
                            <TableCell>
                              <Badge variant="outline" className="bg-green-50 text-green-700 border-green-200">
                                {svc.expected_state}
                              </Badge>
                            </TableCell>
                            <TableCell>
                              <Badge variant="outline" className="bg-red-50 text-red-700 border-red-200">
                                {svc.actual_state}
                              </Badge>
                            </TableCell>
                            <TableCell>
                              <div className="flex items-center gap-2 text-sm">
                                <span className={svc.expected_enabled ? "text-green-600" : "text-muted-foreground"}>
                                  {svc.expected_enabled ? 'Enabled' : 'Disabled'}
                                </span>
                                <span className="text-muted-foreground">→</span>
                                <span className={svc.actual_enabled ? "text-red-600" : "text-muted-foreground"}>
                                  {svc.actual_enabled ? 'Enabled' : 'Disabled'}
                                </span>
                              </div>
                            </TableCell>
                          </TableRow>
                        ))}
                      </TableBody>
                    </Table>
                  )}
                </TabsContent>

                <TabsContent value="files" className="mt-4">
                  {(!driftDetails.drift_details.files || driftDetails.drift_details.files.length === 0) ? (
                    <div className="text-center py-8 text-muted-foreground bg-muted/20 rounded-md">
                      No file drift detected
                    </div>
                  ) : (
                    <div className="space-y-4">
                      {driftDetails.drift_details.files.map((file, idx) => (
                        <Card key={idx} className="border bg-card">
                          <div 
                            className="flex items-center justify-between p-4 cursor-pointer hover:bg-muted/50 transition-colors"
                            onClick={() => toggleFileExpand(file.path)}
                          >
                            <div className="flex items-center gap-3">
                              {expandedFiles[file.path] ? <ChevronDown className="w-4 h-4" /> : <ChevronRight className="w-4 h-4" />}
                              <span className="font-mono text-sm font-medium">{file.path}</span>
                              {file.content_differs && (
                                <Badge variant="destructive" className="text-[10px] h-5">Content Drift</Badge>
                              )}
                              {(file.expected_mode !== file.actual_mode) && (
                                <Badge variant="outline" className="text-[10px] h-5 border-yellow-500 text-yellow-600">
                                  Mode: {file.actual_mode} → {file.expected_mode}
                                </Badge>
                              )}
                            </div>
                          </div>
                          
                          {expandedFiles[file.path] && file.content_differs && (
                            <div className="border-t bg-muted/30 p-4 overflow-x-auto">
                              <div className="grid grid-cols-2 gap-4 min-w-[600px]">
                                <div>
                                  <div className="text-xs font-semibold text-green-600 mb-2 uppercase tracking-wider">Expected Content</div>
                                  <pre className="text-xs bg-background border p-3 rounded font-mono overflow-auto max-h-[300px] whitespace-pre-wrap break-all">
                                    {file.expected_content || '(empty)'}
                                  </pre>
                                </div>
                                <div>
                                  <div className="text-xs font-semibold text-red-600 mb-2 uppercase tracking-wider">Actual Content</div>
                                  <pre className="text-xs bg-background border p-3 rounded font-mono overflow-auto max-h-[300px] whitespace-pre-wrap break-all">
                                    {file.actual_content || '(empty)'}
                                  </pre>
                                </div>
                              </div>
                            </div>
                          )}
                        </Card>
                      ))}
                    </div>
                  )}
                </TabsContent>
              </Tabs>
            </CardContent>
          </Card>
        )}

      {/* Status History */}
      <Card>
        <CardHeader>
          <CardTitle className="flex items-center gap-2">
            <Clock className="w-5 h-5" />
            Status History
            {isRefreshing && (
              <div className="flex items-center gap-1 text-sm text-green-600">
                <RotateCcw className="w-3 h-3 animate-spin" />
                <span>Refreshing...</span>
              </div>
            )}
          </CardTitle>
          <CardDescription>
            Recent configuration enforcement history and status changes
            {isAutoRefreshEnabled && (
              <span className="ml-2 text-xs text-green-600">
                • Auto-refresh every 30 seconds
              </span>
            )}
          </CardDescription>
        </CardHeader>
        <CardContent>
          {isHistoryLoading ? (
            <div className="space-y-4">
              {[...Array(5)].map((_, i) => (
                <div key={i} className="flex items-center space-x-4">
                  <Skeleton className="h-4 w-[100px]" />
                  <Skeleton className="h-4 w-[120px]" />
                  <Skeleton className="h-4 w-[80px]" />
                  <Skeleton className="h-4 w-[200px]" />
                </div>
              ))}
            </div>
          ) : (
            <div className="overflow-x-auto">
              <Table className="table-fixed w-full">
                <TableHeader>
                  <TableRow>
                    <TableHead className="w-1/5">Status</TableHead>
                    <TableHead className="w-1/5">Commit</TableHead>
                    <TableHead className="w-1/5">Drift</TableHead>
                    <TableHead className="w-1/5">Timestamp</TableHead>
                    <TableHead className="w-1/5">Message</TableHead>
                  </TableRow>
                </TableHeader>
              <TableBody>
                {statusHistory?.data?.length === 0 ? (
                  <TableRow>
                    <TableCell colSpan={5} className="text-center py-8">
                      <div className="flex flex-col items-center gap-2">
                        <Clock className="w-8 h-8 text-muted-foreground" />
                        <p className="text-muted-foreground">No status history found</p>
                      </div>
                    </TableCell>
                  </TableRow>
                ) : (
                  statusHistory?.data?.map((entry: ServerStatusHistory) => (
                    <TableRow key={entry.id} className="h-14">
                      <TableCell className="w-1/5 py-3">
                        {getStatusBadge(entry.status, entry.is_drifted)}
                      </TableCell>
                      <TableCell className="w-1/5 py-3">
                        <code className="px-2 py-1 rounded text-sm bg-muted text-foreground">
                          {entry.commit_hash?.substring(0, 8) || 'N/A'}
                        </code>
                      </TableCell>
                      <TableCell className="w-1/5 py-3">
                        <Badge variant={entry.is_drifted ? 'destructive' : 'secondary'}>
                          {entry.is_drifted ? 'Yes' : 'No'}
                        </Badge>
                      </TableCell>
                      <TableCell className="w-1/5 py-3 text-sm text-muted-foreground">
                        {formatTimestamp(entry.timestamp)}
                      </TableCell>
                      <TableCell className="w-1/5 py-3">
                        {entry.error_message && entry.error_message !== '-' ? (
                          <SmartTooltip
                            content={entry.error_message}
                            side="top"
                            className={`text-sm ${
                              entry.error_message.includes('auto-reconciled successfully') 
                                ? 'text-green-600' 
                                : 'text-red-600'
                            } truncate max-w-full`}
                          >
                            <span className="block truncate">{entry.error_message}</span>
                          </SmartTooltip>
                        ) : (
                          <span className="text-sm text-muted-foreground">-</span>
                        )}
                      </TableCell>
                    </TableRow>
                  ))
                )}
              </TableBody>
                </Table>
              </div>
          )}

          {/* Pagination */}
          {statusHistory && statusHistory.total_pages > 1 && (
            <div className="flex items-center justify-between mt-4">
              <p className="text-sm text-muted-foreground">
                Showing {statusHistory.data.length} of {statusHistory.total} entries
              </p>
              <div className="flex items-center gap-2">
                <Button
                  variant="outline"
                  size="sm"
                  onClick={() => loadStatusHistory(currentHistoryPage - 1)}
                  disabled={currentHistoryPage <= 1 || isHistoryLoading}
                >
                  Previous
                </Button>
                <span className="text-sm text-muted-foreground">
                  Page {currentHistoryPage} of {statusHistory.total_pages}
                </span>
                <Button
                  variant="outline"
                  size="sm"
                  onClick={() => loadStatusHistory(currentHistoryPage + 1)}
                  disabled={currentHistoryPage >= statusHistory.total_pages || isHistoryLoading}
                >
                  Next
                </Button>
              </div>
            </div>
          )}
        </CardContent>
      </Card>
      {!isViewer ? (
      <ServerTokenManagement
        serverId={server.id}
        serverName={server.name || server.id}
      />
    ) : (
      <Card>
        <CardHeader>
          <CardTitle>Agent Tokens</CardTitle>
          <CardDescription>Viewer role has read-only access. Token management is restricted.</CardDescription>
        </CardHeader>
        <CardContent className="space-y-3 text-sm text-muted-foreground">
          <p>
            Request token operations from an administrator. They can issue, revoke, or rotate tokens using the
            Pullbase CLI or the admin dashboard.
          </p>
          <Button variant="outline" size="sm" asChild>
            <a
              href="https://github.com/pullbase/pullbase"
              target="_blank"
              rel="noopener noreferrer"
            >
              View CLI documentation
            </a>
          </Button>
        </CardContent>
      </Card>
    )}
    </div>
  )
}
