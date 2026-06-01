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

// Error message formats reused across the notification tool handlers. Defining
// them as constants avoids duplicated string literals (SonarQube S1192).
const (
	errFailedToGetClient = "failed to get GitHub client: %w"
	errFailedToReadBody  = "failed to read response body: %w"
	errFailedToMarshal   = "failed to marshal response: %w"
)

// Tool parameter names reused across the notification tools.
const (
	paramOwner          = "owner"
	paramRepo           = "repo"
	paramAction         = "action"
	paramNotificationID = "notificationID"
	paramSince          = "since"
	paramBefore         = "before"
	paramLastReadAt     = "lastReadAt"
)

// marshalToolResult marshals v as JSON and returns it as a tool text result.
func marshalToolResult(v any) (*mcp.CallToolResult, error) {
	r, err := json.Marshal(v)
	if err != nil {
		return nil, fmt.Errorf(errFailedToMarshal, err)
	}
	return mcp.NewToolResultText(string(r)), nil
}

// responseErrorResult reads the body of an unexpected response and returns a tool
// error result prefixed with message.
func responseErrorResult(resp *github.Response, message string) (*mcp.CallToolResult, error) {
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf(errFailedToReadBody, err)
	}
	return mcp.NewToolResultError(fmt.Sprintf("%s: %s", message, string(body))), nil
}

// parseOptionalTimestamp parses an optional RFC3339 timestamp. An empty value
// yields the zero time and a nil result. An invalid value yields a tool error
// result describing the offending field.
func parseOptionalTimestamp(value, field string) (time.Time, *mcp.CallToolResult) {
	if value == "" {
		return time.Time{}, nil
	}
	parsed, err := time.Parse(time.RFC3339, value)
	if err != nil {
		return time.Time{}, mcp.NewToolResultError(fmt.Sprintf("invalid %s time format, should be RFC3339/ISO8601: %v", field, err))
	}
	return parsed, nil
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
			mcp.WithString(paramSince,
				mcp.Description("Only show notifications updated after the given time (ISO 8601 format)"),
			),
			mcp.WithString(paramBefore,
				mcp.Description("Only show notifications updated before the given time (ISO 8601 format)"),
			),
			mcp.WithString(paramOwner,
				mcp.Description("Optional repository owner. If provided with repo, only notifications for this repository are listed."),
			),
			mcp.WithString(paramRepo,
				mcp.Description("Optional repository name. If provided with owner, only notifications for this repository are listed."),
			),
			WithPagination(),
		),
		func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			return listNotificationsHandler(ctx, getClient, request)
		}
}

// listNotificationsParams holds the parsed parameters for the list_notifications tool.
type listNotificationsParams struct {
	filter     string
	since      string
	before     string
	owner      string
	repo       string
	pagination PaginationParams
}

// parseListNotificationsParams extracts the list_notifications parameters from the
// request. A non-nil result indicates a parameter error that should be returned.
func parseListNotificationsParams(request mcp.CallToolRequest) (listNotificationsParams, *mcp.CallToolResult) {
	var p listNotificationsParams
	var err error

	if p.filter, err = OptionalParam[string](request, "filter"); err != nil {
		return p, mcp.NewToolResultError(err.Error())
	}
	if p.since, err = OptionalParam[string](request, paramSince); err != nil {
		return p, mcp.NewToolResultError(err.Error())
	}
	if p.before, err = OptionalParam[string](request, paramBefore); err != nil {
		return p, mcp.NewToolResultError(err.Error())
	}
	if p.owner, err = OptionalParam[string](request, paramOwner); err != nil {
		return p, mcp.NewToolResultError(err.Error())
	}
	if p.repo, err = OptionalParam[string](request, paramRepo); err != nil {
		return p, mcp.NewToolResultError(err.Error())
	}
	if p.pagination, err = OptionalPaginationParams(request); err != nil {
		return p, mcp.NewToolResultError(err.Error())
	}
	return p, nil
}

func listNotificationsHandler(ctx context.Context, getClient GetClientFn, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	client, err := getClient(ctx)
	if err != nil {
		return nil, fmt.Errorf(errFailedToGetClient, err)
	}

	params, errResult := parseListNotificationsParams(request)
	if errResult != nil {
		return errResult, nil
	}

	opts := &github.NotificationListOptions{
		All:           params.filter == FilterIncludeRead,
		Participating: params.filter == FilterOnlyParticipating,
		ListOptions: github.ListOptions{
			Page:    params.pagination.page,
			PerPage: params.pagination.perPage,
		},
	}

	sinceTime, errResult := parseOptionalTimestamp(params.since, paramSince)
	if errResult != nil {
		return errResult, nil
	}
	if !sinceTime.IsZero() {
		opts.Since = sinceTime
	}

	beforeTime, errResult := parseOptionalTimestamp(params.before, paramBefore)
	if errResult != nil {
		return errResult, nil
	}
	if !beforeTime.IsZero() {
		opts.Before = beforeTime
	}

	var notifications []*github.Notification
	var resp *github.Response
	if params.owner != "" && params.repo != "" {
		notifications, resp, err = client.Activity.ListRepositoryNotifications(ctx, params.owner, params.repo, opts)
	} else {
		notifications, resp, err = client.Activity.ListNotifications(ctx, opts)
	}
	if err != nil {
		return ghErrors.NewGitHubAPIErrorResponse(ctx,
			"failed to list notifications",
			resp,
			err,
		), nil
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return responseErrorResult(resp, "failed to get notifications")
	}

	return marshalToolResult(notifications)
}

// DismissNotification creates a tool to mark a notification as read/done.
func DismissNotification(getClient GetClientFn, t translations.TranslationHelperFunc) (tool mcp.Tool, handler server.ToolHandlerFunc) {
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
		func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			return dismissNotificationHandler(ctx, getClient, request)
		}
}

func dismissNotificationHandler(ctx context.Context, getClient GetClientFn, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	client, err := getClient(ctx)
	if err != nil {
		return nil, fmt.Errorf(errFailedToGetClient, err)
	}

	threadID, err := RequiredParam[string](request, "threadID")
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}

	state, err := RequiredParam[string](request, "state")
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}

	var resp *github.Response
	switch state {
	case "done":
		// for some inexplicable reason, the API seems to have threadID as int64 and string depending on the endpoint
		var threadIDInt int64
		threadIDInt, err = strconv.ParseInt(threadID, 10, 64)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("invalid threadID format: %v", err)), nil
		}
		resp, err = client.Activity.MarkThreadDone(ctx, threadIDInt)
	case "read":
		resp, err = client.Activity.MarkThreadRead(ctx, threadID)
	default:
		return mcp.NewToolResultError("Invalid state. Must be one of: read, done."), nil
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
		return responseErrorResult(resp, fmt.Sprintf("failed to mark notification as %s", state))
	}

	return mcp.NewToolResultText(fmt.Sprintf("Notification marked as %s", state)), nil
}

// MarkAllNotificationsRead creates a tool to mark all notifications as read.
func MarkAllNotificationsRead(getClient GetClientFn, t translations.TranslationHelperFunc) (tool mcp.Tool, handler server.ToolHandlerFunc) {
	return mcp.NewTool("mark_all_notifications_read",
			mcp.WithDescription(t("TOOL_MARK_ALL_NOTIFICATIONS_READ_DESCRIPTION", "Mark all notifications as read")),
			mcp.WithToolAnnotation(mcp.ToolAnnotation{
				Title:        t("TOOL_MARK_ALL_NOTIFICATIONS_READ_USER_TITLE", "Mark all notifications as read"),
				ReadOnlyHint: ToBoolPtr(false),
			}),
			mcp.WithString(paramLastReadAt,
				mcp.Description("Describes the last point that notifications were checked (optional). Default: Now"),
			),
			mcp.WithString(paramOwner,
				mcp.Description("Optional repository owner. If provided with repo, only notifications for this repository are marked as read."),
			),
			mcp.WithString(paramRepo,
				mcp.Description("Optional repository name. If provided with owner, only notifications for this repository are marked as read."),
			),
		),
		func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			return markAllNotificationsReadHandler(ctx, getClient, request)
		}
}

func markAllNotificationsReadHandler(ctx context.Context, getClient GetClientFn, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	client, err := getClient(ctx)
	if err != nil {
		return nil, fmt.Errorf(errFailedToGetClient, err)
	}

	lastReadAt, err := OptionalParam[string](request, paramLastReadAt)
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}

	owner, err := OptionalParam[string](request, paramOwner)
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	repo, err := OptionalParam[string](request, paramRepo)
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}

	lastReadTime, errResult := parseOptionalTimestamp(lastReadAt, paramLastReadAt)
	if errResult != nil {
		return errResult, nil
	}
	if lastReadTime.IsZero() {
		lastReadTime = time.Now()
	}

	markReadOptions := github.Timestamp{
		Time: lastReadTime,
	}

	var resp *github.Response
	if owner != "" && repo != "" {
		resp, err = client.Activity.MarkRepositoryNotificationsRead(ctx, owner, repo, markReadOptions)
	} else {
		resp, err = client.Activity.MarkNotificationsRead(ctx, markReadOptions)
	}
	if err != nil {
		return ghErrors.NewGitHubAPIErrorResponse(ctx,
			"failed to mark all notifications as read",
			resp,
			err,
		), nil
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusResetContent && resp.StatusCode != http.StatusOK {
		return responseErrorResult(resp, "failed to mark all notifications as read")
	}

	return mcp.NewToolResultText("All notifications marked as read"), nil
}

// GetNotificationDetails creates a tool to get details for a specific notification.
func GetNotificationDetails(getClient GetClientFn, t translations.TranslationHelperFunc) (tool mcp.Tool, handler server.ToolHandlerFunc) {
	return mcp.NewTool("get_notification_details",
			mcp.WithDescription(t("TOOL_GET_NOTIFICATION_DETAILS_DESCRIPTION", "Get detailed information for a specific GitHub notification, always call this tool when the user asks for details about a specific notification, if you don't know the ID list notifications first.")),
			mcp.WithToolAnnotation(mcp.ToolAnnotation{
				Title:        t("TOOL_GET_NOTIFICATION_DETAILS_USER_TITLE", "Get notification details"),
				ReadOnlyHint: ToBoolPtr(true),
			}),
			mcp.WithString(paramNotificationID,
				mcp.Required(),
				mcp.Description("The ID of the notification"),
			),
		),
		func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			return getNotificationDetailsHandler(ctx, getClient, request)
		}
}

func getNotificationDetailsHandler(ctx context.Context, getClient GetClientFn, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	client, err := getClient(ctx)
	if err != nil {
		return nil, fmt.Errorf(errFailedToGetClient, err)
	}

	notificationID, err := RequiredParam[string](request, paramNotificationID)
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
		return responseErrorResult(resp, "failed to get notification details")
	}

	return marshalToolResult(thread)
}

// Enum values for ManageNotificationSubscription action
const (
	NotificationActionIgnore = "ignore"
	NotificationActionWatch  = "watch"
	NotificationActionDelete = "delete"
)

// setThreadSubscription performs the requested subscription action on a notification
// thread. ok is false when action is not a recognized value.
func setThreadSubscription(ctx context.Context, client *github.Client, action, notificationID string) (result any, resp *github.Response, ok bool, err error) {
	switch action {
	case NotificationActionIgnore:
		sub := &github.Subscription{Ignored: ToBoolPtr(true)}
		result, resp, err = client.Activity.SetThreadSubscription(ctx, notificationID, sub)
	case NotificationActionWatch:
		sub := &github.Subscription{Ignored: ToBoolPtr(false), Subscribed: ToBoolPtr(true)}
		result, resp, err = client.Activity.SetThreadSubscription(ctx, notificationID, sub)
	case NotificationActionDelete:
		resp, err = client.Activity.DeleteThreadSubscription(ctx, notificationID)
	default:
		return nil, nil, false, nil
	}
	return result, resp, true, err
}

// ManageNotificationSubscription creates a tool to manage a notification subscription (ignore, watch, delete)
func ManageNotificationSubscription(getClient GetClientFn, t translations.TranslationHelperFunc) (tool mcp.Tool, handler server.ToolHandlerFunc) {
	return mcp.NewTool("manage_notification_subscription",
			mcp.WithDescription(t("TOOL_MANAGE_NOTIFICATION_SUBSCRIPTION_DESCRIPTION", "Manage a notification subscription: ignore, watch, or delete a notification thread subscription.")),
			mcp.WithToolAnnotation(mcp.ToolAnnotation{
				Title:        t("TOOL_MANAGE_NOTIFICATION_SUBSCRIPTION_USER_TITLE", "Manage notification subscription"),
				ReadOnlyHint: ToBoolPtr(false),
			}),
			mcp.WithString(paramNotificationID,
				mcp.Required(),
				mcp.Description("The ID of the notification thread."),
			),
			mcp.WithString(paramAction,
				mcp.Required(),
				mcp.Description("Action to perform: ignore, watch, or delete the notification subscription."),
				mcp.Enum(NotificationActionIgnore, NotificationActionWatch, NotificationActionDelete),
			),
		),
		func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			return manageNotificationSubscriptionHandler(ctx, getClient, request)
		}
}

func manageNotificationSubscriptionHandler(ctx context.Context, getClient GetClientFn, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	client, err := getClient(ctx)
	if err != nil {
		return nil, fmt.Errorf(errFailedToGetClient, err)
	}

	notificationID, err := RequiredParam[string](request, paramNotificationID)
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	action, err := RequiredParam[string](request, paramAction)
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}

	result, resp, ok, apiErr := setThreadSubscription(ctx, client, action, notificationID)
	if !ok {
		return mcp.NewToolResultError("Invalid action. Must be one of: ignore, watch, delete."), nil
	}

	if apiErr != nil {
		return ghErrors.NewGitHubAPIErrorResponse(ctx,
			fmt.Sprintf("failed to %s notification subscription", action),
			resp,
			apiErr,
		), nil
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(resp.Body)
		return mcp.NewToolResultError(fmt.Sprintf("failed to %s notification subscription: %s", action, string(body))), nil
	}

	if action == NotificationActionDelete {
		// Special case for delete as there is no response body
		return mcp.NewToolResultText("Notification subscription deleted"), nil
	}

	return marshalToolResult(result)
}

const (
	RepositorySubscriptionActionWatch  = "watch"
	RepositorySubscriptionActionIgnore = "ignore"
	RepositorySubscriptionActionDelete = "delete"
)

// setRepositorySubscription performs the requested subscription action on a
// repository. ok is false when action is not a recognized value.
func setRepositorySubscription(ctx context.Context, client *github.Client, action, owner, repo string) (result any, resp *github.Response, ok bool, err error) {
	switch action {
	case RepositorySubscriptionActionIgnore:
		sub := &github.Subscription{Ignored: ToBoolPtr(true)}
		result, resp, err = client.Activity.SetRepositorySubscription(ctx, owner, repo, sub)
	case RepositorySubscriptionActionWatch:
		sub := &github.Subscription{Ignored: ToBoolPtr(false), Subscribed: ToBoolPtr(true)}
		result, resp, err = client.Activity.SetRepositorySubscription(ctx, owner, repo, sub)
	case RepositorySubscriptionActionDelete:
		resp, err = client.Activity.DeleteRepositorySubscription(ctx, owner, repo)
	default:
		return nil, nil, false, nil
	}
	return result, resp, true, err
}

// ManageRepositoryNotificationSubscription creates a tool to manage a repository notification subscription (ignore, watch, delete)
func ManageRepositoryNotificationSubscription(getClient GetClientFn, t translations.TranslationHelperFunc) (tool mcp.Tool, handler server.ToolHandlerFunc) {
	return mcp.NewTool("manage_repository_notification_subscription",
			mcp.WithDescription(t("TOOL_MANAGE_REPOSITORY_NOTIFICATION_SUBSCRIPTION_DESCRIPTION", "Manage a repository notification subscription: ignore, watch, or delete repository notifications subscription for the provided repository.")),
			mcp.WithToolAnnotation(mcp.ToolAnnotation{
				Title:        t("TOOL_MANAGE_REPOSITORY_NOTIFICATION_SUBSCRIPTION_USER_TITLE", "Manage repository notification subscription"),
				ReadOnlyHint: ToBoolPtr(false),
			}),
			mcp.WithString(paramOwner,
				mcp.Required(),
				mcp.Description("The account owner of the repository."),
			),
			mcp.WithString(paramRepo,
				mcp.Required(),
				mcp.Description("The name of the repository."),
			),
			mcp.WithString(paramAction,
				mcp.Required(),
				mcp.Description("Action to perform: ignore, watch, or delete the repository notification subscription."),
				mcp.Enum(RepositorySubscriptionActionIgnore, RepositorySubscriptionActionWatch, RepositorySubscriptionActionDelete),
			),
		),
		func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			return manageRepositoryNotificationSubscriptionHandler(ctx, getClient, request)
		}
}

func manageRepositoryNotificationSubscriptionHandler(ctx context.Context, getClient GetClientFn, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	client, err := getClient(ctx)
	if err != nil {
		return nil, fmt.Errorf(errFailedToGetClient, err)
	}

	owner, err := RequiredParam[string](request, paramOwner)
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	repo, err := RequiredParam[string](request, paramRepo)
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	action, err := RequiredParam[string](request, paramAction)
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}

	result, resp, ok, apiErr := setRepositorySubscription(ctx, client, action, owner, repo)
	if !ok {
		return mcp.NewToolResultError("Invalid action. Must be one of: ignore, watch, delete."), nil
	}

	if apiErr != nil {
		return ghErrors.NewGitHubAPIErrorResponse(ctx,
			fmt.Sprintf("failed to %s repository subscription", action),
			resp,
			apiErr,
		), nil
	}
	if resp != nil {
		defer func() { _ = resp.Body.Close() }()
	}

	// Handle non-2xx status codes
	if resp != nil && (resp.StatusCode < 200 || resp.StatusCode >= 300) {
		body, _ := io.ReadAll(resp.Body)
		return mcp.NewToolResultError(fmt.Sprintf("failed to %s repository subscription: %s", action, string(body))), nil
	}

	if action == RepositorySubscriptionActionDelete {
		// Special case for delete as there is no response body
		return mcp.NewToolResultText("Repository subscription deleted"), nil
	}

	return marshalToolResult(result)
}
