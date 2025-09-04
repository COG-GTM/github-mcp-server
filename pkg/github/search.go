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

// SearchRepositories creates a tool to search for GitHub repositories.
func SearchRepositories(getClient GetClientFn, t translations.TranslationHelperFunc) (tool mcp.Tool, handler server.ToolHandlerFunc) {
	config := ToolConfig{
		Name:        "search_repositories",
		Description: t("TOOL_SEARCH_REPOSITORIES_DESCRIPTION", "Search for GitHub repositories"),
		Title:       t("TOOL_SEARCH_REPOSITORIES_USER_TITLE", "Search repositories"),
		ReadOnly:    true,
		Parameters: []ParameterConfig{
			{Name: "query", Type: "string", Required: true, Description: "Search query"},
		},
		Handler: ToolHandlerConfig{
			APICall: func(ctx context.Context, client *github.Client, params map[string]interface{}) (interface{}, *github.Response, error) {
				opts := &github.SearchOptions{}
				if page, ok := params["page"].(int); ok {
					opts.ListOptions.Page = page
				}
				if perPage, ok := params["perPage"].(int); ok {
					opts.ListOptions.PerPage = perPage
				}
				return client.Search.Repositories(ctx, params["query"].(string), opts)
			},
			ErrorPrefix: "failed to search repositories",
		},
	}
	return CreateGitHubTool(getClient, t, config)
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

			opts := &github.SearchOptions{
				Sort:  sort,
				Order: order,
				ListOptions: github.ListOptions{
					PerPage: pagination.perPage,
					Page:    pagination.page,
				},
			}

			client, err := getClient(ctx)
			if err != nil {
				return nil, fmt.Errorf("failed to get GitHub client: %w", err)
			}

			result, resp, err := client.Search.Code(ctx, query, opts)
			if err != nil {
				return ghErrors.NewGitHubAPIErrorResponse(ctx,
					fmt.Sprintf("failed to search code with query '%s'", query),
					resp,
					err,
				), nil
			}
			defer func() { _ = resp.Body.Close() }()

			if resp.StatusCode != 200 {
				body, err := io.ReadAll(resp.Body)
				if err != nil {
					return nil, fmt.Errorf(ErrFailedToReadResponseBody, err)
				}
				return mcp.NewToolResultError(fmt.Sprintf("failed to search code: %s", string(body))), nil
			}

			r, err := json.Marshal(result)
			if err != nil {
				return nil, fmt.Errorf(ErrFailedToMarshalResponse, err)
			}

			return mcp.NewToolResultText(string(r)), nil
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

type searchParams struct {
	query      string
	sort       string
	order      string
	pagination PaginationParams
}

func userOrOrgHandler(accountType string, getClient GetClientFn) server.ToolHandlerFunc {
	return func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		params, err := extractSearchParams(request)
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}

		client, err := getClient(ctx)
		if err != nil {
			return nil, fmt.Errorf("failed to get GitHub client: %w", err)
		}

		result, resp, err := performUserSearch(ctx, client, accountType, params)
		if err != nil {
			return ghErrors.NewGitHubAPIErrorResponse(ctx,
				fmt.Sprintf("failed to search %ss with query '%s'", accountType, params.query),
				resp,
				err,
			), nil
		}
		defer func() { _ = resp.Body.Close() }()

		if resp.StatusCode != 200 {
			return handleSearchError(resp, accountType)
		}

		minimalResp := buildMinimalSearchResult(result)
		return marshalSearchResponse(minimalResp)
	}
}

func extractSearchParams(request mcp.CallToolRequest) (*searchParams, error) {
	query, err := RequiredParam[string](request, "query")
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

func performUserSearch(ctx context.Context, client *github.Client, accountType string, params *searchParams) (*github.UsersSearchResult, *github.Response, error) {
	opts := &github.SearchOptions{
		Sort:  params.sort,
		Order: params.order,
		ListOptions: github.ListOptions{
			PerPage: params.pagination.perPage,
			Page:    params.pagination.page,
		},
	}

	searchQuery := "type:" + accountType + " " + params.query
	result, resp, err := client.Search.Users(ctx, searchQuery, opts)
	if err != nil {
		return nil, resp, fmt.Errorf("failed to search %ss with query '%s': %w", accountType, params.query, err)
	}
	return result, resp, nil
}

func handleSearchError(resp *github.Response, accountType string) (*mcp.CallToolResult, error) {
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf(ErrFailedToReadResponseBody, err)
	}
	return mcp.NewToolResultError(fmt.Sprintf("failed to search %ss: %s", accountType, string(body))), nil
}

func buildMinimalSearchResult(result *github.UsersSearchResult) *MinimalSearchUsersResult {
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

	return minimalResp
}

func marshalSearchResponse(minimalResp *MinimalSearchUsersResult) (*mcp.CallToolResult, error) {
	r, err := json.Marshal(minimalResp)
	if err != nil {
		return nil, fmt.Errorf(ErrFailedToMarshalResponse, err)
	}
	return mcp.NewToolResultText(string(r)), nil
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
