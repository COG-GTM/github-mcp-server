package github

import (
	"context"

	"github.com/github/github-mcp-server/pkg/translations"
	"github.com/google/go-github/v72/github"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

func GetSecretScanningAlert(getClient GetClientFn, t translations.TranslationHelperFunc) (tool mcp.Tool, handler server.ToolHandlerFunc) {
	return mcp.NewTool(
			"get_secret_scanning_alert",
			mcp.WithDescription(t("TOOL_GET_SECRET_SCANNING_ALERT_DESCRIPTION", "Get details of a specific secret scanning alert in a GitHub repository.")),
			mcp.WithToolAnnotation(mcp.ToolAnnotation{
				Title:        t("TOOL_GET_SECRET_SCANNING_ALERT_USER_TITLE", "Get secret scanning alert"),
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
				func(ctx context.Context, client *github.Client) (*github.SecretScanningAlert, *github.Response, error) {
					owner, repo, alertNumber, _ := ValidateOwnerRepoAlert(request)
					return client.SecretScanning.GetAlert(ctx, owner, repo, int64(alertNumber))
				},
				"get alert",
			)
		}
}

func ListSecretScanningAlerts(getClient GetClientFn, t translations.TranslationHelperFunc) (tool mcp.Tool, handler server.ToolHandlerFunc) {
	return mcp.NewTool(
			"list_secret_scanning_alerts",
			mcp.WithDescription(t("TOOL_LIST_SECRET_SCANNING_ALERTS_DESCRIPTION", "List secret scanning alerts in a GitHub repository.")),
			mcp.WithToolAnnotation(mcp.ToolAnnotation{
				Title:        t("TOOL_LIST_SECRET_SCANNING_ALERTS_USER_TITLE", "List secret scanning alerts"),
				ReadOnlyHint: ToBoolPtr(true),
			}),
			WithOwnerParam(),
			WithRepoParam(),
			mcp.WithString("state",
				mcp.Description("Filter by state"),
				mcp.Enum("open", "resolved"),
			),
			mcp.WithString("secret_type",
				mcp.Description("A comma-separated list of secret types to return. All default secret patterns are returned. To return generic patterns, pass the token name(s) in the parameter."),
			),
			mcp.WithString("resolution",
				mcp.Description("Filter by resolution"),
				mcp.Enum("false_positive", "wont_fix", "revoked", "pattern_edited", "pattern_deleted", "used_in_tests"),
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
				func(ctx context.Context, client *github.Client) ([]*github.SecretScanningAlert, *github.Response, error) {
					owner, repo, _ := ValidateOwnerRepo(request)
					state, _ := OptionalParam[string](request, "state")
					secretType, _ := OptionalParam[string](request, "secret_type")
					resolution, _ := OptionalParam[string](request, "resolution")
					return client.SecretScanning.ListAlertsForRepo(ctx, owner, repo, &github.SecretScanningAlertListOptions{State: state, SecretType: secretType, Resolution: resolution})
				},
				"list alerts",
			)
		}
}
