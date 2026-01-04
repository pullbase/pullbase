import { useState } from "react";
import { useNavigate } from "react-router-dom";
import { environmentsApi } from "../lib/api";
import { Button } from "../components/ui/button";
import { Input } from "../components/ui/input";
import { Label } from "../components/ui/label";
import { Card, CardContent, CardHeader, CardTitle } from "../components/ui/card";
import { toast } from "sonner";

export default function NewEnvironmentPage() {
  const navigate = useNavigate();
  const [form, setForm] = useState({
    name: "",
    repo_url: "",
    branch: "main",
    deploy_path: "config.yaml",
    installation_id: "",
    app_slug: "",
    repository_id: "",
    webhook_secret: "",
    auto_reconcile: true,
  });
  const [isLoading, setIsLoading] = useState(false);

  const handleChange = (e: React.ChangeEvent<HTMLInputElement | HTMLSelectElement>) => {
    const { name, value, type } = e.target;
    if (type === "checkbox") {
      setForm((prev) => ({ ...prev, [name]: (e.target as HTMLInputElement).checked }));
    } else {
      setForm((prev) => ({ ...prev, [name]: value }));
    }
  };

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault();
    setIsLoading(true);
    try {
      const installationId = parseInt(form.installation_id, 10);
      if (isNaN(installationId) || installationId <= 0) {
        throw new Error("Installation ID must be a positive number");
      }

      await environmentsApi.create({
        name: form.name,
        repo_url: form.repo_url,
        branch: form.branch || 'main',
        deploy_path: form.deploy_path || 'config.yaml',
        installation_id: installationId,
        app_slug: form.app_slug || undefined,
        repository_id: form.repository_id ? parseInt(form.repository_id, 10) || undefined : undefined,
        webhook_secret: form.webhook_secret || undefined,
        auto_reconcile: form.auto_reconcile,
      });
      toast.success("Environment created successfully");
      navigate("/ui/environments");
    } catch (error: unknown) {
      if (
        error &&
        typeof error === "object" &&
        "response" in error &&
        error.response &&
        typeof error.response === "object" &&
        "data" in error.response &&
        error.response.data &&
        typeof error.response.data === "object" &&
        "error" in error.response.data
      ) {
        toast.error((error.response.data as { error?: string }).error || "Failed to create environment");
      } else {
        toast.error("Failed to create environment");
      }
    } finally {
      setIsLoading(false);
    }
  };

  return (
    <div className="max-w-xl mx-auto mt-8">
      <Card>
        <CardHeader>
          <CardTitle>Create New Environment</CardTitle>
          <p className="text-sm text-muted-foreground">
            Create an environment to monitor configuration drift and automatically reconcile server configurations with your GitHub repository via a GitHub App installation.
          </p>
        </CardHeader>
        <CardContent>
          <form onSubmit={handleSubmit} className="space-y-6">
            <div>
              <Label htmlFor="name">Name</Label>
              <Input
                id="name"
                name="name"
                value={form.name}
                onChange={handleChange}
                required
                placeholder="Production"
                disabled={isLoading}
              />
            </div>
            <div>
              <Label htmlFor="branch">Git Branch</Label>
              <Input
                id="branch"
                name="branch"
                value={form.branch}
                onChange={handleChange}
                placeholder="main"
                required
                disabled={isLoading}
              />
            </div>
            <div>
              <Label htmlFor="deploy_path">Config Path</Label>
              <Input
                id="deploy_path"
                name="deploy_path"
                value={form.deploy_path}
                onChange={handleChange}
                placeholder="config.yaml"
                required
                disabled={isLoading}
              />
            </div>
            <div>
              <Label htmlFor="repo_url">Repository URL</Label>
              <Input
                id="repo_url"
                name="repo_url"
                value={form.repo_url}
                onChange={handleChange}
                required
                placeholder="https://github.com/your/repo.git"
                disabled={isLoading}
              />
            </div>
            <div>
              <Label htmlFor="installation_id">GitHub Installation ID</Label>
              <Input
                id="installation_id"
                name="installation_id"
                value={form.installation_id}
                onChange={handleChange}
                required
                placeholder="12345678"
                disabled={isLoading}
                type="number"
              />
            </div>
            <div>
              <Label htmlFor="app_slug">GitHub App Slug (optional)</Label>
              <Input
                id="app_slug"
                name="app_slug"
                value={form.app_slug}
                onChange={handleChange}
                placeholder="your-app-slug"
                disabled={isLoading}
              />
            </div>
            <div>
              <Label htmlFor="repository_id">Repository ID (optional)</Label>
              <Input
                id="repository_id"
                name="repository_id"
                value={form.repository_id}
                onChange={handleChange}
                placeholder="Use only if restricting to a single repository"
                disabled={isLoading}
                type="number"
              />
            </div>
            <div>
              <Label htmlFor="webhook_secret">Webhook Secret (optional)</Label>
              <Input
                id="webhook_secret"
                name="webhook_secret"
                value={form.webhook_secret}
                onChange={handleChange}
                placeholder="Secret for webhook validation"
                disabled={isLoading}
              />
            </div>
            <div className="flex items-center gap-2">
              <input
                id="auto_reconcile"
                name="auto_reconcile"
                type="checkbox"
                checked={form.auto_reconcile}
                onChange={handleChange}
                disabled={isLoading}
                className="h-4 w-4"
              />
              <Label htmlFor="auto_reconcile">Enable Auto-Reconcile</Label>
            </div>
            <p className="text-sm text-muted-foreground">
              Automatically correct configuration drift to match the desired state from Git
            </p>
            <Button type="submit" className="w-full" disabled={isLoading}>
              {isLoading ? "Creating..." : "Create Environment"}
            </Button>
          </form>
        </CardContent>
      </Card>
    </div>
  );
}
