import { useState, useEffect } from 'react'
import { serversApi, environmentsApi, type CreateServerResponse, type Environment } from '../lib/api'
import { Button } from './ui/button'
import { Input } from './ui/input'
import { Label } from './ui/label'
import { toast } from 'sonner'
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
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from './ui/card'
import { AlertTriangle, CheckCircle2, Copy, Server, Terminal } from 'lucide-react'

interface CreateServerDialogProps {
  onServerCreated?: () => void
  trigger?: React.ReactNode
}

export default function CreateServerDialog({ onServerCreated, trigger }: CreateServerDialogProps) {
  const [open, setOpen] = useState(false)
  const [isSubmitting, setIsSubmitting] = useState(false)
  const [isLoadingEnvironments, setIsLoadingEnvironments] = useState(true)
  const [environments, setEnvironments] = useState<Environment[]>([])
  const [createdServer, setCreatedServer] = useState<CreateServerResponse | null>(null)
  const [formData, setFormData] = useState({
    id: '',
    name: '',
    environment_id: ''
  })

  // Load environments when dialog opens
  useEffect(() => {
    if (open) {
      loadEnvironments()
    }
  }, [open])

  const loadEnvironments = async () => {
    try {
      setIsLoadingEnvironments(true)
    const response = await environmentsApi.list(1, 100, 'name') // Get all environments
      const envs = Array.isArray(response) ? response : (response?.data || [])
      setEnvironments(envs)
    } catch (error) {
      console.error('Failed to load environments:', error)
      toast.error('Failed to load environments')
    } finally {
      setIsLoadingEnvironments(false)
    }
  }

  const resetForm = () => {
    setFormData({
      id: '',
      name: '',
      environment_id: ''
    })
    setCreatedServer(null)
  }

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault()
    
    if (!formData.id || !formData.name || !formData.environment_id) {
      toast.error('Please fill in all required fields')
      return
    }

    try {
      setIsSubmitting(true)
      const response = await serversApi.create({
        id: formData.id,
        name: formData.name,
        environment_id: parseInt(formData.environment_id)
      })
      setCreatedServer(response)
      toast.success('Server created successfully!')
      onServerCreated?.()
    } catch (error) {
      console.error('Failed to create server:', error)
      toast.error('Failed to create server')
    } finally {
      setIsSubmitting(false)
    }
  }

  const handleInputChange = (e: React.ChangeEvent<HTMLInputElement | HTMLSelectElement>) => {
    const { name, value } = e.target
    setFormData(prev => ({ ...prev, [name]: value }))
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

  const handleClose = () => {
    setOpen(false)
    setTimeout(resetForm, 200)
  }

  const defaultTrigger = (
    <Button className="gap-2">
      <Server className="w-4 h-4" />
      Add Server
    </Button>
  )

  const selectedEnvironment = environments.find(env => env.id.toString() === formData.environment_id)

  return (
    <Dialog open={open} onOpenChange={setOpen}>
      <DialogTrigger asChild>
        {trigger || defaultTrigger}
      </DialogTrigger>
      <DialogContent className="max-w-2xl max-h-[90vh] overflow-y-auto">
        <DialogHeader>
          <DialogTitle className="flex items-center gap-2">
            <Server className="w-5 h-5" />
            {createdServer ? 'Server Created Successfully' : 'Add New Server'}
          </DialogTitle>
          <DialogDescription>
            {createdServer 
              ? 'Your server has been created. Here are the installation details.'
              : 'Configure a new server and assign it to an environment for configuration management.'
            }
          </DialogDescription>
        </DialogHeader>

        {!createdServer ? (
          // Server Creation Form
          <form onSubmit={handleSubmit} className="space-y-4">
            <div className="grid grid-cols-2 gap-4">
              <div>
                <Label htmlFor="id">Server ID *</Label>
                <Input
                  id="id"
                  name="id"
                  value={formData.id}
                  onChange={handleInputChange}
                  placeholder="web-prod-01"
                  required
                />
              </div>
              <div>
                <Label htmlFor="name">Display Name *</Label>
                <Input
                  id="name"
                  name="name"
                  value={formData.name}
                  onChange={handleInputChange}
                  placeholder="Production Web Server"
                  required
                />
              </div>
            </div>

            <div>
              <Label htmlFor="environment_id">Environment *</Label>
              {isLoadingEnvironments ? (
                <div className="h-10 bg-gray-100 animate-pulse rounded"></div>
              ) : environments.length === 0 ? (
                <div className="p-3 border rounded bg-yellow-50 border-yellow-200">
                  <p className="text-sm text-yellow-800">
                    No environments found. You need to create an environment first before adding servers.
                  </p>
                </div>
              ) : (
                <select
                  id="environment_id"
                  name="environment_id"
                  value={formData.environment_id}
                  onChange={handleInputChange}
                  className="w-full h-10 rounded-md border border-input bg-background px-3 py-2 text-sm text-foreground"
                  required
                >
                  <option value="">Select an environment</option>
                  {environments.map((env) => (
                    <option key={env.id} value={env.id.toString()}>
                      {env.name} (GitHub)
                    </option>
                  ))}
                </select>
              )}
            </div>

            {selectedEnvironment && (
              <div className="p-3 bg-blue-50 border border-blue-200 rounded">
                <h4 className="font-medium text-blue-900 mb-2">Environment Configuration</h4>
                <div className="space-y-1 text-sm text-blue-800">
                  <p><strong>Repository:</strong> {selectedEnvironment.repo_url}</p>
                  <p><strong>Installation ID:</strong> {selectedEnvironment.installation_id}</p>
                  {selectedEnvironment.app_slug && (
                    <p><strong>App Slug:</strong> {selectedEnvironment.app_slug}</p>
                  )}
                  {selectedEnvironment.repository_id && (
                    <p><strong>Repository ID:</strong> {selectedEnvironment.repository_id}</p>
                  )}
                  <p><strong>Provider:</strong> GitHub</p>
                </div>
                <p className="text-xs text-blue-600 mt-2">
                  The server will inherit this Git configuration from the environment.
                </p>
              </div>
            )}

            <DialogFooter>
              <Button 
                type="button" 
                variant="outline" 
                onClick={handleClose}
                disabled={isSubmitting}
              >
                Cancel
              </Button>
              <Button 
                type="submit" 
                disabled={isSubmitting || environments.length === 0}
              >
                {isSubmitting ? 'Creating...' : 'Create Server'}
              </Button>
            </DialogFooter>
          </form>
        ) : (
          // Server Created - Show Installation Instructions
          <div className="space-y-6">
            {/* Success Banner */}
            <Card className="border-green-200 bg-green-50">
              <CardContent className="pt-6">
                <div className="flex items-center gap-3">
                  <CheckCircle2 className="w-5 h-5 text-green-600" />
                  <div>
                    <p className="font-medium text-green-900">
                      Server "{createdServer.name}" created successfully
                    </p>
                    <p className="text-sm text-green-700">
                      An agent token has been generated for secure communication.
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
                      The agent token below will only be displayed once. Make sure to copy and securely store it before closing this dialog.
                    </p>
                  </div>
                </div>
              </CardContent>
            </Card>

            {/* Agent Token */}
            <Card>
              <CardHeader>
                <CardTitle className="flex items-center gap-2">
                  <Copy className="w-5 h-5" />
                  Agent Token
                </CardTitle>
                <CardDescription>
                  Use this token to authenticate the agent with the central server.
                </CardDescription>
              </CardHeader>
              <CardContent>
                <div className="flex items-center gap-2">
                  <code className="flex-1 p-2 bg-gray-100 rounded text-sm break-all">
                    {createdServer.agent_token}
                  </code>
                  <Button
                    size="sm"
                    variant="outline"
                    onClick={() => copyToClipboard(createdServer.agent_token, 'Agent token')}
                  >
                    <Copy className="w-4 h-4" />
                  </Button>
                </div>
              </CardContent>
            </Card>

            <Card>
              <CardHeader>
                <CardTitle className="flex items-center gap-2">
                  <Terminal className="w-5 h-5" />
                  Installation Instructions
                </CardTitle>
                <CardDescription>
                  Follow these steps to install and configure the agent on your server.
                </CardDescription>
              </CardHeader>
              <CardContent>
                <div className="space-y-4">
                  <div>
                    <h4 className="font-medium mb-2">1. Environment Variables</h4>
                    <div className="space-y-2">
                      <div className="flex items-center gap-2">
                        <code className="flex-1 p-2 bg-gray-100 rounded text-sm">
                          export CENTRAL_SERVER_URL={window.location.origin}
                        </code>
                        <Button
                          size="sm"
                          variant="outline"
                          onClick={() => copyToClipboard(`export CENTRAL_SERVER_URL=${window.location.origin}`, 'Server URL')}
                        >
                          <Copy className="w-4 h-4" />
                        </Button>
                      </div>
                      <div className="flex items-center gap-2">
                        <code className="flex-1 p-2 bg-gray-100 rounded text-sm">
                          export AGENT_TOKEN={createdServer.agent_token}
                        </code>
                        <Button
                          size="sm"
                          variant="outline"
                          onClick={() => copyToClipboard(`export AGENT_TOKEN=${createdServer.agent_token}`, 'Agent token env var')}
                        >
                          <Copy className="w-4 h-4" />
                        </Button>
                      </div>
                      <div className="flex items-center gap-2">
                        <code className="flex-1 p-2 bg-gray-100 rounded text-sm">
                          export SERVER_ID={createdServer.id}
                        </code>
                        <Button
                          size="sm"
                          variant="outline"
                          onClick={() => copyToClipboard(`export SERVER_ID=${createdServer.id}`, 'Server ID env var')}
                        >
                          <Copy className="w-4 h-4" />
                        </Button>
                      </div>
                    </div>
                  </div>

                  <div>
                    <h4 className="font-medium mb-2">2. Agent Configuration</h4>
                    <div className="p-3 bg-gray-100 rounded text-sm">
                      <p className="mb-2"><strong>Server ID:</strong> {createdServer.id}</p>
                      <p className="mb-2"><strong>Repository:</strong> {selectedEnvironment?.repo_url}</p>
                      <p className="mb-2"><strong>Installation ID:</strong> {selectedEnvironment?.installation_id}</p>
                      {selectedEnvironment?.app_slug && (
                        <p className="mb-2"><strong>App Slug:</strong> {selectedEnvironment.app_slug}</p>
                      )}
                      {selectedEnvironment?.repository_id && (
                        <p className="mb-2"><strong>Repository ID:</strong> {selectedEnvironment.repository_id}</p>
                      )}
                      <p><strong>Environment:</strong> {selectedEnvironment?.name}</p>
                    </div>
                  </div>

                  <div>
                    <h4 className="font-medium mb-2">3. Complete Installation Command</h4>
                    <div className="flex items-center gap-2">
                      <code className="flex-1 p-2 bg-gray-100 rounded text-sm">
                        {createdServer.installation_info.example_cmd}
                      </code>
                      <Button
                        size="sm"
                        variant="outline"
                        onClick={() => copyToClipboard(createdServer.installation_info.example_cmd, 'Installation command')}
                      >
                        <Copy className="w-4 h-4" />
                      </Button>
                    </div>
                  </div>
                </div>
              </CardContent>
            </Card>

            <DialogFooter>
              <Button onClick={handleClose} className="w-full">
                Done
              </Button>
            </DialogFooter>
          </div>
        )}
      </DialogContent>
    </Dialog>
  )
} 
