package main

import (
	"github.com/spf13/cobra"
)

func newRootCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:           "pullbasectl",
		Short:         "Pullbase command-line interface",
		SilenceUsage:  true,
		SilenceErrors: true,
	}

	cmd.CompletionOptions.DisableDefaultCmd = true

	cmd.AddCommand(
		newBootstrapCommand(),
		newTokensCommand(),
		newUsersCommand(),
		newAuthCommand(),
		newGitHubAppCommand(),
		newServersCommand(),
		newEnvironmentsCommand(),
		newStatusCommand(),
		newValidateConfigCommand(),
	)

	return cmd
}

func newBootstrapCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:           "bootstrap",
		Short:         "Bootstrap Pullbase services",
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			return cmd.Help()
		},
	}

	cmd.AddCommand(newBootstrapWizardCommand())

	return cmd
}

func newBootstrapWizardCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:                "wizard",
		Short:              "Interactive first-run setup (admin and GitHub App)",
		DisableFlagParsing: true,
		SilenceUsage:       true,
		SilenceErrors:      true,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runBootstrapWizard(args)
		},
	}

	cmd.SetHelpFunc(func(cmd *cobra.Command, args []string) {
		_ = runBootstrapWizard([]string{"--help"})
	})

	return cmd
}

func newTokensCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:           "tokens",
		Short:         "Manage agent tokens",
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			return cmd.Help()
		},
	}

	cmd.AddCommand(
		newTokensListCommand(),
		newTokensCreateCommand(),
		newTokensRevokeCommand(),
	)

	return cmd
}

func newTokensListCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:                "list",
		Short:              "List tokens for a server",
		DisableFlagParsing: true,
		SilenceUsage:       true,
		SilenceErrors:      true,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runTokensList(args)
		},
	}

	cmd.SetHelpFunc(func(cmd *cobra.Command, args []string) {
		_ = runTokensList([]string{"--help"})
	})

	return cmd
}

func newTokensCreateCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:                "create",
		Short:              "Create a new agent token",
		DisableFlagParsing: true,
		SilenceUsage:       true,
		SilenceErrors:      true,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runTokensCreate(args)
		},
	}

	cmd.SetHelpFunc(func(cmd *cobra.Command, args []string) {
		_ = runTokensCreate([]string{"--help"})
	})

	return cmd
}

func newTokensRevokeCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:                "revoke",
		Short:              "Revoke an existing agent token",
		DisableFlagParsing: true,
		SilenceUsage:       true,
		SilenceErrors:      true,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runTokensRevoke(args)
		},
	}

	cmd.SetHelpFunc(func(cmd *cobra.Command, args []string) {
		_ = runTokensRevoke([]string{"--help"})
	})

	return cmd
}

func newUsersCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:           "users",
		Short:         "Manage Pullbase users",
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			return cmd.Help()
		},
	}

	cmd.AddCommand(
		newUsersListCommand(),
		newUsersCreateCommand(),
		newUsersDeleteCommand(),
	)

	return cmd
}

func newUsersListCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:                "list",
		Short:              "List active Pullbase users",
		DisableFlagParsing: true,
		SilenceUsage:       true,
		SilenceErrors:      true,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runUsersList(args)
		},
	}

	cmd.SetHelpFunc(func(cmd *cobra.Command, args []string) {
		_ = runUsersList([]string{"--help"})
	})

	return cmd
}

func newUsersCreateCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:                "create",
		Short:              "Create a new Pullbase user",
		DisableFlagParsing: true,
		SilenceUsage:       true,
		SilenceErrors:      true,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runUsersCreate(args)
		},
	}

	cmd.SetHelpFunc(func(cmd *cobra.Command, args []string) {
		_ = runUsersCreate([]string{"--help"})
	})

	return cmd
}

func newUsersDeleteCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:                "delete",
		Short:              "Delete an existing Pullbase user",
		DisableFlagParsing: true,
		SilenceUsage:       true,
		SilenceErrors:      true,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runUsersDelete(args)
		},
	}

	cmd.SetHelpFunc(func(cmd *cobra.Command, args []string) {
		_ = runUsersDelete([]string{"--help"})
	})

	return cmd
}

func newAuthCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:           "auth",
		Short:         "Authentication helpers",
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			return cmd.Help()
		},
	}

	cmd.AddCommand(
		newAuthBootstrapAdminCommand(),
		newAuthLoginCommand(),
	)

	return cmd
}

func newAuthBootstrapAdminCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:                "bootstrap-admin",
		Short:              "Create the initial Pullbase admin using the bootstrap secret",
		DisableFlagParsing: true,
		SilenceUsage:       true,
		SilenceErrors:      true,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runAuthBootstrapAdmin(args)
		},
	}

	cmd.SetHelpFunc(func(cmd *cobra.Command, args []string) {
		_ = runAuthBootstrapAdmin([]string{"--help"})
	})

	return cmd
}

func newAuthLoginCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:                "login",
		Short:              "Exchange credentials for an admin JWT",
		DisableFlagParsing: true,
		SilenceUsage:       true,
		SilenceErrors:      true,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runAuthLogin(args)
		},
	}

	cmd.SetHelpFunc(func(cmd *cobra.Command, args []string) {
		_ = runAuthLogin([]string{"--help"})
	})

	return cmd
}

func newGitHubAppCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:           "github-app",
		Short:         "GitHub App operations",
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			return cmd.Help()
		},
	}

	cmd.AddCommand(
		newGitHubAppBootstrapCommand(),
		newGitHubAppStatusCommand(),
	)

	return cmd
}

func newGitHubAppBootstrapCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:                "bootstrap",
		Short:              "Validate GitHub App credentials and optionally register an environment",
		DisableFlagParsing: true,
		SilenceUsage:       true,
		SilenceErrors:      true,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runGitHubAppBootstrap(args)
		},
	}

	cmd.SetHelpFunc(func(cmd *cobra.Command, args []string) {
		_ = runGitHubAppBootstrap([]string{"--help"})
	})

	return cmd
}

func newGitHubAppStatusCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:                "status",
		Short:              "Show GitHub App environment status",
		DisableFlagParsing: true,
		SilenceUsage:       true,
		SilenceErrors:      true,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runGitHubAppStatus(args)
		},
	}

	cmd.SetHelpFunc(func(cmd *cobra.Command, args []string) {
		_ = runGitHubAppStatus([]string{"--help"})
	})

	return cmd
}

func newServersCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:           "servers",
		Short:         "Manage Pullbase servers",
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			return cmd.Help()
		},
	}

	cmd.AddCommand(
		newServersListCommand(),
		newServersCreateCommand(),
		newServersGetCommand(),
		newServersDeleteCommand(),
		newServersInstallScriptCommand(),
	)

	return cmd
}

func newServersListCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:                "list",
		Short:              "List all servers",
		DisableFlagParsing: true,
		SilenceUsage:       true,
		SilenceErrors:      true,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runServersList(args)
		},
	}

	cmd.SetHelpFunc(func(cmd *cobra.Command, args []string) {
		_ = runServersList([]string{"--help"})
	})

	return cmd
}

func newServersCreateCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:                "create",
		Short:              "Create a new server",
		DisableFlagParsing: true,
		SilenceUsage:       true,
		SilenceErrors:      true,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runServersCreate(args)
		},
	}

	cmd.SetHelpFunc(func(cmd *cobra.Command, args []string) {
		_ = runServersCreate([]string{"--help"})
	})

	return cmd
}

func newServersGetCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:                "get",
		Short:              "Get server details",
		DisableFlagParsing: true,
		SilenceUsage:       true,
		SilenceErrors:      true,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runServersGet(args)
		},
	}

	cmd.SetHelpFunc(func(cmd *cobra.Command, args []string) {
		_ = runServersGet([]string{"--help"})
	})

	return cmd
}

func newServersDeleteCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:                "delete",
		Short:              "Delete a server",
		DisableFlagParsing: true,
		SilenceUsage:       true,
		SilenceErrors:      true,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runServersDelete(args)
		},
	}

	cmd.SetHelpFunc(func(cmd *cobra.Command, args []string) {
		_ = runServersDelete([]string{"--help"})
	})

	return cmd
}

func newServersInstallScriptCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:                "install-script",
		Short:              "Generate install script for a server",
		DisableFlagParsing: true,
		SilenceUsage:       true,
		SilenceErrors:      true,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runServersInstallScript(args)
		},
	}

	cmd.SetHelpFunc(func(cmd *cobra.Command, args []string) {
		_ = runServersInstallScript([]string{"--help"})
	})

	return cmd
}

func newEnvironmentsCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:           "environments",
		Short:         "Manage Pullbase environments",
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			return cmd.Help()
		},
	}

	cmd.AddCommand(
		newEnvironmentsListCommand(),
		newEnvironmentsCreateCommand(),
		newEnvironmentsGetCommand(),
		newEnvironmentsDeleteCommand(),
		newEnvironmentsRollbackCommand(),
		newEnvironmentsRollbackListCommand(),
	)

	return cmd
}

func newEnvironmentsListCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:                "list",
		Short:              "List all environments",
		DisableFlagParsing: true,
		SilenceUsage:       true,
		SilenceErrors:      true,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runEnvironmentsList(args)
		},
	}

	cmd.SetHelpFunc(func(cmd *cobra.Command, args []string) {
		_ = runEnvironmentsList([]string{"--help"})
	})

	return cmd
}

func newEnvironmentsCreateCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:                "create",
		Short:              "Create a new environment",
		DisableFlagParsing: true,
		SilenceUsage:       true,
		SilenceErrors:      true,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runEnvironmentsCreate(args)
		},
	}

	cmd.SetHelpFunc(func(cmd *cobra.Command, args []string) {
		_ = runEnvironmentsCreate([]string{"--help"})
	})

	return cmd
}

func newEnvironmentsGetCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:                "get",
		Short:              "Get environment details",
		DisableFlagParsing: true,
		SilenceUsage:       true,
		SilenceErrors:      true,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runEnvironmentsGet(args)
		},
	}

	cmd.SetHelpFunc(func(cmd *cobra.Command, args []string) {
		_ = runEnvironmentsGet([]string{"--help"})
	})

	return cmd
}

func newEnvironmentsDeleteCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:                "delete",
		Short:              "Delete an environment",
		DisableFlagParsing: true,
		SilenceUsage:       true,
		SilenceErrors:      true,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runEnvironmentsDelete(args)
		},
	}

	cmd.SetHelpFunc(func(cmd *cobra.Command, args []string) {
		_ = runEnvironmentsDelete([]string{"--help"})
	})

	return cmd
}

func newEnvironmentsRollbackCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:                "rollback",
		Short:              "Rollback environment to a specific commit",
		DisableFlagParsing: true,
		SilenceUsage:       true,
		SilenceErrors:      true,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runEnvironmentsRollback(args)
		},
	}

	cmd.SetHelpFunc(func(cmd *cobra.Command, args []string) {
		_ = runEnvironmentsRollback([]string{"--help"})
	})

	return cmd
}

func newEnvironmentsRollbackListCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:                "rollback-list",
		Short:              "List rollback history for an environment",
		DisableFlagParsing: true,
		SilenceUsage:       true,
		SilenceErrors:      true,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runEnvironmentsRollbackList(args)
		},
	}

	cmd.SetHelpFunc(func(cmd *cobra.Command, args []string) {
		_ = runEnvironmentsRollbackList([]string{"--help"})
	})

	return cmd
}

func newStatusCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:                "status",
		Short:              "Show fleet status overview",
		DisableFlagParsing: true,
		SilenceUsage:       true,
		SilenceErrors:      true,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runStatus(args)
		},
	}

	cmd.SetHelpFunc(func(cmd *cobra.Command, args []string) {
		_ = runStatus([]string{"--help"})
	})

	return cmd
}

func newValidateConfigCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:                "validate-config",
		Short:              "Validate a config.yaml file locally",
		DisableFlagParsing: true,
		SilenceUsage:       true,
		SilenceErrors:      true,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runValidateConfig(args)
		},
	}

	cmd.SetHelpFunc(func(cmd *cobra.Command, args []string) {
		_ = runValidateConfig([]string{"--help"})
	})

	return cmd
}
