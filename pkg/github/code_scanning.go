package github

import (
	"context"

	"github.com/github/github-mcp-server/pkg/translations"
	"github.com/google/go-github/v72/github"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

func GetCodeScanningAlert(getClient GetClientFn, t translations.TranslationHelperFunc) (tool mcp.Tool, handler server.ToolHandlerFunc) {
	return mcp.NewTool("get_code_scanning_alert",
			mcp.WithDescription(t("TOOL_GET_CODE_SCANNING_ALERT_DESCRIPTION", "Get details of a specific code scanning alert in a GitHub repository.")),
			mcp.WithToolAnnotation(mcp.ToolAnnotation{
				Title:        t("TOOL_GET_CODE_SCANNING_ALERT_USER_TITLE", "Get code scanning alert"),
				ReadOnlyHint: ToBoolPtr(true),
			}),
			WithOwnerParam(),
			WithRepoParam(),
			WithAlertNumberParam(),
		),
		func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			return ExecuteWithClientAndValidation(
				ctx,
				getClient,
				request,
				func(req mcp.CallToolRequest) error {
					_, _, _, err := ValidateOwnerRepoAlert(req)
					return err
				},
				func(ctx context.Context, client *github.Client) (*github.Alert, *github.Response, error) {
					owner, repo, alertNumber, _ := ValidateOwnerRepoAlert(request)
					return client.CodeScanning.GetAlert(ctx, owner, repo, int64(alertNumber))
				},
				"get alert",
			)
		}
}

func ListCodeScanningAlerts(getClient GetClientFn, t translations.TranslationHelperFunc) (tool mcp.Tool, handler server.ToolHandlerFunc) {
	return mcp.NewTool("list_code_scanning_alerts",
			mcp.WithDescription(t("TOOL_LIST_CODE_SCANNING_ALERTS_DESCRIPTION", "List code scanning alerts in a GitHub repository.")),
			mcp.WithToolAnnotation(mcp.ToolAnnotation{
				Title:        t("TOOL_LIST_CODE_SCANNING_ALERTS_USER_TITLE", "List code scanning alerts"),
				ReadOnlyHint: ToBoolPtr(true),
			}),
			WithOwnerParam(),
			WithRepoParam(),
			mcp.WithString("state",
				mcp.Description("Filter code scanning alerts by state. Defaults to open"),
				mcp.DefaultString("open"),
				mcp.Enum("open", "closed", "dismissed", "fixed"),
			),
			mcp.WithString("ref",
				mcp.Description("The Git reference for the results you want to list."),
			),
			mcp.WithString("severity",
				mcp.Description("Filter code scanning alerts by severity"),
				mcp.Enum("critical", "high", "medium", "low", "warning", "note", "error"),
			),
			mcp.WithString("tool_name",
				mcp.Description("The name of the tool used for code scanning."),
			),
		),
		func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			return ExecuteWithClientAndValidation(
				ctx,
				getClient,
				request,
				func(req mcp.CallToolRequest) error {
					_, _, err := ValidateOwnerRepo(req)
					return err
				},
				func(ctx context.Context, client *github.Client) ([]*github.Alert, *github.Response, error) {
					owner, repo, _ := ValidateOwnerRepo(request)
					ref, _ := OptionalParam[string](request, "ref")
					state, _ := OptionalParam[string](request, "state")
					severity, _ := OptionalParam[string](request, "severity")
					toolName, _ := OptionalParam[string](request, "tool_name")
					return client.CodeScanning.ListAlertsForRepo(ctx, owner, repo, &github.AlertListOptions{Ref: ref, State: state, Severity: severity, ToolName: toolName})
				},
				"list alerts",
			)
		}
}
