package github

import (
	"context"

	"github.com/github/github-mcp-server/pkg/translations"
	"github.com/google/go-github/v72/github"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

func GetCodeScanningAlert(getClient GetClientFn, t translations.TranslationHelperFunc) (tool mcp.Tool, handler server.ToolHandlerFunc) {
	config := ToolConfig{
		Name:        "get_code_scanning_alert",
		Description: t("TOOL_GET_CODE_SCANNING_ALERT_DESCRIPTION", "Get details of a specific code scanning alert in a GitHub repository."),
		Title:       t("TOOL_GET_CODE_SCANNING_ALERT_USER_TITLE", "Get code scanning alert"),
		ReadOnly:    true,
		Parameters: []ParameterConfig{
			{Name: "owner", Type: "string", Required: true, Description: "The owner of the repository."},
			{Name: "repo", Type: "string", Required: true, Description: "The name of the repository."},
			{Name: "alertNumber", Type: "number", Required: true, Description: "The number of the alert."},
		},
		Handler: ToolHandlerConfig{
			APICall: func(ctx context.Context, client *github.Client, params map[string]interface{}) (interface{}, *github.Response, error) {
				return client.CodeScanning.GetAlert(ctx, params["owner"].(string), params["repo"].(string), int64(params["alertNumber"].(int)))
			},
			ErrorPrefix: "failed to get alert",
		},
	}
	return CreateGitHubTool(getClient, t, config)
}

func ListCodeScanningAlerts(getClient GetClientFn, t translations.TranslationHelperFunc) (tool mcp.Tool, handler server.ToolHandlerFunc) {
	config := ToolConfig{
		Name:        "list_code_scanning_alerts",
		Description: t("TOOL_LIST_CODE_SCANNING_ALERTS_DESCRIPTION", "List code scanning alerts in a GitHub repository."),
		Title:       t("TOOL_LIST_CODE_SCANNING_ALERTS_USER_TITLE", "List code scanning alerts"),
		ReadOnly:    true,
		Parameters: []ParameterConfig{
			{Name: "owner", Type: "string", Required: true, Description: "The owner of the repository."},
			{Name: "repo", Type: "string", Required: true, Description: "The name of the repository."},
			{Name: "state", Type: "string", Required: false, Description: "Filter code scanning alerts by state. Defaults to open", Enum: []string{"open", "closed", "dismissed", "fixed"}},
			{Name: "ref", Type: "string", Required: false, Description: "The Git reference for the results you want to list."},
			{Name: "severity", Type: "string", Required: false, Description: "Filter code scanning alerts by severity", Enum: []string{"critical", "high", "medium", "low", "warning", "note", "error"}},
			{Name: "tool_name", Type: "string", Required: false, Description: "The name of the tool used for code scanning."},
		},
		Handler: ToolHandlerConfig{
			APICall: func(ctx context.Context, client *github.Client, params map[string]interface{}) (interface{}, *github.Response, error) {
				opts := &github.AlertListOptions{}
				if ref, ok := params["ref"].(string); ok {
					opts.Ref = ref
				}
				if state, ok := params["state"].(string); ok {
					opts.State = state
				}
				if severity, ok := params["severity"].(string); ok {
					opts.Severity = severity
				}
				if toolName, ok := params["tool_name"].(string); ok {
					opts.ToolName = toolName
				}
				if page, ok := params["page"].(int); ok {
					opts.ListOptions.Page = page
				}
				if perPage, ok := params["perPage"].(int); ok {
					opts.ListOptions.PerPage = perPage
				}
				return client.CodeScanning.ListAlertsForRepo(ctx, params["owner"].(string), params["repo"].(string), opts)
			},
			ErrorPrefix: "failed to list alerts",
		},
	}
	return CreateGitHubTool(getClient, t, config)
}
