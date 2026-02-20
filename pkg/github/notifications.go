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

	errGetClientFmt = "failed to get GitHub client: %w"
	errReadBodyFmt  = "failed to read response body: %w"
	errMarshalFmt   = "failed to marshal response: %w"
)

type listNotificationsParams struct {
	filter  string
	since   string
	before  string
	owner   string
	repo    string
	page    int
	perPage int
}

func parseListNotificationsParams(request mcp.CallToolRequest) (listNotificationsParams, error) {
	var p listNotificationsParams
	var err error

	p.filter, err = OptionalParam[string](request, "filter")
	if err != nil {
		return p, err
	}
	p.since, err = OptionalParam[string](request, "since")
	if err != nil {
		return p, err
	}
	p.before, err = OptionalParam[string](request, "before")
	if err != nil {
		return p, err
	}
	p.owner, err = OptionalParam[string](request, "owner")
	if err != nil {
		return p, err
	}
	p.repo, err = OptionalParam[string](request, "repo")
	if err != nil {
		return p, err
	}
	pagination, err := OptionalPaginationParams(request)
	if err != nil {
		return p, err
	}
	p.page = pagination.page
	p.perPage = pagination.perPage
	return p, nil
}

func buildNotificationListOptions(p listNotificationsParams) (*github.NotificationListOptions, error) {
	opts := &github.NotificationListOptions{
		All:           p.filter == FilterIncludeRead,
		Participating: p.filter == FilterOnlyParticipating,
		ListOptions: github.ListOptions{
			Page:    p.page,
			PerPage: p.perPage,
		},
	}
	if p.since != "" {
		sinceTime, err := time.Parse(time.RFC3339, p.since)
		if err != nil {
			return nil, fmt.Errorf("invalid since time format, should be RFC3339/ISO8601: %v", err)
		}
		opts.Since = sinceTime
	}
	if p.before != "" {
		beforeTime, err := time.Parse(time.RFC3339, p.before)
		if err != nil {
			return nil, fmt.Errorf("invalid before time format, should be RFC3339/ISO8601: %v", err)
		}
		opts.Before = beforeTime
	}
	return opts, nil
}

func fetchNotifications(ctx context.Context, client *github.Client, p listNotificationsParams, opts *github.NotificationListOptions) ([]*github.Notification, *github.Response, error) {
	if p.owner != "" && p.repo != "" {
		return client.Activity.ListRepositoryNotifications(ctx, p.owner, p.repo, opts)
	}
	return client.Activity.ListNotifications(ctx, opts)
}

func handleListNotifications(ctx context.Context, client *github.Client, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	p, err := parseListNotificationsParams(request)
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}

	opts, err := buildNotificationListOptions(p)
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}

	notifications, resp, err := fetchNotifications(ctx, client, p, opts)
	if err != nil {
		return ghErrors.NewGitHubAPIErrorResponse(ctx, "failed to list notifications", resp, err), nil
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		body, err := io.ReadAll(resp.Body)
		if err != nil {
			return nil, fmt.Errorf(errReadBodyFmt, err)
		}
		return mcp.NewToolResultError(fmt.Sprintf("failed to get notifications: %s", string(body))), nil
	}

	r, err := json.Marshal(notifications)
	if err != nil {
		return nil, fmt.Errorf(errMarshalFmt, err)
	}
	return mcp.NewToolResultText(string(r)), nil
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
		func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			client, err := getClient(ctx)
			if err != nil {
				return nil, fmt.Errorf(errGetClientFmt, err)
			}
			return handleListNotifications(ctx, client, request)
		}
}

func executeDismissAction(ctx context.Context, client *github.Client, threadID, state string) (*mcp.CallToolResult, error) {
	var resp *github.Response
	var err error

	switch state {
	case "done":
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
		body, err := io.ReadAll(resp.Body)
		if err != nil {
			return nil, fmt.Errorf(errReadBodyFmt, err)
		}
		return mcp.NewToolResultError(fmt.Sprintf("failed to mark notification as %s: %s", state, string(body))), nil
	}

	return mcp.NewToolResultText(fmt.Sprintf("Notification marked as %s", state)), nil
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
				return nil, fmt.Errorf(errGetClientFmt, err)
			}

			threadID, err := RequiredParam[string](request, "threadID")
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}

			state, err := RequiredParam[string](request, "state")
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}

			return executeDismissAction(ctx, client, threadID, state)
		}
}

type markAllReadParams struct {
	lastReadAt string
	owner      string
	repo       string
}

func parseMarkAllReadParams(request mcp.CallToolRequest) (markAllReadParams, error) {
	var p markAllReadParams
	var err error

	p.lastReadAt, err = OptionalParam[string](request, "lastReadAt")
	if err != nil {
		return p, err
	}
	p.owner, err = OptionalParam[string](request, "owner")
	if err != nil {
		return p, err
	}
	p.repo, err = OptionalParam[string](request, "repo")
	if err != nil {
		return p, err
	}
	return p, nil
}

func resolveMarkReadTime(lastReadAt string) (time.Time, error) {
	if lastReadAt != "" {
		t, err := time.Parse(time.RFC3339, lastReadAt)
		if err != nil {
			return time.Time{}, fmt.Errorf("invalid lastReadAt time format, should be RFC3339/ISO8601: %v", err)
		}
		return t, nil
	}
	return time.Now(), nil
}

func executeMarkAllRead(ctx context.Context, client *github.Client, p markAllReadParams) (*mcp.CallToolResult, error) {
	lastReadTime, err := resolveMarkReadTime(p.lastReadAt)
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}

	markReadOptions := github.Timestamp{Time: lastReadTime}

	var resp *github.Response
	if p.owner != "" && p.repo != "" {
		resp, err = client.Activity.MarkRepositoryNotificationsRead(ctx, p.owner, p.repo, markReadOptions)
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
		body, err := io.ReadAll(resp.Body)
		if err != nil {
			return nil, fmt.Errorf(errReadBodyFmt, err)
		}
		return mcp.NewToolResultError(fmt.Sprintf("failed to mark all notifications as read: %s", string(body))), nil
	}

	return mcp.NewToolResultText("All notifications marked as read"), nil
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
		func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			client, err := getClient(ctx)
			if err != nil {
				return nil, fmt.Errorf(errGetClientFmt, err)
			}

			p, err := parseMarkAllReadParams(request)
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}

			return executeMarkAllRead(ctx, client, p)
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
			mcp.WithString("notificationID",
				mcp.Required(),
				mcp.Description("The ID of the notification"),
			),
		),
		func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			client, err := getClient(ctx)
			if err != nil {
				return nil, fmt.Errorf(errGetClientFmt, err)
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
				body, err := io.ReadAll(resp.Body)
				if err != nil {
					return nil, fmt.Errorf(errReadBodyFmt, err)
				}
				return mcp.NewToolResultError(fmt.Sprintf("failed to get notification details: %s", string(body))), nil
			}

			r, err := json.Marshal(thread)
			if err != nil {
				return nil, fmt.Errorf(errMarshalFmt, err)
			}

			return mcp.NewToolResultText(string(r)), nil
		}
}

const (
	NotificationActionIgnore = "ignore"
	NotificationActionWatch  = "watch"
	NotificationActionDelete = "delete"
)

func executeNotificationAction(ctx context.Context, client *github.Client, notificationID, action string) (*mcp.CallToolResult, error) {
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
		return mcp.NewToolResultText("Notification subscription deleted"), nil
	}

	r, err := json.Marshal(result)
	if err != nil {
		return nil, fmt.Errorf(errMarshalFmt, err)
	}
	return mcp.NewToolResultText(string(r)), nil
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
		func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			client, err := getClient(ctx)
			if err != nil {
				return nil, fmt.Errorf(errGetClientFmt, err)
			}

			notificationID, err := RequiredParam[string](request, "notificationID")
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			action, err := RequiredParam[string](request, "action")
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}

			return executeNotificationAction(ctx, client, notificationID, action)
		}
}

const (
	RepositorySubscriptionActionWatch  = "watch"
	RepositorySubscriptionActionIgnore = "ignore"
	RepositorySubscriptionActionDelete = "delete"
)

func executeRepoSubscriptionAction(ctx context.Context, client *github.Client, owner, repo, action string) (*mcp.CallToolResult, error) {
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

	if resp != nil && (resp.StatusCode < 200 || resp.StatusCode >= 300) {
		body, _ := io.ReadAll(resp.Body)
		return mcp.NewToolResultError(fmt.Sprintf("failed to %s repository subscription: %s", action, string(body))), nil
	}

	if action == RepositorySubscriptionActionDelete {
		return mcp.NewToolResultText("Repository subscription deleted"), nil
	}

	r, err := json.Marshal(result)
	if err != nil {
		return nil, fmt.Errorf(errMarshalFmt, err)
	}
	return mcp.NewToolResultText(string(r)), nil
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
		func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			client, err := getClient(ctx)
			if err != nil {
				return nil, fmt.Errorf(errGetClientFmt, err)
			}

			owner, err := RequiredParam[string](request, "owner")
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			repo, err := RequiredParam[string](request, "repo")
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			action, err := RequiredParam[string](request, "action")
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}

			return executeRepoSubscriptionAction(ctx, client, owner, repo, action)
		}
}
