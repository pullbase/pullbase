import { useState, useEffect } from 'react'
import { serversApi, type AgentToken, type CreateTokenRequest } from '../lib/api'
import { Button } from './ui/button'
import { Input } from './ui/input'
import { Label } from './ui/label'
import { toast } from 'sonner'
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from './ui/card'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
  DialogTrigger,
} from './ui/dialog'
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
} from './ui/alert-dialog'
import { Badge } from './ui/badge'
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from './ui/table'
import { Skeleton } from './ui/skeleton'
import { 
  Plus, 
  Key, 
  Copy, 
  Trash2, 
  AlertTriangle,
  CheckCircle2,
  Terminal
} from 'lucide-react'
import { formatDistanceToNow, differenceInDays } from 'date-fns'

interface ServerTokenManagementProps {
  serverId: string
  serverName: string
}

export default function ServerTokenManagement({ serverId, serverName }: ServerTokenManagementProps) {
  const [tokens, setTokens] = useState<AgentToken[]>([])
  const [isLoading, setIsLoading] = useState(true)
  const [isCreating, setIsCreating] = useState(false)
  const [createDialogOpen, setCreateDialogOpen] = useState(false)
  const [newToken, setNewToken] = useState<{ token: string; instructions: string; exampleCmd?: string } | null>(null)
  const [formData, setFormData] = useState<CreateTokenRequest>({
    description: '',
    expires_in: undefined
  })

  const loadTokens = async () => {
    try {
      setIsLoading(true)
      const data = await serversApi.listTokens(serverId)
      setTokens(data)
    } catch (error) {
      console.error('Failed to load tokens:', error)
      toast.error('Failed to load agent tokens')
    } finally {
      setIsLoading(false)
    }
  }

  const handleCreateToken = async (e: React.FormEvent) => {
    e.preventDefault()
    
    try {
      setIsCreating(true)
      const response = await serversApi.createToken(serverId, formData)
      setNewToken({
        token: response.token,
        instructions: response.installation_info.instructions,
        exampleCmd: response.installation_info.example_cmd
      })
      await loadTokens()
      toast.success('Agent token created successfully')
      setFormData({ description: '', expires_in: undefined })
    } catch (error) {
      console.error('Failed to create token:', error)
      toast.error('Failed to create agent token')
    } finally {
      setIsCreating(false)
    }
  }

  const handleDeactivateToken = async (tokenId: number) => {
    try {
      await serversApi.deactivateToken(serverId, tokenId)
      await loadTokens()
      toast.success('Agent token deactivated successfully')
    } catch (error) {
      console.error('Failed to deactivate token:', error)
      toast.error('Failed to deactivate agent token')
    }
  }

  const copyToClipboard = async (text: string, label: string) => {
    try {
      await navigator.clipboard.writeText(text)
      toast.success(`${label} copied to clipboard`)
    } catch (error) {
      console.error('Failed to copy to clipboard:', error)
      toast.error('Failed to copy to clipboard')
    }
  }

  const resetCreateDialog = () => {
    setCreateDialogOpen(false)
    setNewToken(null)
    setFormData({ description: '', expires_in: undefined })
  }

  const formatDate = (dateString: string) => {
    try {
      return formatDistanceToNow(new Date(dateString), { addSuffix: true })
    } catch {
      return 'Invalid date'
    }
  }

  useEffect(() => {
    loadTokens()
  }, [serverId]) // eslint-disable-line react-hooks/exhaustive-deps

  return (
    <Card>
      <CardHeader>
        <div className="flex items-center justify-between">
          <div>
            <CardTitle className="flex items-center gap-2">
              <Key className="w-5 h-5" />
              Agent Tokens
            </CardTitle>
            <CardDescription>
              Manage authentication tokens for agents connecting to this server
            </CardDescription>
          </div>
          <Dialog open={createDialogOpen} onOpenChange={setCreateDialogOpen}>
            <DialogTrigger asChild>
              <Button className="gap-2">
                <Plus className="w-4 h-4" />
                Create Token
              </Button>
            </DialogTrigger>
            <DialogContent className="w-full sm:max-w-3xl lg:max-w-4xl max-h-[90vh] overflow-y-auto">
              <DialogHeader>
                <DialogTitle className="flex items-center gap-2">
                  <Key className="w-5 h-5" />
                  {newToken ? 'Token Created Successfully' : 'Create Agent Token'}
                </DialogTitle>
                <DialogDescription>
                  {newToken 
                    ? 'Your token has been created. Make sure to copy it before closing this dialog.'
                    : `Create a new authentication token for agents connecting to "${serverName}".`
                  }
                </DialogDescription>
              </DialogHeader>

              {!newToken ? (
                <form onSubmit={handleCreateToken} className="space-y-4">
                  <div>
                    <Label htmlFor="description">Description</Label>
                    <Input
                      id="description"
                      value={formData.description || ''}
                      onChange={(e) => setFormData(prev => ({ ...prev, description: e.target.value }))}
                      placeholder="e.g., Production deployment token"
                    />
                  </div>

                  <div>
                    <Label htmlFor="expires_in">Expiration (days)</Label>
                    <Input
                      id="expires_in"
                      type="number"
                      min="1"
                      max="365"
                      value={formData.expires_in || ''}
                      onChange={(e) => setFormData(prev => ({ 
                        ...prev, 
                        expires_in: e.target.value ? parseInt(e.target.value) : undefined 
                      }))}
                      placeholder="Leave empty for no expiration"
                    />
                  </div>

                  <DialogFooter>
                    <Button type="button" variant="outline" onClick={resetCreateDialog}>
                      Cancel
                    </Button>
                    <Button type="submit" disabled={isCreating}>
                      {isCreating ? 'Creating...' : 'Create Token'}
                    </Button>
                  </DialogFooter>
                </form>
              ) : (
                <div className="space-y-6">
                  {/* Success Banner */}
                  <Card className="border-green-200 bg-green-50">
                    <CardContent className="pt-6">
                      <div className="flex items-center gap-3">
                        <CheckCircle2 className="w-5 h-5 text-green-600" />
                        <div>
                          <p className="font-medium text-green-900">
                            Agent token created successfully
                          </p>
                          <p className="text-sm text-green-700">
                            The token is ready for use with your agent.
                          </p>
                        </div>
                      </div>
                    </CardContent>
                  </Card>

                  {/* Security Warning */}
                  <Card className="border-amber-200 bg-amber-50">
                    <CardContent className="pt-6">
                      <div className="flex items-start gap-3">
                        <AlertTriangle className="w-5 h-5 text-amber-600 mt-0.5" />
                        <div>
                          <p className="font-medium text-amber-900">Important Security Notice</p>
                          <p className="text-sm text-amber-700 mt-1">
                            This token will only be displayed once. Make sure to copy and securely store it.
                          </p>
                        </div>
                      </div>
                    </CardContent>
                  </Card>

                  {/* Token Display */}
                  <Card>
                    <CardHeader>
                      <CardTitle className="text-lg">Agent Token</CardTitle>
                      <CardDescription>
                        Use this token to authenticate your agent with the controller.
                      </CardDescription>
                    </CardHeader>
                    <CardContent>
                      <div className="flex items-center gap-2">
                        <code className="flex-1 p-3 bg-gray-100 rounded text-sm font-mono break-all">
                          {newToken.token}
                        </code>
                        <Button
                          size="sm"
                          variant="outline"
                          onClick={() => copyToClipboard(newToken.token, 'Agent token')}
                        >
                          <Copy className="w-4 h-4" />
                        </Button>
                      </div>
                    </CardContent>
                  </Card>

                  {/* Installation Instructions */}
                  <Card>
                    <CardHeader>
                      <CardTitle className="flex items-center gap-2">
                        <Terminal className="w-5 h-5" />
                        Installation Instructions
                      </CardTitle>
                    </CardHeader>
                    <CardContent className="space-y-4">
                      <div className="space-y-2">
                        <div className="flex items-center justify-between gap-2">
                          <p className="text-sm font-medium text-foreground">Step-by-step guide</p>
                          <Button
                            size="sm"
                            variant="outline"
                            onClick={() => copyToClipboard(newToken.instructions, 'Installation instructions')}
                            className="gap-2"
                          >
                            <Copy className="w-4 h-4" />
                            Copy
                          </Button>
                        </div>
                      <pre className="bg-gray-100 rounded p-3 text-sm font-mono whitespace-pre-wrap text-muted-foreground overflow-x-auto">
{newToken.instructions}
                      </pre>
                      </div>

                      {newToken.exampleCmd && (
                        <div className="space-y-2">
                          <div className="flex items-center justify-between gap-2">
                            <p className="text-sm font-medium text-foreground">Quick start command</p>
                            <Button
                              size="sm"
                              variant="outline"
                              onClick={() => copyToClipboard(newToken.exampleCmd!, 'Example command')}
                              className="gap-2"
                            >
                              <Copy className="w-4 h-4" />
                              Copy
                            </Button>
                          </div>
                          <pre className="bg-gray-100 rounded p-3 text-sm font-mono whitespace-pre-wrap text-muted-foreground overflow-x-auto">
{newToken.exampleCmd}
                          </pre>
                        </div>
                      )}
                    </CardContent>
                  </Card>

                  <DialogFooter>
                    <Button onClick={resetCreateDialog} className="w-full">
                      Done
                    </Button>
                  </DialogFooter>
                </div>
              )}
            </DialogContent>
          </Dialog>
        </div>
      </CardHeader>
      <CardContent>
        {isLoading ? (
          <div className="space-y-4">
            {[...Array(3)].map((_, i) => (
              <div key={i} className="flex items-center space-x-4">
                <Skeleton className="h-4 w-[200px]" />
                <Skeleton className="h-4 w-[100px]" />
                <Skeleton className="h-4 w-[120px]" />
                <Skeleton className="h-4 w-[80px]" />
              </div>
            ))}
          </div>
        ) : tokens.length === 0 ? (
          <div className="text-center py-8">
            <Key className="w-8 h-8 text-muted-foreground mx-auto mb-2" />
            <p className="text-muted-foreground mb-2">No agent tokens found</p>
            <p className="text-sm text-muted-foreground mb-4">
              Create a token to allow agents to authenticate with this server.
            </p>
          </div>
        ) : (
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead>Description</TableHead>
                <TableHead>Created</TableHead>
                <TableHead>Last Used</TableHead>
                <TableHead>Status</TableHead>
                <TableHead>Actions</TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              {tokens.map((token) => (
                <TableRow key={token.id}>
                  <TableCell>
                    <div>
                      <p className="font-medium">
                        {token.description || `Token ${token.id}`}
                      </p>
                      {token.expires_at && (
                        <div className="flex flex-wrap items-center gap-2 mt-0.5">
                          <p className="text-sm text-muted-foreground">
                            Expires {formatDate(token.expires_at)}
                          </p>
                          {(() => {
                            const days = differenceInDays(new Date(token.expires_at), new Date())
                            if (days <= 7 && days >= 0) {
                              return (
                                <Badge 
                                  variant="outline" 
                                  className="h-5 gap-1 border-amber-500 text-amber-600 bg-amber-50 px-1.5 text-[10px] whitespace-nowrap"
                                >
                                  <AlertTriangle className="w-3 h-3" />
                                  Expires in {days === 0 ? 'today' : `${days} day${days === 1 ? '' : 's'}`}
                                </Badge>
                              )
                            }
                            return null
                          })()}
                        </div>
                      )}
                    </div>
                  </TableCell>
                  <TableCell className="text-sm text-muted-foreground">
                    {formatDate(token.created_at)}
                  </TableCell>
                  <TableCell className="text-sm text-muted-foreground">
                    {token.last_used_at ? formatDate(token.last_used_at) : 'Never'}
                  </TableCell>
                  <TableCell>
                    <Badge variant={token.is_active ? 'default' : 'secondary'}>
                      {token.is_active ? 'Active' : 'Inactive'}
                    </Badge>
                  </TableCell>
                  <TableCell>
                    {token.is_active && (
                      <AlertDialog>
                        <AlertDialogTrigger asChild>
                          <Button variant="ghost" size="sm" className="text-red-600 hover:text-red-800">
                            <Trash2 className="w-4 h-4" />
                          </Button>
                        </AlertDialogTrigger>
                        <AlertDialogContent>
                          <AlertDialogHeader>
                            <AlertDialogTitle>Deactivate Token</AlertDialogTitle>
                            <AlertDialogDescription>
                              Are you sure you want to deactivate this agent token? Agents using this token will no longer be able to authenticate. This action cannot be undone.
                            </AlertDialogDescription>
                          </AlertDialogHeader>
                          <AlertDialogFooter>
                            <AlertDialogCancel>Cancel</AlertDialogCancel>
                            <AlertDialogAction 
                              onClick={() => handleDeactivateToken(token.id)}
                              className="bg-red-600 hover:bg-red-700"
                            >
                              Deactivate
                            </AlertDialogAction>
                          </AlertDialogFooter>
                        </AlertDialogContent>
                      </AlertDialog>
                    )}
                  </TableCell>
                </TableRow>
              ))}
            </TableBody>
          </Table>
        )}
      </CardContent>
    </Card>
  )
} 
