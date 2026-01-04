// Package server implements the Pullbase API server.
//
//	@title						Pullbase API
//	@version					1.0
//	@description				GitOps for Servers - Git-driven configuration management for VMs and bare-metal servers.
//	@termsOfService				https://pullbase.io/terms
//	@contact.name				Pullbase Support
//	@contact.url				https://github.com/pullbase/pullbase
//	@license.name				MIT
//	@license.url				https://opensource.org/licenses/MIT
//	@host						localhost:8080
//	@BasePath					/api/v1
//	@securityDefinitions.apikey	BearerAuth
//	@in							header
//	@name						Authorization
//	@description				JWT token obtained from /api/v1/auth/login. Format: "Bearer {token}"
//	@securityDefinitions.apikey	AgentAuth
//	@in							header
//	@name						Authorization
//	@description				Agent token generated when creating a server. Format: "Bearer {token}"
package server
