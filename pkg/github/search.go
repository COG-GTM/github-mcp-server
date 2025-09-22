package github

import (
	"context"
	"encoding/json"
	"fmt"
	"io"

	ghErrors "github.com/github/github-mcp-server/pkg/errors"
	"github.com/github/github-mcp-server/pkg/translations"
	"github.com/google/go-github/v72/github"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

const (
	errFailedToReadResponseBody = "failed to read response body: %w"
	errFailedToMarshalResponse  = "failed to marshal response: %w"
)

func handleSearchResponse(ctx context.Context, resp *github.Response, err error, operation string) (*mcp.CallToolResult, error) {
	if err != nil {
		return ghErrors.NewGitHubAPIErrorResponse(ctx, operation, resp, err), nil
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != 200 {
		body, err := io.ReadAll(resp.Body)
		if err != nil {
			return nil, fmt.Errorf(errFailedToReadResponseBody, err)
		}
		return mcp.NewToolResultError(fmt.Sprintf("%s: %s", operation, string(body))), nil
	}
	return nil, nil
}

func marshalAndReturnResult(result interface{}) (*mcp.CallToolResult, error) {
	r, err := json.Marshal(result)
	if err != nil {
		return nil, fmt.Errorf(errFailedToMarshalResponse, err)
	}
	return mcp.NewToolResultText(string(r)), nil
}

func extractSearchParams(request mcp.CallToolRequest) (query, sort, order string, pagination PaginationParams, err error) {
	query, err = RequiredParam[string](request, "query")
	if err != nil {
		return "", "", "", PaginationParams{}, err
	}
	sort, _ = OptionalParam[string](request, "sort")
	order, _ = OptionalParam[string](request, "order")
	pagination, err = OptionalPaginationParams(request)
	return query, sort, order, pagination, err
}

// SearchRepositories creates a tool to search for GitHub repositories.
func SearchRepositories(getClient GetClientFn, t translations.TranslationHelperFunc) (tool mcp.Tool, handler server.ToolHandlerFunc) {
	return mcp.NewTool("search_repositories",
			mcp.WithDescription(t("TOOL_SEARCH_REPOSITORIES_DESCRIPTION", "Search for GitHub repositories")),
			mcp.WithToolAnnotation(mcp.ToolAnnotation{
				Title:        t("TOOL_SEARCH_REPOSITORIES_USER_TITLE", "Search repositories"),
				ReadOnlyHint: ToBoolPtr(true),
			}),
			mcp.WithString("query",
				mcp.Required(),
				mcp.Description("Search query"),
			),
			WithPagination(),
		),
		func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			query, err := RequiredParam[string](request, "query")
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			pagination, err := OptionalPaginationParams(request)
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}

			client, err := getClient(ctx)
			if err != nil {
				return nil, fmt.Errorf("failed to get GitHub client: %w", err)
			}

			opts := &github.SearchOptions{
				ListOptions: github.ListOptions{
					Page:    pagination.page,
					PerPage: pagination.perPage,
				},
			}

			result, resp, err := client.Search.Repositories(ctx, query, opts)
			if errorResult, err := handleSearchResponse(ctx, resp, err, fmt.Sprintf("failed to search repositories with query '%s'", query)); errorResult != nil || err != nil {
				return errorResult, err
			}

			return marshalAndReturnResult(result)
		}
}

// SearchCode creates a tool to search for code across GitHub repositories.
func SearchCode(getClient GetClientFn, t translations.TranslationHelperFunc) (tool mcp.Tool, handler server.ToolHandlerFunc) {
	return mcp.NewTool("search_code",
			mcp.WithDescription(t("TOOL_SEARCH_CODE_DESCRIPTION", "Search for code across GitHub repositories")),
			mcp.WithToolAnnotation(mcp.ToolAnnotation{
				Title:        t("TOOL_SEARCH_CODE_USER_TITLE", "Search code"),
				ReadOnlyHint: ToBoolPtr(true),
			}),
			mcp.WithString("q",
				mcp.Required(),
				mcp.Description("Search query using GitHub code search syntax"),
			),
			mcp.WithString("sort",
				mcp.Description("Sort field ('indexed' only)"),
			),
			mcp.WithString("order",
				mcp.Description("Sort order"),
				mcp.Enum("asc", "desc"),
			),
			WithPagination(),
		),
		func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			query, err := RequiredParam[string](request, "q")
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			sort, err := OptionalParam[string](request, "sort")
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			order, err := OptionalParam[string](request, "order")
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			pagination, err := OptionalPaginationParams(request)
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}

			client, err := getClient(ctx)
			if err != nil {
				return nil, fmt.Errorf("failed to get GitHub client: %w", err)
			}

			opts := &github.SearchOptions{
				Sort:  sort,
				Order: order,
				ListOptions: github.ListOptions{
					PerPage: pagination.perPage,
					Page:    pagination.page,
				},
			}

			result, resp, err := client.Search.Code(ctx, query, opts)
			if errorResult, err := handleSearchResponse(ctx, resp, err, fmt.Sprintf("failed to search code with query '%s'", query)); errorResult != nil || err != nil {
				return errorResult, err
			}

			return marshalAndReturnResult(result)
		}
}

type MinimalUser struct {
	Login      string `json:"login"`
	ID         int64  `json:"id,omitempty"`
	ProfileURL string `json:"profile_url,omitempty"`
	AvatarURL  string `json:"avatar_url,omitempty"`
}

type MinimalSearchUsersResult struct {
	TotalCount        int           `json:"total_count"`
	IncompleteResults bool          `json:"incomplete_results"`
	Items             []MinimalUser `json:"items"`
}

func userOrOrgHandler(accountType string, getClient GetClientFn) server.ToolHandlerFunc {
	return func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		query, sort, order, pagination, err := extractSearchParams(request)
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}

		client, err := getClient(ctx)
		if err != nil {
			return nil, fmt.Errorf("failed to get GitHub client: %w", err)
		}

		opts := &github.SearchOptions{
			Sort:  sort,
			Order: order,
			ListOptions: github.ListOptions{
				PerPage: pagination.perPage,
				Page:    pagination.page,
			},
		}

		searchQuery := "type:" + accountType + " " + query
		result, resp, err := client.Search.Users(ctx, searchQuery, opts)
		if errorResult, err := handleSearchResponse(ctx, resp, err, fmt.Sprintf("failed to search %ss with query '%s'", accountType, query)); errorResult != nil || err != nil {
			return errorResult, err
		}

		minimalUsers := processSearchUsers(result.Users)
		minimalResp := &MinimalSearchUsersResult{
			TotalCount:        result.GetTotal(),
			IncompleteResults: result.GetIncompleteResults(),
			Items:             minimalUsers,
		}
		if result.Total != nil {
			minimalResp.TotalCount = *result.Total
		}
		if result.IncompleteResults != nil {
			minimalResp.IncompleteResults = *result.IncompleteResults
		}

		return marshalAndReturnResult(minimalResp)
	}
}

func processSearchUsers(users []*github.User) []MinimalUser {
	minimalUsers := make([]MinimalUser, 0, len(users))
	for _, user := range users {
		if user.Login != nil {
			mu := MinimalUser{Login: *user.Login}
			if user.ID != nil {
				mu.ID = *user.ID
			}
			if user.HTMLURL != nil {
				mu.ProfileURL = *user.HTMLURL
			}
			if user.AvatarURL != nil {
				mu.AvatarURL = *user.AvatarURL
			}
			minimalUsers = append(minimalUsers, mu)
		}
	}
	return minimalUsers
}

// SearchUsers creates a tool to search for GitHub users.
func SearchUsers(getClient GetClientFn, t translations.TranslationHelperFunc) (tool mcp.Tool, handler server.ToolHandlerFunc) {
	return mcp.NewTool("search_users",
		mcp.WithDescription(t("TOOL_SEARCH_USERS_DESCRIPTION", "Search for GitHub users exclusively")),
		mcp.WithToolAnnotation(mcp.ToolAnnotation{
			Title:        t("TOOL_SEARCH_USERS_USER_TITLE", "Search users"),
			ReadOnlyHint: ToBoolPtr(true),
		}),
		mcp.WithString("query",
			mcp.Required(),
			mcp.Description("Search query using GitHub users search syntax scoped to type:user"),
		),
		mcp.WithString("sort",
			mcp.Description("Sort field by category"),
			mcp.Enum("followers", "repositories", "joined"),
		),
		mcp.WithString("order",
			mcp.Description("Sort order"),
			mcp.Enum("asc", "desc"),
		),
		WithPagination(),
	), userOrOrgHandler("user", getClient)
}

// SearchOrgs creates a tool to search for GitHub organizations.
func SearchOrgs(getClient GetClientFn, t translations.TranslationHelperFunc) (tool mcp.Tool, handler server.ToolHandlerFunc) {
	return mcp.NewTool("search_orgs",
		mcp.WithDescription(t("TOOL_SEARCH_ORGS_DESCRIPTION", "Search for GitHub organizations exclusively")),
		mcp.WithToolAnnotation(mcp.ToolAnnotation{
			Title:        t("TOOL_SEARCH_ORGS_USER_TITLE", "Search organizations"),
			ReadOnlyHint: ToBoolPtr(true),
		}),
		mcp.WithString("query",
			mcp.Required(),
			mcp.Description("Search query using GitHub organizations search syntax scoped to type:org"),
		),
		mcp.WithString("sort",
			mcp.Description("Sort field by category"),
			mcp.Enum("followers", "repositories", "joined"),
		),
		mcp.WithString("order",
			mcp.Description("Sort order"),
			mcp.Enum("asc", "desc"),
		),
		WithPagination(),
	), userOrOrgHandler("org", getClient)
}
