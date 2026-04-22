package github

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"time"

	ghErrors "github.com/github/github-mcp-server/pkg/errors"
	"github.com/github/github-mcp-server/pkg/translations"
	"github.com/google/go-github/v72/github"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

const (
	FilterDefault           = "default"
	FilterIncludeRead       = "include_read_notifications"
	FilterOnlyParticipating = "only_participating"
)

// Error message formats reused by the notification tool handlers when
// wrapping common failures.
const (
	errFailedGitHubClient    = "failed to get GitHub client: %w"
	errFailedReadResponse    = "failed to read response body: %w"
	errFailedMarshalResponse = "failed to marshal response: %w"
)

// Enum values for ManageNotificationSubscription action
const (
	NotificationActionIgnore = "ignore"
	NotificationActionWatch  = "watch"
	NotificationActionDelete = "delete"
)

const (
	RepositorySubscriptionActionWatch  = "watch"
	RepositorySubscriptionActionIgnore = "ignore"
	RepositorySubscriptionActionDelete = "delete"
)

// extractOptionalStringParams reads each named optional string parameter from
// the request. If any parameter fails to parse, a tool error result is
// returned so the caller can forward it to the client.
func extractOptionalStringParams(request mcp.CallToolRequest, names []string) (map[string]string, *mcp.CallToolResult) {
	result := make(map[string]string, len(names))
	for _, name := range names {
		val, err := OptionalParam[string](request, name)
		if err != nil {
			return nil, mcp.NewToolResultError(err.Error())
		}
		result[name] = val
	}
	return result, nil
}

// extractRequiredStringParams reads each named required string parameter from
// the request. If any parameter is missing or fails to parse, a tool error
// result is returned.
func extractRequiredStringParams(request mcp.CallToolRequest, names []string) (map[string]string, *mcp.CallToolResult) {
	result := make(map[string]string, len(names))
	for _, name := range names {
		val, err := RequiredParam[string](request, name)
		if err != nil {
			return nil, mcp.NewToolResultError(err.Error())
		}
		result[name] = val
	}
	return result, nil
}

// parseOptionalTime parses an optional RFC3339 time parameter. An empty input
// returns (nil, nil). A parse failure returns a tool error result describing
// the bad input.
func parseOptionalTime(value, paramName string) (*time.Time, *mcp.CallToolResult) {
	if value == "" {
		return nil, nil
	}
	t, err := time.Parse(time.RFC3339, value)
	if err != nil {
		return nil, mcp.NewToolResultError(fmt.Sprintf("invalid %s time format, should be RFC3339/ISO8601: %v", paramName, err))
	}
	return &t, nil
}

// applyOptionalTime parses an optional RFC3339 time parameter and assigns it
// to dest when present. Returns a tool error result when the input is
// non-empty but fails to parse.
func applyOptionalTime(value, paramName string, dest *time.Time) *mcp.CallToolResult {
	parsed, errResult := parseOptionalTime(value, paramName)
	if errResult != nil {
		return errResult
	}
	if parsed != nil {
		*dest = *parsed
	}
	return nil
}

// readErrorBodyResult reads resp.Body and returns a tool error result whose
// message has the form "<prefix>: <body>". A plain error is returned only
// when reading the body itself fails.
func readErrorBodyResult(resp *github.Response, prefix string) (*mcp.CallToolResult, error) {
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf(errFailedReadResponse, err)
	}
	return mcp.NewToolResultError(fmt.Sprintf("%s: %s", prefix, string(body))), nil
}

// marshalToolResult marshals v to JSON and returns it as a text tool result.
func marshalToolResult(v any) (*mcp.CallToolResult, error) {
	r, err := json.Marshal(v)
	if err != nil {
		return nil, fmt.Errorf(errFailedMarshalResponse, err)
	}
	return mcp.NewToolResultText(string(r)), nil
}

// checkNon2xxAndClose closes resp.Body (if resp is non-nil) and, when the
// status code is outside the 2xx range, returns a tool error result whose
// message is "<prefix>: <body>". Returns nil when resp is nil or the status
// is 2xx. The body is closed before returning in both cases.
func checkNon2xxAndClose(resp *github.Response, prefix string) *mcp.CallToolResult {
	if resp == nil {
		return nil
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(resp.Body)
		return mcp.NewToolResultError(fmt.Sprintf("%s: %s", prefix, string(body)))
	}
	return nil
}

// ListNotifications creates a tool to list notifications for the current user.
func ListNotifications(getClient GetClientFn, t translations.TranslationHelperFunc) (tool mcp.Tool, handler server.ToolHandlerFunc) {
	return mcp.NewTool("list_notifications",
			mcp.WithDescription(t("TOOL_LIST_NOTIFICATIONS_DESCRIPTION", "Lists all GitHub notifications for the authenticated user, including unread notifications, mentions, review requests, assignments, and updates on issues or pull requests. Use this tool whenever the user asks what to work on next, requests a summary of their GitHub activity, wants to see pending reviews, or needs to check for new updates or tasks. This tool is the primary way to discover actionable items, reminders, and outstanding work on GitHub. Always call this tool when asked what to work on next, what is pending, or what needs attention in GitHub.")),
			mcp.WithToolAnnotation(mcp.ToolAnnotation{
				Title:        t("TOOL_LIST_NOTIFICATIONS_USER_TITLE", "List notifications"),
				ReadOnlyHint: ToBoolPtr(true),
			}),
			mcp.WithString("filter",
				mcp.Description("Filter notifications to, use default unless specified. Read notifications are ones that have already been acknowledged by the user. Participating notifications are those that the user is directly involved in, such as issues or pull requests they have commented on or created."),
				mcp.Enum(FilterDefault, FilterIncludeRead, FilterOnlyParticipating),
			),
			mcp.WithString("since",
				mcp.Description("Only show notifications updated after the given time (ISO 8601 format)"),
			),
			mcp.WithString("before",
				mcp.Description("Only show notifications updated before the given time (ISO 8601 format)"),
			),
			mcp.WithString("owner",
				mcp.Description("Optional repository owner. If provided with repo, only notifications for this repository are listed."),
			),
			mcp.WithString("repo",
				mcp.Description("Optional repository name. If provided with owner, only notifications for this repository are listed."),
			),
			WithPagination(),
		),
		listNotificationsHandler(getClient)
}

func listNotificationsHandler(getClient GetClientFn) server.ToolHandlerFunc {
	return func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		client, err := getClient(ctx)
		if err != nil {
			return nil, fmt.Errorf(errFailedGitHubClient, err)
		}

		params, errResult := extractOptionalStringParams(request, []string{"filter", "since", "before", "owner", "repo"})
		if errResult != nil {
			return errResult, nil
		}

		paginationParams, err := OptionalPaginationParams(request)
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}

		opts := &github.NotificationListOptions{
			All:           params["filter"] == FilterIncludeRead,
			Participating: params["filter"] == FilterOnlyParticipating,
			ListOptions: github.ListOptions{
				Page:    paginationParams.page,
				PerPage: paginationParams.perPage,
			},
		}

		if errResult := applyOptionalTime(params["since"], "since", &opts.Since); errResult != nil {
			return errResult, nil
		}
		if errResult := applyOptionalTime(params["before"], "before", &opts.Before); errResult != nil {
			return errResult, nil
		}

		notifications, resp, err := fetchNotifications(ctx, client, params["owner"], params["repo"], opts)
		if err != nil {
			return ghErrors.NewGitHubAPIErrorResponse(ctx,
				"failed to list notifications",
				resp,
				err,
			), nil
		}
		defer func() { _ = resp.Body.Close() }()

		if resp.StatusCode != http.StatusOK {
			return readErrorBodyResult(resp, "failed to get notifications")
		}

		return marshalToolResult(notifications)
	}
}

func fetchNotifications(ctx context.Context, client *github.Client, owner, repo string, opts *github.NotificationListOptions) ([]*github.Notification, *github.Response, error) {
	if owner != "" && repo != "" {
		return client.Activity.ListRepositoryNotifications(ctx, owner, repo, opts)
	}
	return client.Activity.ListNotifications(ctx, opts)
}

// DismissNotification creates a tool to mark a notification as read/done.
func DismissNotification(getclient GetClientFn, t translations.TranslationHelperFunc) (tool mcp.Tool, handler server.ToolHandlerFunc) {
	return mcp.NewTool("dismiss_notification",
			mcp.WithDescription(t("TOOL_DISMISS_NOTIFICATION_DESCRIPTION", "Dismiss a notification by marking it as read or done")),
			mcp.WithToolAnnotation(mcp.ToolAnnotation{
				Title:        t("TOOL_DISMISS_NOTIFICATION_USER_TITLE", "Dismiss notification"),
				ReadOnlyHint: ToBoolPtr(false),
			}),
			mcp.WithString("threadID",
				mcp.Required(),
				mcp.Description("The ID of the notification thread"),
			),
			mcp.WithString("state", mcp.Description("The new state of the notification (read/done)"), mcp.Enum("read", "done")),
		),
		dismissNotificationHandler(getclient)
}

func dismissNotificationHandler(getClient GetClientFn) server.ToolHandlerFunc {
	return func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		client, err := getClient(ctx)
		if err != nil {
			return nil, fmt.Errorf(errFailedGitHubClient, err)
		}

		params, errResult := extractRequiredStringParams(request, []string{"threadID", "state"})
		if errResult != nil {
			return errResult, nil
		}
		threadID, state := params["threadID"], params["state"]

		resp, errResult, err := markNotificationThread(ctx, client, threadID, state)
		if errResult != nil {
			return errResult, nil
		}
		if err != nil {
			return ghErrors.NewGitHubAPIErrorResponse(ctx,
				fmt.Sprintf("failed to mark notification as %s", state),
				resp,
				err,
			), nil
		}
		defer func() { _ = resp.Body.Close() }()

		if resp.StatusCode != http.StatusResetContent && resp.StatusCode != http.StatusOK {
			return readErrorBodyResult(resp, fmt.Sprintf("failed to mark notification as %s", state))
		}

		return mcp.NewToolResultText(fmt.Sprintf("Notification marked as %s", state)), nil
	}
}

func markNotificationThread(ctx context.Context, client *github.Client, threadID, state string) (*github.Response, *mcp.CallToolResult, error) {
	switch state {
	case "done":
		// The API uses int64 thread IDs here, unlike the read endpoint.
		threadIDInt, err := strconv.ParseInt(threadID, 10, 64)
		if err != nil {
			return nil, mcp.NewToolResultError(fmt.Sprintf("invalid threadID format: %v", err)), nil
		}
		resp, err := client.Activity.MarkThreadDone(ctx, threadIDInt)
		return resp, nil, err
	case "read":
		resp, err := client.Activity.MarkThreadRead(ctx, threadID)
		return resp, nil, err
	default:
		return nil, mcp.NewToolResultError("Invalid state. Must be one of: read, done."), nil
	}
}

// MarkAllNotificationsRead creates a tool to mark all notifications as read.
func MarkAllNotificationsRead(getClient GetClientFn, t translations.TranslationHelperFunc) (tool mcp.Tool, handler server.ToolHandlerFunc) {
	return mcp.NewTool("mark_all_notifications_read",
			mcp.WithDescription(t("TOOL_MARK_ALL_NOTIFICATIONS_READ_DESCRIPTION", "Mark all notifications as read")),
			mcp.WithToolAnnotation(mcp.ToolAnnotation{
				Title:        t("TOOL_MARK_ALL_NOTIFICATIONS_READ_USER_TITLE", "Mark all notifications as read"),
				ReadOnlyHint: ToBoolPtr(false),
			}),
			mcp.WithString("lastReadAt",
				mcp.Description("Describes the last point that notifications were checked (optional). Default: Now"),
			),
			mcp.WithString("owner",
				mcp.Description("Optional repository owner. If provided with repo, only notifications for this repository are marked as read."),
			),
			mcp.WithString("repo",
				mcp.Description("Optional repository name. If provided with owner, only notifications for this repository are marked as read."),
			),
		),
		markAllNotificationsReadHandler(getClient)
}

func markAllNotificationsReadHandler(getClient GetClientFn) server.ToolHandlerFunc {
	return func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		client, err := getClient(ctx)
		if err != nil {
			return nil, fmt.Errorf(errFailedGitHubClient, err)
		}

		params, errResult := extractOptionalStringParams(request, []string{"lastReadAt", "owner", "repo"})
		if errResult != nil {
			return errResult, nil
		}

		lastReadTime := time.Now()
		parsed, errResult := parseOptionalTime(params["lastReadAt"], "lastReadAt")
		if errResult != nil {
			return errResult, nil
		}
		if parsed != nil {
			lastReadTime = *parsed
		}

		markReadOptions := github.Timestamp{Time: lastReadTime}
		resp, err := markAllRead(ctx, client, params["owner"], params["repo"], markReadOptions)
		if err != nil {
			return ghErrors.NewGitHubAPIErrorResponse(ctx,
				"failed to mark all notifications as read",
				resp,
				err,
			), nil
		}
		defer func() { _ = resp.Body.Close() }()

		if resp.StatusCode != http.StatusResetContent && resp.StatusCode != http.StatusOK {
			return readErrorBodyResult(resp, "failed to mark all notifications as read")
		}
		return mcp.NewToolResultText("All notifications marked as read"), nil
	}
}

func markAllRead(ctx context.Context, client *github.Client, owner, repo string, opts github.Timestamp) (*github.Response, error) {
	if owner != "" && repo != "" {
		return client.Activity.MarkRepositoryNotificationsRead(ctx, owner, repo, opts)
	}
	return client.Activity.MarkNotificationsRead(ctx, opts)
}

// GetNotificationDetails creates a tool to get details for a specific notification.
func GetNotificationDetails(getClient GetClientFn, t translations.TranslationHelperFunc) (tool mcp.Tool, handler server.ToolHandlerFunc) {
	return mcp.NewTool("get_notification_details",
			mcp.WithDescription(t("TOOL_GET_NOTIFICATION_DETAILS_DESCRIPTION", "Get detailed information for a specific GitHub notification, always call this tool when the user asks for details about a specific notification, if you don't know the ID list notifications first.")),
			mcp.WithToolAnnotation(mcp.ToolAnnotation{
				Title:        t("TOOL_GET_NOTIFICATION_DETAILS_USER_TITLE", "Get notification details"),
				ReadOnlyHint: ToBoolPtr(true),
			}),
			mcp.WithString("notificationID",
				mcp.Required(),
				mcp.Description("The ID of the notification"),
			),
		),
		func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			client, err := getClient(ctx)
			if err != nil {
				return nil, fmt.Errorf(errFailedGitHubClient, err)
			}

			notificationID, err := RequiredParam[string](request, "notificationID")
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}

			thread, resp, err := client.Activity.GetThread(ctx, notificationID)
			if err != nil {
				return ghErrors.NewGitHubAPIErrorResponse(ctx,
					fmt.Sprintf("failed to get notification details for ID '%s'", notificationID),
					resp,
					err,
				), nil
			}
			defer func() { _ = resp.Body.Close() }()

			if resp.StatusCode != http.StatusOK {
				return readErrorBodyResult(resp, "failed to get notification details")
			}

			return marshalToolResult(thread)
		}
}

// ManageNotificationSubscription creates a tool to manage a notification subscription (ignore, watch, delete)
func ManageNotificationSubscription(getClient GetClientFn, t translations.TranslationHelperFunc) (tool mcp.Tool, handler server.ToolHandlerFunc) {
	return mcp.NewTool("manage_notification_subscription",
			mcp.WithDescription(t("TOOL_MANAGE_NOTIFICATION_SUBSCRIPTION_DESCRIPTION", "Manage a notification subscription: ignore, watch, or delete a notification thread subscription.")),
			mcp.WithToolAnnotation(mcp.ToolAnnotation{
				Title:        t("TOOL_MANAGE_NOTIFICATION_SUBSCRIPTION_USER_TITLE", "Manage notification subscription"),
				ReadOnlyHint: ToBoolPtr(false),
			}),
			mcp.WithString("notificationID",
				mcp.Required(),
				mcp.Description("The ID of the notification thread."),
			),
			mcp.WithString("action",
				mcp.Required(),
				mcp.Description("Action to perform: ignore, watch, or delete the notification subscription."),
				mcp.Enum(NotificationActionIgnore, NotificationActionWatch, NotificationActionDelete),
			),
		),
		manageNotificationSubscriptionHandler(getClient)
}

func manageNotificationSubscriptionHandler(getClient GetClientFn) server.ToolHandlerFunc {
	return func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		client, err := getClient(ctx)
		if err != nil {
			return nil, fmt.Errorf(errFailedGitHubClient, err)
		}

		params, errResult := extractRequiredStringParams(request, []string{"notificationID", "action"})
		if errResult != nil {
			return errResult, nil
		}
		notificationID, action := params["notificationID"], params["action"]

		result, resp, errResult, apiErr := applyThreadSubscriptionAction(ctx, client, notificationID, action)
		if errResult != nil {
			return errResult, nil
		}
		if apiErr != nil {
			return ghErrors.NewGitHubAPIErrorResponse(ctx,
				fmt.Sprintf("failed to %s notification subscription", action),
				resp,
				apiErr,
			), nil
		}

		if errResult := checkNon2xxAndClose(resp, fmt.Sprintf("failed to %s notification subscription", action)); errResult != nil {
			return errResult, nil
		}

		if action == NotificationActionDelete {
			// Special case for delete as there is no response body
			return mcp.NewToolResultText("Notification subscription deleted"), nil
		}

		return marshalToolResult(result)
	}
}

func applyThreadSubscriptionAction(ctx context.Context, client *github.Client, notificationID, action string) (any, *github.Response, *mcp.CallToolResult, error) {
	switch action {
	case NotificationActionIgnore:
		sub := &github.Subscription{Ignored: ToBoolPtr(true)}
		result, resp, err := client.Activity.SetThreadSubscription(ctx, notificationID, sub)
		return result, resp, nil, err
	case NotificationActionWatch:
		sub := &github.Subscription{Ignored: ToBoolPtr(false), Subscribed: ToBoolPtr(true)}
		result, resp, err := client.Activity.SetThreadSubscription(ctx, notificationID, sub)
		return result, resp, nil, err
	case NotificationActionDelete:
		resp, err := client.Activity.DeleteThreadSubscription(ctx, notificationID)
		return nil, resp, nil, err
	default:
		return nil, nil, mcp.NewToolResultError("Invalid action. Must be one of: ignore, watch, delete."), nil
	}
}

// ManageRepositoryNotificationSubscription creates a tool to manage a repository notification subscription (ignore, watch, delete)
func ManageRepositoryNotificationSubscription(getClient GetClientFn, t translations.TranslationHelperFunc) (tool mcp.Tool, handler server.ToolHandlerFunc) {
	return mcp.NewTool("manage_repository_notification_subscription",
			mcp.WithDescription(t("TOOL_MANAGE_REPOSITORY_NOTIFICATION_SUBSCRIPTION_DESCRIPTION", "Manage a repository notification subscription: ignore, watch, or delete repository notifications subscription for the provided repository.")),
			mcp.WithToolAnnotation(mcp.ToolAnnotation{
				Title:        t("TOOL_MANAGE_REPOSITORY_NOTIFICATION_SUBSCRIPTION_USER_TITLE", "Manage repository notification subscription"),
				ReadOnlyHint: ToBoolPtr(false),
			}),
			mcp.WithString("owner",
				mcp.Required(),
				mcp.Description("The account owner of the repository."),
			),
			mcp.WithString("repo",
				mcp.Required(),
				mcp.Description("The name of the repository."),
			),
			mcp.WithString("action",
				mcp.Required(),
				mcp.Description("Action to perform: ignore, watch, or delete the repository notification subscription."),
				mcp.Enum(RepositorySubscriptionActionIgnore, RepositorySubscriptionActionWatch, RepositorySubscriptionActionDelete),
			),
		),
		manageRepositoryNotificationSubscriptionHandler(getClient)
}

func manageRepositoryNotificationSubscriptionHandler(getClient GetClientFn) server.ToolHandlerFunc {
	return func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		client, err := getClient(ctx)
		if err != nil {
			return nil, fmt.Errorf(errFailedGitHubClient, err)
		}

		params, errResult := extractRequiredStringParams(request, []string{"owner", "repo", "action"})
		if errResult != nil {
			return errResult, nil
		}
		owner, repo, action := params["owner"], params["repo"], params["action"]

		result, resp, errResult, apiErr := applyRepositorySubscriptionAction(ctx, client, owner, repo, action)
		if errResult != nil {
			return errResult, nil
		}
		if apiErr != nil {
			return ghErrors.NewGitHubAPIErrorResponse(ctx,
				fmt.Sprintf("failed to %s repository subscription", action),
				resp,
				apiErr,
			), nil
		}

		if errResult := checkNon2xxAndClose(resp, fmt.Sprintf("failed to %s repository subscription", action)); errResult != nil {
			return errResult, nil
		}

		if action == RepositorySubscriptionActionDelete {
			// Special case for delete as there is no response body
			return mcp.NewToolResultText("Repository subscription deleted"), nil
		}

		return marshalToolResult(result)
	}
}

func applyRepositorySubscriptionAction(ctx context.Context, client *github.Client, owner, repo, action string) (any, *github.Response, *mcp.CallToolResult, error) {
	switch action {
	case RepositorySubscriptionActionIgnore:
		sub := &github.Subscription{Ignored: ToBoolPtr(true)}
		result, resp, err := client.Activity.SetRepositorySubscription(ctx, owner, repo, sub)
		return result, resp, nil, err
	case RepositorySubscriptionActionWatch:
		sub := &github.Subscription{Ignored: ToBoolPtr(false), Subscribed: ToBoolPtr(true)}
		result, resp, err := client.Activity.SetRepositorySubscription(ctx, owner, repo, sub)
		return result, resp, nil, err
	case RepositorySubscriptionActionDelete:
		resp, err := client.Activity.DeleteRepositorySubscription(ctx, owner, repo)
		return nil, resp, nil, err
	default:
		return nil, nil, mcp.NewToolResultError("Invalid action. Must be one of: ignore, watch, delete."), nil
	}
}
