import { useState, useEffect, useMemo, useCallback } from 'react'
import { Link } from 'react-router-dom'
import { serversApi, type Server, type PaginationResponse } from '../lib/api'
import { Button } from '../components/ui/button'
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '../components/ui/card'
import { Badge } from '../components/ui/badge'
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from '../components/ui/table'
import { Skeleton } from '../components/ui/skeleton'
import { Input } from '../components/ui/input'
import { ExternalLink, Server as ServerIcon, GitBranch, Clock, CheckCircle, AlertCircle, XCircle } from 'lucide-react'
import { formatDistanceToNow } from 'date-fns'
import CreateServerDialog from '../components/CreateServerDialog'
import { useAuth } from '../contexts/auth-context'

export default function ServersPage() {
  const [serversData, setServersData] = useState<PaginationResponse<Server> | null>(null)
  const [isLoading, setIsLoading] = useState(true)
  const [currentPage, setCurrentPage] = useState(1)
  const [searchTerm, setSearchTerm] = useState('')
  const [statusFilter, setStatusFilter] = useState<'all' | 'applied' | 'failed' | 'drifted'>('all')
  const [sortKey, setSortKey] = useState<'recent' | 'name' | 'status'>('recent')
  const { user } = useAuth()
  const isViewer = user?.role === 'viewer'

  const loadServers = useCallback(async (page = 1) => {
    try {
      setIsLoading(true)
      const data = await serversApi.list(page, 10, sortKey)
      setServersData(data)
      setCurrentPage(page)
    } catch (error) {
      console.error('Failed to load servers:', error)
    } finally {
      setIsLoading(false)
    }
  }, [sortKey])

  useEffect(() => {
    loadServers(1)
  }, [loadServers])

  const filteredServers = useMemo(() => {
    const list = serversData?.data ?? []
    const term = searchTerm.trim().toLowerCase()

    const filtered = list.filter((server) => {
      const matchesSearch =
        term.length === 0 ||
        server.name.toLowerCase().includes(term) ||
        server.id.toLowerCase().includes(term) ||
        (server.environment_name || '').toLowerCase().includes(term)

      let matchesStatus = true
      if (statusFilter === 'applied') {
        matchesStatus = server.last_status === 'Applied'
      } else if (statusFilter === 'failed') {
        matchesStatus = server.last_status === 'Failed'
      } else if (statusFilter === 'drifted') {
        matchesStatus = !!server.last_is_drifted
      }

      return matchesSearch && matchesStatus
    })

    const sorted = [...filtered]
    if (sortKey === 'name') {
      sorted.sort((a, b) => (a.name || a.id).localeCompare(b.name || b.id))
    } else if (sortKey === 'status') {
      sorted.sort((a, b) => (a.last_status || '').localeCompare(b.last_status || ''))
    } else {
      sorted.sort((a, b) => {
        const aTime = a.last_timestamp ? new Date(a.last_timestamp).getTime() : 0
        const bTime = b.last_timestamp ? new Date(b.last_timestamp).getTime() : 0
        return bTime - aTime
      })
    }

    return sorted
  }, [serversData, searchTerm, statusFilter, sortKey])

  const hasNoServers = !serversData || !Array.isArray(serversData.data) || serversData.data.length === 0
  const hasNoFilteredServers = filteredServers.length === 0

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

  const formatLastUpdate = (timestamp?: string) => {
    if (!timestamp) return 'Never'
    try {
      return formatDistanceToNow(new Date(timestamp), { addSuffix: true })
    } catch {
      return 'Invalid date'
    }
  }

  return (
    <div className="space-y-6">
      {/* Header */}
      <div className="flex justify-between items-center">
      <div>
        <h1 className="text-3xl font-bold text-foreground">Servers</h1>
        <p className="text-muted-foreground mt-1">
          Monitor and enforce server configuration compliance
        </p>
        {isViewer && (
          <p className="text-xs text-muted-foreground mt-2">
            Viewer role is read-only. Token and server management actions are disabled.
          </p>
        )}
      </div>
      {isViewer ? (
        <Button variant="outline" disabled title="Viewer role cannot register servers" className="cursor-not-allowed">
          Create Server
        </Button>
      ) : (
        <CreateServerDialog onServerCreated={() => loadServers(1)} />
      )}
      </div>

      {/* Servers Table */}
      <Card>
        <CardHeader>
          <CardTitle className="flex items-center gap-2">
            <ServerIcon className="w-5 h-5" />
            Server List
          </CardTitle>
          <CardDescription>
            {filteredServers.length} of {serversData?.total || 0} server{(serversData?.total || 0) !== 1 ? 's' : ''}
          </CardDescription>
        </CardHeader>
        <CardContent>
          <div className="flex flex-col gap-3 md:flex-row md:items-center md:justify-between mb-4">
            <Input
              placeholder="Search by server or environment..."
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
                  <option value="applied">Applied</option>
                  <option value="failed">Failed</option>
                  <option value="drifted">Drifted</option>
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
                  <option value="status">Status</option>
                </select>
              </div>
            </div>
          </div>
          {isLoading ? (
            <div className="space-y-4">
              {[...Array(5)].map((_, i) => (
                <div key={i} className="flex items-center space-x-4">
                  <Skeleton className="h-4 w-[100px]" />
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
                    <TableHead>Server</TableHead>
                    <TableHead>Environment</TableHead>
                    <TableHead>Status</TableHead>
                    <TableHead>Last Update</TableHead>
                    <TableHead>Auto Reconcile</TableHead>
                    <TableHead></TableHead>
                  </TableRow>
                </TableHeader>
                <TableBody>
                  {hasNoServers ? (
                    <TableRow>
                      <TableCell colSpan={6} className="text-center py-8">
                        <div className="flex flex-col items-center gap-2">
                        <ServerIcon className="w-8 h-8 text-muted-foreground" />
                        <p className="text-muted-foreground">No servers found</p>
                        <p className="text-sm text-muted-foreground">
                            Add your first server to start monitoring configuration compliance
                          </p>
                        </div>
                      </TableCell>
                    </TableRow>
                  ) : hasNoFilteredServers ? (
                    <TableRow>
                      <TableCell colSpan={6} className="py-12">
                        <div className="flex flex-col items-center gap-3 text-center">
                          <ServerIcon className="w-8 h-8 text-muted-foreground" />
                          <p className="text-muted-foreground">No servers match the current filters.</p>
                          <p className="text-sm text-muted-foreground">Adjust your search or status filter to see more results.</p>
                        </div>
                      </TableCell>
                    </TableRow>
                  ) : (
                    filteredServers.map((server) => (
                      <TableRow
                        key={server.id}
                        className="transition-colors hover:bg-muted/60 dark:hover:bg-muted/20"
                      >
                        <TableCell className="font-medium">
                          <div className="flex items-center gap-2">
                            <ServerIcon className="w-4 h-4 text-muted-foreground" />
                            {server.name || server.id}
                          </div>
                        </TableCell>
                        <TableCell>
                          <div className="flex items-center gap-2">
                            <GitBranch className="w-4 h-4 text-muted-foreground" />
                            {server.environment_name ? (
                              <Badge variant="outline" className="dark:border-muted dark:text-muted-foreground">
                                {server.environment_name}
                              </Badge>
                            ) : (
                              <span className="text-sm text-muted-foreground">No environment</span>
                            )}
                          </div>
                        </TableCell>
                        <TableCell>
                          {getStatusBadge(server.last_status, server.last_is_drifted)}
                        </TableCell>
                        <TableCell className="text-sm text-muted-foreground">
                          {formatLastUpdate(server.last_timestamp)}
                        </TableCell>
                        <TableCell>
                          <Badge variant={server.auto_reconcile ? 'default' : 'secondary'}>
                            {server.auto_reconcile ? 'Enabled' : 'Disabled'}
                          </Badge>
                        </TableCell>
                        <TableCell>
                          <Link to={`/ui/servers/${server.id}`}>
                            <Button variant="ghost" size="sm" className="gap-1 text-primary">
                              <ExternalLink className="w-3 h-3" />
                              View
                            </Button>
                          </Link>
                        </TableCell>
                      </TableRow>
                    ))
                  )}
                </TableBody>
              </Table>

              {/* Pagination */}
          {serversData && serversData.total_pages > 1 && (
            <div className="flex items-center justify-between mt-4">
              <p className="text-sm text-muted-foreground">
                Showing {filteredServers.length} of {serversData.total} results
              </p>
                  <div className="flex gap-2">
                    <Button
                      variant="outline"
                      size="sm"
                      onClick={() => loadServers(currentPage - 1)}
                      disabled={currentPage === 1}
                    >
                      Previous
                    </Button>
                    <Button
                      variant="outline"
                      size="sm"
                      onClick={() => loadServers(currentPage + 1)}
                      disabled={currentPage === serversData.total_pages}
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
    </div>
  )
} 
