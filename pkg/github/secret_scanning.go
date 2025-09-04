package github

import (
	"context"
	"fmt"
	"net/http"

	"github.com/github/github-mcp-server/pkg/translations"
	"github.com/google/go-github/v72/github"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

func GetSecretScanningAlert(getClient GetClientFn, t translations.TranslationHelperFunc) (tool mcp.Tool, handler server.ToolHandlerFunc) {
	options := CreateStandardToolBase(
		t("TOOL_GET_SECRET_SCANNING_ALERT_DESCRIPTION", "Get details of a specific secret scanning alert in a GitHub repository."),
		t("TOOL_GET_SECRET_SCANNING_ALERT_USER_TITLE", "Get secret scanning alert"),
		true,
	)

	options = append(options, mcp.WithNumber("alertNumber",
		mcp.Required(),
		mcp.Description("The number of the alert."),
	))

	return mcp.NewTool("get_secret_scanning_alert", options...), func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		params, err := ExtractStandardParams(request)
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}

		alertNumber, err := RequiredInt(request, "alertNumber")
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}

		client, err := getClient(ctx)
		if err != nil {
			return nil, fmt.Errorf("failed to get GitHub client: %w", err)
		}

		alert, resp, err := client.SecretScanning.GetAlert(ctx, params.Owner, params.Repo, int64(alertNumber))
		if errResult, errReturn := HandleStandardAPIError(ctx, resp, err, fmt.Sprintf("failed to get alert with number '%d'", alertNumber)); errResult != nil {
			return errResult, errReturn
		}
		defer func() { _ = resp.Body.Close() }()

		if resp.StatusCode != http.StatusOK {
			return HandleHTTPError(resp, "failed to get alert")
		}

		return MarshalResponse(alert)
	}
}

func ListSecretScanningAlerts(getClient GetClientFn, t translations.TranslationHelperFunc) (tool mcp.Tool, handler server.ToolHandlerFunc) {
	options := CreateStandardToolBase(
		t("TOOL_LIST_SECRET_SCANNING_ALERTS_DESCRIPTION", "List secret scanning alerts in a GitHub repository."),
		t("TOOL_LIST_SECRET_SCANNING_ALERTS_USER_TITLE", "List secret scanning alerts"),
		true,
	)

	options = append(options,
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
	)

	return mcp.NewTool("list_secret_scanning_alerts", options...), func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		params, err := ExtractStandardParams(request)
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}

		state, err := OptionalParam[string](request, "state")
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		secretType, err := OptionalParam[string](request, "secret_type")
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		resolution, err := OptionalParam[string](request, "resolution")
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}

		client, err := getClient(ctx)
		if err != nil {
			return nil, fmt.Errorf("failed to get GitHub client: %w", err)
		}
		alerts, resp, err := client.SecretScanning.ListAlertsForRepo(ctx, params.Owner, params.Repo, &github.SecretScanningAlertListOptions{State: state, SecretType: secretType, Resolution: resolution})
		if errResult, errReturn := HandleStandardAPIError(ctx, resp, err, fmt.Sprintf("failed to list alerts for repository '%s/%s'", params.Owner, params.Repo)); errResult != nil {
			return errResult, errReturn
		}
		defer func() { _ = resp.Body.Close() }()

		if resp.StatusCode != http.StatusOK {
			return HandleHTTPError(resp, "failed to list alerts")
		}

		return MarshalResponse(alerts)
	}
}
