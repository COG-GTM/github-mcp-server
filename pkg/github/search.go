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

type searchParams struct {
	query      string
	sort       string
	order      string
	pagination PaginationParams
}

func extractSearchParams(request mcp.CallToolRequest, queryParam string) (*searchParams, error) {
	query, err := RequiredParam[string](request, queryParam)
	if err != nil {
		return nil, err
	}

	sort, err := OptionalParam[string](request, "sort")
	if err != nil {
		return nil, err
	}

	order, err := OptionalParam[string](request, "order")
	if err != nil {
		return nil, err
	}

	pagination, err := OptionalPaginationParams(request)
	if err != nil {
		return nil, err
	}

	return &searchParams{
		query:      query,
		sort:       sort,
		order:      order,
		pagination: pagination,
	}, nil
}

func buildSearchOptions(params *searchParams) *github.SearchOptions {
	return &github.SearchOptions{
		Sort:  params.sort,
		Order: params.order,
		ListOptions: github.ListOptions{
			Page:    params.pagination.page,
			PerPage: params.pagination.perPage,
		},
	}
}

func handleSearchAPIError(ctx context.Context, err error, resp *github.Response, operation, query string) (*mcp.CallToolResult, error) {
	if err != nil {
		return ghErrors.NewGitHubAPIErrorResponse(ctx,
			fmt.Sprintf("failed to %s with query '%s'", operation, query),
			resp,
			err,
		), nil
	}

	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != 200 {
		body, err := io.ReadAll(resp.Body)
		if err != nil {
			return nil, fmt.Errorf(errFailedToReadResponseBody, err)
		}
		return mcp.NewToolResultError(fmt.Sprintf("failed to %s: %s", operation, string(body))), nil
	}

	return nil, nil
}

func marshalSearchResult(result interface{}) (*mcp.CallToolResult, error) {
	r, err := json.Marshal(result)
	if err != nil {
		return nil, fmt.Errorf(errFailedToMarshalResponse, err)
	}
	return mcp.NewToolResultText(string(r)), nil
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
			params, err := extractSearchParams(request, "query")
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}

			opts := &github.SearchOptions{
				ListOptions: github.ListOptions{
					Page:    params.pagination.page,
					PerPage: params.pagination.perPage,
				},
			}

			client, err := getClient(ctx)
			if err != nil {
				return nil, fmt.Errorf("failed to get GitHub client: %w", err)
			}
			result, resp, err := client.Search.Repositories(ctx, params.query, opts)
			if errorResult, handleErr := handleSearchAPIError(ctx, err, resp, "search repositories", params.query); errorResult != nil || handleErr != nil {
				if handleErr != nil {
					return nil, handleErr
				}
				return errorResult, nil
			}

			return marshalSearchResult(result)
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
			params, err := extractSearchParams(request, "q")
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}

			opts := buildSearchOptions(params)

			client, err := getClient(ctx)
			if err != nil {
				return nil, fmt.Errorf("failed to get GitHub client: %w", err)
			}

			result, resp, err := client.Search.Code(ctx, params.query, opts)
			if errorResult, handleErr := handleSearchAPIError(ctx, err, resp, "search code", params.query); errorResult != nil || handleErr != nil {
				if handleErr != nil {
					return nil, handleErr
				}
				return errorResult, nil
			}

			return marshalSearchResult(result)
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
		params, err := extractSearchParams(request, "query")
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}

		opts := buildSearchOptions(params)

		client, err := getClient(ctx)
		if err != nil {
			return nil, fmt.Errorf("failed to get GitHub client: %w", err)
		}

		searchQuery := "type:" + accountType + " " + params.query
		result, resp, err := client.Search.Users(ctx, searchQuery, opts)
		if errorResult, handleErr := handleSearchAPIError(ctx, err, resp, fmt.Sprintf("search %ss", accountType), params.query); errorResult != nil || handleErr != nil {
			if handleErr != nil {
				return nil, handleErr
			}
			return errorResult, nil
		}

		minimalUsers := make([]MinimalUser, 0, len(result.Users))

		for _, user := range result.Users {
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

		return marshalSearchResult(minimalResp)
	}
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
