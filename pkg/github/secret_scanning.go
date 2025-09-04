package github

import (
	"context"

	"github.com/github/github-mcp-server/pkg/translations"
	"github.com/google/go-github/v72/github"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

func GetSecretScanningAlert(getClient GetClientFn, t translations.TranslationHelperFunc) (tool mcp.Tool, handler server.ToolHandlerFunc) {
	config := ToolConfig{
		Name:        "get_secret_scanning_alert",
		Description: t("TOOL_GET_SECRET_SCANNING_ALERT_DESCRIPTION", "Get details of a specific secret scanning alert in a GitHub repository."),
		Title:       t("TOOL_GET_SECRET_SCANNING_ALERT_USER_TITLE", "Get secret scanning alert"),
		ReadOnly:    true,
		Parameters: []ParameterConfig{
			{Name: "owner", Type: "string", Required: true, Description: "The owner of the repository."},
			{Name: "repo", Type: "string", Required: true, Description: "The name of the repository."},
			{Name: "alertNumber", Type: "number", Required: true, Description: "The number of the alert."},
		},
		Handler: ToolHandlerConfig{
			APICall: func(ctx context.Context, client *github.Client, params map[string]interface{}) (interface{}, *github.Response, error) {
				return client.SecretScanning.GetAlert(ctx, params["owner"].(string), params["repo"].(string), int64(params["alertNumber"].(int)))
			},
			ErrorPrefix: "failed to get alert",
		},
	}
	return CreateGitHubTool(getClient, t, config)
}

func ListSecretScanningAlerts(getClient GetClientFn, t translations.TranslationHelperFunc) (tool mcp.Tool, handler server.ToolHandlerFunc) {
	config := ToolConfig{
		Name:        "list_secret_scanning_alerts",
		Description: t("TOOL_LIST_SECRET_SCANNING_ALERTS_DESCRIPTION", "List secret scanning alerts in a GitHub repository."),
		Title:       t("TOOL_LIST_SECRET_SCANNING_ALERTS_USER_TITLE", "List secret scanning alerts"),
		ReadOnly:    true,
		Parameters: []ParameterConfig{
			{Name: "owner", Type: "string", Required: true, Description: "The owner of the repository."},
			{Name: "repo", Type: "string", Required: true, Description: "The name of the repository."},
			{Name: "state", Type: "string", Required: false, Description: "Filter by state", Enum: []string{"open", "resolved"}},
			{Name: "secret_type", Type: "string", Required: false, Description: "A comma-separated list of secret types to return. All default secret patterns are returned. To return generic patterns, pass the token name(s) in the parameter."},
			{Name: "resolution", Type: "string", Required: false, Description: "Filter by resolution", Enum: []string{"false_positive", "wont_fix", "revoked", "pattern_edited", "pattern_deleted", "used_in_tests"}},
		},
		Handler: ToolHandlerConfig{
			APICall: func(ctx context.Context, client *github.Client, params map[string]interface{}) (interface{}, *github.Response, error) {
				opts := &github.SecretScanningAlertListOptions{}
				if state, ok := params["state"].(string); ok {
					opts.State = state
				}
				if secretType, ok := params["secret_type"].(string); ok {
					opts.SecretType = secretType
				}
				if resolution, ok := params["resolution"].(string); ok {
					opts.Resolution = resolution
				}
				if page, ok := params["page"].(int); ok {
					opts.ListOptions.Page = page
				}
				if perPage, ok := params["perPage"].(int); ok {
					opts.ListOptions.PerPage = perPage
				}
				return client.SecretScanning.ListAlertsForRepo(ctx, params["owner"].(string), params["repo"].(string), opts)
			},
			ErrorPrefix: "failed to list alerts",
		},
	}
	return CreateGitHubTool(getClient, t, config)
}
