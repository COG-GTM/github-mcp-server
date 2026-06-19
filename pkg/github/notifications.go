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

// Parameter names shared across the notification tools.
const (
	ownerParam          = "owner"
	actionParam         = "action"
	notificationIDParam = "notificationID"
)

// Error message formats shared across the notification tools.
const (
	errFailedToGetClient       = "failed to get GitHub client: %w"
	errFailedToReadBody        = "failed to read response body: %w"
	errFailedToMarshalResponse = "failed to marshal response: %w"
)

// notificationResponseError reads the body of a failed notifications API
// response and returns a tool result error describing the failure.
func notificationResponseError(resp *github.Response, message string) (*mcp.CallToolResult, error) {
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf(errFailedToReadBody, err)
	}
	return mcp.NewToolResultError(fmt.Sprintf("%s: %s", message, string(body))), nil
}

// buildNotificationListOptions assembles the notification list options from the
// provided filter, optional time bounds, and pagination parameters.
func buildNotificationListOptions(filter, since, before string, pagination PaginationParams) (*github.NotificationListOptions, error) {
	opts := &github.NotificationListOptions{
		All:           filter == FilterIncludeRead,
		Participating: filter == FilterOnlyParticipating,
		ListOptions: github.ListOptions{
			Page:    pagination.page,
			PerPage: pagination.perPage,
		},
	}

	if since != "" {
		sinceTime, err := time.Parse(time.RFC3339, since)
		if err != nil {
			return nil, fmt.Errorf("invalid since time format, should be RFC3339/ISO8601: %w", err)
		}
		opts.Since = sinceTime
	}

	if before != "" {
		beforeTime, err := time.Parse(time.RFC3339, before)
		if err != nil {
			return nil, fmt.Errorf("invalid before time format, should be RFC3339/ISO8601: %w", err)
		}
		opts.Before = beforeTime
	}

	return opts, nil
}

// listNotificationsByScope lists notifications scoped to a single repository
// when both owner and repo are provided, otherwise across all repositories.
func listNotificationsByScope(ctx context.Context, client *github.Client, owner, repo string, opts *github.NotificationListOptions) ([]*github.Notification, *github.Response, error) {
	if owner != "" && repo != "" {
		return client.Activity.ListRepositoryNotifications(ctx, owner, repo, opts)
	}
	return client.Activity.ListNotifications(ctx, opts)
}

// parseLastReadAt parses the optional lastReadAt timestamp, defaulting to the
// current time when no value is provided.
func parseLastReadAt(lastReadAt string) (time.Time, error) {
	if lastReadAt == "" {
		return time.Now(), nil
	}
	return time.Parse(time.RFC3339, lastReadAt)
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
			mcp.WithString(ownerParam,
				mcp.Description("Optional repository owner. If provided with repo, only notifications for this repository are listed."),
			),
			mcp.WithString("repo",
				mcp.Description("Optional repository name. If provided with owner, only notifications for this repository are listed."),
			),
			WithPagination(),
		),
		func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			client, err := getClient(ctx)
			if err != nil {
				return nil, fmt.Errorf(errFailedToGetClient, err)
			}

			filter, err := OptionalParam[string](request, "filter")
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}

			since, err := OptionalParam[string](request, "since")
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}

			before, err := OptionalParam[string](request, "before")
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}

			owner, err := OptionalParam[string](request, ownerParam)
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			repo, err := OptionalParam[string](request, "repo")
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}

			paginationParams, err := OptionalPaginationParams(request)
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}

			opts, err := buildNotificationListOptions(filter, since, before, paginationParams)
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}

			notifications, resp, err := listNotificationsByScope(ctx, client, owner, repo, opts)
			if err != nil {
				return ghErrors.NewGitHubAPIErrorResponse(ctx,
					"failed to list notifications",
					resp,
					err,
				), nil
			}
			defer func() { _ = resp.Body.Close() }()

			if resp.StatusCode != http.StatusOK {
				return notificationResponseError(resp, "failed to get notifications")
			}

			// Marshal response to JSON
			r, err := json.Marshal(notifications)
			if err != nil {
				return nil, fmt.Errorf(errFailedToMarshalResponse, err)
			}

			return mcp.NewToolResultText(string(r)), nil
		}
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
		func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			client, err := getclient(ctx)
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
				return notificationResponseError(resp, fmt.Sprintf("failed to mark notification as %s", state))
			}

			return mcp.NewToolResultText(fmt.Sprintf("Notification marked as %s", state)), nil
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
			mcp.WithString(ownerParam,
				mcp.Description("Optional repository owner. If provided with repo, only notifications for this repository are marked as read."),
			),
			mcp.WithString("repo",
				mcp.Description("Optional repository name. If provided with owner, only notifications for this repository are marked as read."),
			),
		),
		func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			client, err := getClient(ctx)
			if err != nil {
				return nil, fmt.Errorf(errFailedToGetClient, err)
			}

			lastReadAt, err := OptionalParam[string](request, "lastReadAt")
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}

			owner, err := OptionalParam[string](request, ownerParam)
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			repo, err := OptionalParam[string](request, "repo")
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}

			lastReadTime, err := parseLastReadAt(lastReadAt)
			if err != nil {
				return mcp.NewToolResultError(fmt.Sprintf("invalid lastReadAt time format, should be RFC3339/ISO8601: %v", err)), nil
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
				return notificationResponseError(resp, "failed to mark all notifications as read")
			}

			return mcp.NewToolResultText("All notifications marked as read"), nil
		}
}

// GetNotificationDetails creates a tool to get details for a specific notification.
func GetNotificationDetails(getClient GetClientFn, t translations.TranslationHelperFunc) (tool mcp.Tool, handler server.ToolHandlerFunc) {
	return mcp.NewTool("get_notification_details",
			mcp.WithDescription(t("TOOL_GET_NOTIFICATION_DETAILS_DESCRIPTION", "Get detailed information for a specific GitHub notification, always call this tool when the user asks for details about a specific notification, if you don't know the ID list notifications first.")),
			mcp.WithToolAnnotation(mcp.ToolAnnotation{
				Title:        t("TOOL_GET_NOTIFICATION_DETAILS_USER_TITLE", "Get notification details"),
				ReadOnlyHint: ToBoolPtr(true),
			}),
			mcp.WithString(notificationIDParam,
				mcp.Required(),
				mcp.Description("The ID of the notification"),
			),
		),
		func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			client, err := getClient(ctx)
			if err != nil {
				return nil, fmt.Errorf(errFailedToGetClient, err)
			}

			notificationID, err := RequiredParam[string](request, notificationIDParam)
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
				return notificationResponseError(resp, "failed to get notification details")
			}

			r, err := json.Marshal(thread)
			if err != nil {
				return nil, fmt.Errorf(errFailedToMarshalResponse, err)
			}

			return mcp.NewToolResultText(string(r)), nil
		}
}

// Enum values for ManageNotificationSubscription action
const (
	NotificationActionIgnore = "ignore"
	NotificationActionWatch  = "watch"
	NotificationActionDelete = "delete"
)

// ManageNotificationSubscription creates a tool to manage a notification subscription (ignore, watch, delete)
func ManageNotificationSubscription(getClient GetClientFn, t translations.TranslationHelperFunc) (tool mcp.Tool, handler server.ToolHandlerFunc) {
	return mcp.NewTool("manage_notification_subscription",
			mcp.WithDescription(t("TOOL_MANAGE_NOTIFICATION_SUBSCRIPTION_DESCRIPTION", "Manage a notification subscription: ignore, watch, or delete a notification thread subscription.")),
			mcp.WithToolAnnotation(mcp.ToolAnnotation{
				Title:        t("TOOL_MANAGE_NOTIFICATION_SUBSCRIPTION_USER_TITLE", "Manage notification subscription"),
				ReadOnlyHint: ToBoolPtr(false),
			}),
			mcp.WithString(notificationIDParam,
				mcp.Required(),
				mcp.Description("The ID of the notification thread."),
			),
			mcp.WithString(actionParam,
				mcp.Required(),
				mcp.Description("Action to perform: ignore, watch, or delete the notification subscription."),
				mcp.Enum(NotificationActionIgnore, NotificationActionWatch, NotificationActionDelete),
			),
		),
		func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			client, err := getClient(ctx)
			if err != nil {
				return nil, fmt.Errorf(errFailedToGetClient, err)
			}

			notificationID, err := RequiredParam[string](request, notificationIDParam)
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			action, err := RequiredParam[string](request, actionParam)
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}

			var (
				resp   *github.Response
				result any
				apiErr error
			)

			switch action {
			case NotificationActionIgnore:
				sub := &github.Subscription{Ignored: ToBoolPtr(true)}
				result, resp, apiErr = client.Activity.SetThreadSubscription(ctx, notificationID, sub)
			case NotificationActionWatch:
				sub := &github.Subscription{Ignored: ToBoolPtr(false), Subscribed: ToBoolPtr(true)}
				result, resp, apiErr = client.Activity.SetThreadSubscription(ctx, notificationID, sub)
			case NotificationActionDelete:
				resp, apiErr = client.Activity.DeleteThreadSubscription(ctx, notificationID)
			default:
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

			r, err := json.Marshal(result)
			if err != nil {
				return nil, fmt.Errorf(errFailedToMarshalResponse, err)
			}
			return mcp.NewToolResultText(string(r)), nil
		}
}

const (
	RepositorySubscriptionActionWatch  = "watch"
	RepositorySubscriptionActionIgnore = "ignore"
	RepositorySubscriptionActionDelete = "delete"
)

// ManageRepositoryNotificationSubscription creates a tool to manage a repository notification subscription (ignore, watch, delete)
func ManageRepositoryNotificationSubscription(getClient GetClientFn, t translations.TranslationHelperFunc) (tool mcp.Tool, handler server.ToolHandlerFunc) {
	return mcp.NewTool("manage_repository_notification_subscription",
			mcp.WithDescription(t("TOOL_MANAGE_REPOSITORY_NOTIFICATION_SUBSCRIPTION_DESCRIPTION", "Manage a repository notification subscription: ignore, watch, or delete repository notifications subscription for the provided repository.")),
			mcp.WithToolAnnotation(mcp.ToolAnnotation{
				Title:        t("TOOL_MANAGE_REPOSITORY_NOTIFICATION_SUBSCRIPTION_USER_TITLE", "Manage repository notification subscription"),
				ReadOnlyHint: ToBoolPtr(false),
			}),
			mcp.WithString(ownerParam,
				mcp.Required(),
				mcp.Description("The account owner of the repository."),
			),
			mcp.WithString("repo",
				mcp.Required(),
				mcp.Description("The name of the repository."),
			),
			mcp.WithString(actionParam,
				mcp.Required(),
				mcp.Description("Action to perform: ignore, watch, or delete the repository notification subscription."),
				mcp.Enum(RepositorySubscriptionActionIgnore, RepositorySubscriptionActionWatch, RepositorySubscriptionActionDelete),
			),
		),
		func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			client, err := getClient(ctx)
			if err != nil {
				return nil, fmt.Errorf(errFailedToGetClient, err)
			}

			owner, err := RequiredParam[string](request, ownerParam)
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			repo, err := RequiredParam[string](request, "repo")
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			action, err := RequiredParam[string](request, actionParam)
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}

			var (
				resp   *github.Response
				result any
				apiErr error
			)

			switch action {
			case RepositorySubscriptionActionIgnore:
				sub := &github.Subscription{Ignored: ToBoolPtr(true)}
				result, resp, apiErr = client.Activity.SetRepositorySubscription(ctx, owner, repo, sub)
			case RepositorySubscriptionActionWatch:
				sub := &github.Subscription{Ignored: ToBoolPtr(false), Subscribed: ToBoolPtr(true)}
				result, resp, apiErr = client.Activity.SetRepositorySubscription(ctx, owner, repo, sub)
			case RepositorySubscriptionActionDelete:
				resp, apiErr = client.Activity.DeleteRepositorySubscription(ctx, owner, repo)
			default:
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

			r, err := json.Marshal(result)
			if err != nil {
				return nil, fmt.Errorf(errFailedToMarshalResponse, err)
			}
			return mcp.NewToolResultText(string(r)), nil
		}
}
