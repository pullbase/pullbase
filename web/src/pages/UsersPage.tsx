import { useCallback, useEffect, useState } from 'react'
import { toast } from 'sonner'
import { ShieldCheck, UserPlus, AlertTriangle, Filter, Trash2 } from 'lucide-react'
import { useAuth } from '../contexts/auth-context'
import { usersApi, type UserSummary } from '../lib/api'
import { Card, CardHeader, CardTitle, CardDescription, CardContent } from '../components/ui/card'
import { Label } from '../components/ui/label'
import { Input } from '../components/ui/input'
import { Button } from '../components/ui/button'
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from '../components/ui/table'
import { Badge } from '../components/ui/badge'
import { Skeleton } from '../components/ui/skeleton'
import {
  AlertDialog,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle,
} from '../components/ui/alert-dialog'

const PASSWORD_MIN_LENGTH = 12
const PAGE_SIZE = 20
const ROLE_FILTERS: Array<{ value: 'all' | 'admin' | 'user' | 'viewer'; label: string }> = [
  { value: 'all', label: 'All roles' },
  { value: 'admin', label: 'Admins' },
  { value: 'user', label: 'Users' },
  { value: 'viewer', label: 'Viewers' },
]

export default function UsersPage() {
  const { user } = useAuth()
  const [form, setForm] = useState({
    username: '',
    password: '',
    role: 'user',
  })
  const [isSubmitting, setIsSubmitting] = useState(false)
  const [lastCreatedUser, setLastCreatedUser] = useState<UserSummary | null>(null)
  const [users, setUsers] = useState<UserSummary[]>([])
  const [isLoadingUsers, setIsLoadingUsers] = useState(false)
  const [usersError, setUsersError] = useState<string | null>(null)
  const [roleFilter, setRoleFilter] = useState<'all' | 'admin' | 'user' | 'viewer'>('all')
  const [page, setPage] = useState(0)
  const [totalUsers, setTotalUsers] = useState(0)
  const [deleteDialog, setDeleteDialog] = useState<{ open: boolean; user: UserSummary | null }>({ open: false, user: null })
  const [confirmUsername, setConfirmUsername] = useState('')
  const [isDeletingUser, setIsDeletingUser] = useState(false)
  const totalPages = Math.max(1, Math.ceil(totalUsers / PAGE_SIZE))

  const loadUsers = useCallback(async () => {
    if (user?.role !== 'admin') {
      return
    }
    try {
      setIsLoadingUsers(true)
      setUsersError(null)
      const response = await usersApi.list({
        limit: PAGE_SIZE,
        offset: page * PAGE_SIZE,
        role: roleFilter === 'all' ? undefined : roleFilter,
      })
      setUsers(response.users)
      setTotalUsers(response.total ?? response.users.length)
    } catch (error: unknown) {
      const message =
        (error as { response?: { data?: { error?: string } } }).response?.data?.error ||
        'Failed to load users'
      setUsersError(message)
    } finally {
      setIsLoadingUsers(false)
    }
  }, [user?.role, page, roleFilter])

  useEffect(() => {
    void loadUsers()
  }, [loadUsers])

  if (user?.role !== 'admin') {
    return (
      <div className="max-w-2xl mx-auto">
        <Card className="border border-amber-200/70 bg-amber-50">
          <CardHeader>
            <CardTitle className="flex items-center gap-2 text-amber-700">
              <AlertTriangle className="h-5 w-5" />
              Restricted Area
            </CardTitle>
            <CardDescription className="text-amber-700/90">
              User provisioning is limited to administrators. Ask an existing admin to create accounts for your team.
            </CardDescription>
          </CardHeader>
        </Card>
      </div>
    )
  }

  const handleSubmit = async (event: React.FormEvent<HTMLFormElement>) => {
    event.preventDefault()

    if (!form.username.trim()) {
      toast.error('Username is required')
      return
    }

    if (form.password.length < PASSWORD_MIN_LENGTH) {
      toast.error(`Password must be at least ${PASSWORD_MIN_LENGTH} characters long`)
      return
    }

    setIsSubmitting(true)
    try {
      const created = await usersApi.create({
        username: form.username.trim(),
        password: form.password,
        role: form.role,
      })

      toast.success(`User ${created.username} created`)
      setLastCreatedUser(created)
      setPage(0)
      setUsers((prev) => {
        const exists = prev.some((u) => u.id === created.id)
        if (exists) {
          return prev.map((u) => (u.id === created.id ? created : u))
        }
        return [created, ...prev]
      })
      setForm({
        username: '',
        password: '',
        role: 'user',
      })
    } catch (error: unknown) {
      const message =
        (error as { response?: { data?: { error?: string } } }).response?.data?.error ||
        'Failed to create user'
      toast.error(message)
    } finally {
      setIsSubmitting(false)
      // Ensure the list reflects any server-side defaults (e.g. created_at ordering)
      void loadUsers()
    }
  }

  const handleChange = (event: React.ChangeEvent<HTMLInputElement | HTMLSelectElement>) => {
    const { name, value } = event.target
    setForm((prev) => ({
      ...prev,
      [name]: value,
    }))
  }

  const handleConfirmDelete = async () => {
    if (!deleteDialog.user) {
      return
    }

    setIsDeletingUser(true)
    try {
      await usersApi.delete(deleteDialog.user.id, confirmUsername.trim())
      toast.success(`User ${deleteDialog.user.username} deleted`)

      if (lastCreatedUser?.id === deleteDialog.user.id) {
        setLastCreatedUser(null)
      }

      if (page > 0 && users.length <= 1) {
        setPage((prev) => Math.max(0, prev - 1))
      } else {
        await loadUsers()
      }

      setDeleteDialog({ open: false, user: null })
      setConfirmUsername('')
    } catch (error: unknown) {
      const message =
        (error as { response?: { data?: { error?: string } } }).response?.data?.error ||
        'Failed to delete user'
      toast.error(message)
    } finally {
      setIsDeletingUser(false)
    }
  }

  return (
    <div className="mx-auto max-w-3xl space-y-8">
      <div className="space-y-2">
        <h1 className="text-3xl font-bold text-foreground">Operator Accounts</h1>
        <p className="text-muted-foreground">
          Provision additional administrators or read-only viewers for Pullbase. Use long, unique passwords and share them through your preferred secure channel.
        </p>
      </div>

      <Card>
        <CardHeader>
          <CardTitle className="flex items-center gap-2">
            <UserPlus className="h-5 w-5" />
            Create a User
          </CardTitle>
          <CardDescription>
            Accounts created here gain access immediately. Passwords are never stored in plain text and are only shown to you once.
          </CardDescription>
        </CardHeader>
        <CardContent>
          <form className="space-y-6" onSubmit={handleSubmit}>
            <div className="grid gap-4 md:grid-cols-2">
              <div className="space-y-2">
                <Label htmlFor="username">Username</Label>
                <Input
                  id="username"
                  name="username"
                  value={form.username}
                  onChange={handleChange}
                  placeholder="ops-admin"
                  autoComplete="off"
                  required
                  minLength={3}
                  disabled={isSubmitting}
                />
                <p className="text-xs text-muted-foreground">
                  Letters, numbers, dots, dashes, and underscores only.
                </p>
              </div>

              <div className="space-y-2">
                <Label htmlFor="role">Role</Label>
                <select
                  id="role"
                  name="role"
                  value={form.role}
                  onChange={handleChange}
                  disabled={isSubmitting}
                  className="h-10 w-full rounded-md border border-input bg-background px-3 py-2 text-sm ring-offset-background focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring focus-visible:ring-offset-2"
                >
                  <option value="user">User — can manage servers & environments</option>
                  <option value="viewer">Viewer — read-only dashboard access</option>
                  <option value="admin">Admin — full access, including user management</option>
                </select>
              </div>
            </div>

            <div className="space-y-2">
              <Label htmlFor="password">Temporary Password</Label>
              <Input
                id="password"
                name="password"
                type="password"
                value={form.password}
                onChange={handleChange}
                placeholder="Use a unique passphrase"
                required
                minLength={PASSWORD_MIN_LENGTH}
                disabled={isSubmitting}
                allowPasswordToggle
              />
              <p className="text-xs text-muted-foreground">
                Minimum {PASSWORD_MIN_LENGTH} characters. Encourage recipients to rotate their password after first login.
              </p>
            </div>

            {lastCreatedUser && (
              <div className="rounded-md border border-emerald-300 bg-emerald-50 p-4 text-sm text-emerald-700">
                <p>
                  <strong>{lastCreatedUser.username}</strong> ({lastCreatedUser.role}) created successfully.
                </p>
                <p className="mt-1">
                  Share the username and password securely; this message won&apos;t appear again once you leave the page.
                </p>
              </div>
            )}

            <Button type="submit" disabled={isSubmitting} className="w-full md:w-auto">
              {isSubmitting ? 'Creating…' : 'Create user'}
            </Button>
          </form>
        </CardContent>
      </Card>

      <AlertDialog
        open={deleteDialog.open}
        onOpenChange={(open: boolean) => {
          if (!open && !isDeletingUser) {
            setDeleteDialog({ open: false, user: null })
            setConfirmUsername('')
          }
        }}
      >
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle>Delete User</AlertDialogTitle>
            <AlertDialogDescription className="space-y-2 text-sm text-muted-foreground leading-relaxed">
              <p>
                This action removes <span className="font-medium text-foreground">{deleteDialog.user?.username}</span> from Pullbase.
                They will lose access immediately. This cannot be undone.
              </p>
              <p>Type the username to confirm.</p>
            </AlertDialogDescription>
          </AlertDialogHeader>
          <div className="space-y-3">
            <Label htmlFor="delete-confirm-username">Confirm username</Label>
            <Input
              id="delete-confirm-username"
              value={confirmUsername}
              onChange={(event) => setConfirmUsername(event.target.value)}
              placeholder={deleteDialog.user?.username ?? 'username'}
              autoFocus
              disabled={isDeletingUser}
            />
          </div>
          <AlertDialogFooter>
            <AlertDialogCancel disabled={isDeletingUser}>Cancel</AlertDialogCancel>
            <Button
              variant="destructive"
              onClick={() => void handleConfirmDelete()}
              disabled={
                !deleteDialog.user ||
                confirmUsername.trim() !== deleteDialog.user.username ||
                isDeletingUser
              }
            >
              {isDeletingUser ? 'Deleting…' : 'Delete user'}
            </Button>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>

      <Card>
        <CardHeader className="flex flex-col gap-4 md:flex-row md:items-center md:justify-between">
          <div>
            <CardTitle className="flex items-center gap-2">
              <Filter className="h-5 w-5" />
              Existing Users
            </CardTitle>
            <CardDescription>Review currently active accounts and their permissions.</CardDescription>
          </div>
          <div className="flex w-full flex-col gap-2 md:w-auto md:flex-row md:items-center">
            <Label htmlFor="role-filter" className="text-sm font-medium text-muted-foreground">
              Filter by role
            </Label>
            <select
              id="role-filter"
              value={roleFilter}
              onChange={(event) => {
                const value = event.target.value as 'all' | 'admin' | 'user' | 'viewer'
                setRoleFilter(value)
                setPage(0)
              }}
              className="h-9 rounded-md border border-input bg-background px-3 text-sm ring-offset-background focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring focus-visible:ring-offset-2 md:w-40"
              disabled={isLoadingUsers}
            >
              {ROLE_FILTERS.map((option) => (
                <option key={option.value} value={option.value}>
                  {option.label}
                </option>
              ))}
            </select>
          </div>
        </CardHeader>
        <CardContent>
          {isLoadingUsers ? (
            <div className="space-y-3">
              <Skeleton className="h-6 w-48" />
              <Skeleton className="h-10 w-full" />
              <Skeleton className="h-10 w-full" />
            </div>
          ) : usersError ? (
            <div className="rounded-md border border-destructive/40 bg-destructive/10 p-4 text-sm text-destructive">
              <p>{usersError}</p>
              <Button variant="outline" size="sm" className="mt-3" onClick={() => void loadUsers()}>
                Retry
              </Button>
            </div>
          ) : users.length === 0 ? (
            <div className="space-y-3 text-sm text-muted-foreground">
              <p>
                {totalUsers === 0
                  ? 'No users found. Create a user above to get started.'
                  : 'No users match the current filters or page. Adjust the filters or return to the first page.'}
              </p>
              {totalUsers > 0 && (
                <Button variant="outline" size="sm" className="w-fit" onClick={() => setPage(0)}>
                  Go to first page
                </Button>
              )}
            </div>
          ) : (
            <div className="overflow-hidden rounded-md border border-border">
              <Table>
                <TableHeader>
                  <TableRow>
                    <TableHead className="w-16">ID</TableHead>
                    <TableHead>Username</TableHead>
                    <TableHead>Role</TableHead>
                    <TableHead className="w-32 text-right">Actions</TableHead>
                  </TableRow>
                </TableHeader>
                <TableBody>
                  {users.map((account) => (
                    <TableRow key={account.id}>
                      <TableCell className="font-mono text-sm text-muted-foreground">{account.id}</TableCell>
                      <TableCell className="font-medium">{account.username}</TableCell>
                      <TableCell>
                        <Badge variant={account.role === 'admin' ? 'default' : 'secondary'} className="capitalize">
                          {account.role}
                        </Badge>
                      </TableCell>
                      <TableCell className="text-right">
                        <Button
                          variant="destructive"
                          size="sm"
                          onClick={() => {
                            setDeleteDialog({ open: true, user: account })
                            setConfirmUsername('')
                          }}
                          disabled={account.id === user?.id || isLoadingUsers}
                          title={account.id === user?.id ? "You can't delete your own account" : undefined}
                        >
                          <Trash2 className="h-4 w-4" />
                        </Button>
                      </TableCell>
                    </TableRow>
                  ))}
                </TableBody>
              </Table>
              <div className="flex flex-col gap-3 px-4 py-3 text-xs text-muted-foreground md:flex-row md:items-center md:justify-between">
                <span>
                  Showing {users.length} of {totalUsers} user{totalUsers === 1 ? '' : 's'}.
                </span>
                <div className="flex items-center gap-2 text-sm">
                  <Button
                    variant="outline"
                    size="sm"
                    onClick={() => setPage((prev) => Math.max(0, prev - 1))}
                    disabled={page === 0 || isLoadingUsers}
                  >
                    Previous
                  </Button>
                  <span>
                    Page {page + 1} of {totalPages}
                  </span>
                  <Button
                    variant="outline"
                    size="sm"
                    onClick={() => setPage((prev) => Math.min(totalPages - 1, prev + 1))}
                    disabled={isLoadingUsers || page + 1 >= totalPages}
                  >
                    Next
                  </Button>
                </div>
              </div>
            </div>
          )}
        </CardContent>
      </Card>

      <Card>
        <CardHeader>
          <CardTitle className="flex items-center gap-2">
            <ShieldCheck className="h-5 w-5" />
            Recommended Hand-off
          </CardTitle>
          <CardDescription>
            Keep operator provisioning simple and auditable.
          </CardDescription>
        </CardHeader>
        <CardContent className="space-y-3 text-sm text-muted-foreground">
          <ul className="list-disc space-y-2 pl-5">
            <li>Create credentials on demand and deliver them through your trusted secret-sharing method.</li>
            <li>Encourage new users to log in immediately and rotate the password via CLI once password management endpoints are available.</li>
            <li>Use the CLI (<code>pullbasectl users create</code>) for automated workflows or if you prefer terminal-based provisioning.</li>
          </ul>
        </CardContent>
      </Card>
    </div>
  )
}
