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

func GetCodeScanningAlert(getClient GetClientFn, t translations.TranslationHelperFunc) (tool mcp.Tool, handler server.ToolHandlerFunc) {
	options := CreateStandardToolBase(
		t("TOOL_GET_CODE_SCANNING_ALERT_DESCRIPTION", "Get details of a specific code scanning alert in a GitHub repository."),
		t("TOOL_GET_CODE_SCANNING_ALERT_USER_TITLE", "Get code scanning alert"),
		true,
	)

	options = append(options, mcp.WithNumber("alertNumber",
		mcp.Required(),
		mcp.Description("The number of the alert."),
	))

	return mcp.NewTool("get_code_scanning_alert", options...), func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
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

		alert, resp, err := client.CodeScanning.GetAlert(ctx, params.Owner, params.Repo, int64(alertNumber))
		if errResult, errReturn := HandleStandardAPIError(ctx, resp, err, "failed to get alert"); errResult != nil {
			return errResult, errReturn
		}
		defer func() { _ = resp.Body.Close() }()

		if resp.StatusCode != http.StatusOK {
			return HandleHTTPError(resp, "failed to get alert")
		}

		return MarshalResponse(alert)
	}
}

func ListCodeScanningAlerts(getClient GetClientFn, t translations.TranslationHelperFunc) (tool mcp.Tool, handler server.ToolHandlerFunc) {
	options := CreateStandardToolBase(
		t("TOOL_LIST_CODE_SCANNING_ALERTS_DESCRIPTION", "List code scanning alerts in a GitHub repository."),
		t("TOOL_LIST_CODE_SCANNING_ALERTS_USER_TITLE", "List code scanning alerts"),
		true,
	)

	options = append(options,
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
	)

	return mcp.NewTool("list_code_scanning_alerts", options...), func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		params, err := ExtractStandardParams(request)
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}

		ref, err := OptionalParam[string](request, "ref")
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		state, err := OptionalParam[string](request, "state")
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		severity, err := OptionalParam[string](request, "severity")
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		toolName, err := OptionalParam[string](request, "tool_name")
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}

		client, err := getClient(ctx)
		if err != nil {
			return nil, fmt.Errorf("failed to get GitHub client: %w", err)
		}
		alerts, resp, err := client.CodeScanning.ListAlertsForRepo(ctx, params.Owner, params.Repo, &github.AlertListOptions{Ref: ref, State: state, Severity: severity, ToolName: toolName})
		if errResult, errReturn := HandleStandardAPIError(ctx, resp, err, "failed to list alerts"); errResult != nil {
			return errResult, errReturn
		}
		defer func() { _ = resp.Body.Close() }()

		if resp.StatusCode != http.StatusOK {
			return HandleHTTPError(resp, "failed to list alerts")
		}

		return MarshalResponse(alerts)
	}
}
