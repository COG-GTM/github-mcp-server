package github

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/github/github-mcp-server/pkg/translations"
	"github.com/go-viper/mapstructure/v2"
	"github.com/google/go-github/v72/github"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	"github.com/shurcooL/githubv4"
)

func ListDiscussions(getGQLClient GetGQLClientFn, t translations.TranslationHelperFunc) (tool mcp.Tool, handler server.ToolHandlerFunc) {
	return mcp.NewTool("list_discussions",
			mcp.WithDescription(t("TOOL_LIST_DISCUSSIONS_DESCRIPTION", "List discussions for a repository")),
			mcp.WithToolAnnotation(mcp.ToolAnnotation{
				Title:        t("TOOL_LIST_DISCUSSIONS_USER_TITLE", "List discussions"),
				ReadOnlyHint: ToBoolPtr(true),
			}),
			mcp.WithString("owner",
				mcp.Required(),
				mcp.Description(DescRepositoryOwner),
			),
			mcp.WithString("repo",
				mcp.Required(),
				mcp.Description(DescRepositoryName),
			),
			mcp.WithString("category",
				mcp.Description("Optional filter by discussion category ID. If provided, only discussions with this category are listed."),
			),
		),
		func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			owner, repo, category, err := parseListDiscussionsParams(request)
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}

			client, err := getGQLClient(ctx)
			if err != nil {
				return mcp.NewToolResultError(fmt.Sprintf(ErrFailedToGetGitHubGQLClient, err)), nil
			}

			discussions, err := fetchDiscussions(ctx, client, owner, repo, category)
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}

			out, err := json.Marshal(discussions)
			if err != nil {
				return nil, fmt.Errorf("failed to marshal discussions: %w", err)
			}
			return mcp.NewToolResultText(string(out)), nil
		}
}

func GetDiscussion(getGQLClient GetGQLClientFn, t translations.TranslationHelperFunc) (tool mcp.Tool, handler server.ToolHandlerFunc) {
	return mcp.NewTool("get_discussion",
			mcp.WithDescription(t("TOOL_GET_DISCUSSION_DESCRIPTION", "Get a specific discussion by ID")),
			mcp.WithToolAnnotation(mcp.ToolAnnotation{
				Title:        t("TOOL_GET_DISCUSSION_USER_TITLE", "Get discussion"),
				ReadOnlyHint: ToBoolPtr(true),
			}),
			mcp.WithString("owner",
				mcp.Required(),
				mcp.Description(DescRepositoryOwner),
			),
			mcp.WithString("repo",
				mcp.Required(),
				mcp.Description(DescRepositoryName),
			),
			mcp.WithNumber("discussionNumber",
				mcp.Required(),
				mcp.Description("Discussion Number"),
			),
		),
		func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			// Decode params
			var params struct {
				Owner            string
				Repo             string
				DiscussionNumber int32
			}
			if err := mapstructure.Decode(request.Params.Arguments, &params); err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			client, err := getGQLClient(ctx)
			if err != nil {
				return mcp.NewToolResultError(fmt.Sprintf(ErrFailedToGetGitHubGQLClient, err)), nil
			}

			var q struct {
				Repository struct {
					Discussion struct {
						Number    githubv4.Int
						Body      githubv4.String
						State     githubv4.String
						CreatedAt githubv4.DateTime
						URL       githubv4.String `graphql:"url"`
						Category  struct {
							Name githubv4.String
						} `graphql:"category"`
					} `graphql:"discussion(number: $discussionNumber)"`
				} `graphql:"repository(owner: $owner, name: $repo)"`
			}
			vars := map[string]interface{}{
				"owner":            githubv4.String(params.Owner),
				"repo":             githubv4.String(params.Repo),
				"discussionNumber": githubv4.Int(params.DiscussionNumber),
			}
			if err := client.Query(ctx, &q, vars); err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			d := q.Repository.Discussion
			discussion := &github.Issue{
				Number:    github.Ptr(int(d.Number)),
				Body:      github.Ptr(string(d.Body)),
				State:     github.Ptr(string(d.State)),
				HTMLURL:   github.Ptr(string(d.URL)),
				CreatedAt: &github.Timestamp{Time: d.CreatedAt.Time},
				Labels: []*github.Label{
					{
						Name: github.Ptr(fmt.Sprintf(CategoryPrefix, string(d.Category.Name))),
					},
				},
			}
			out, err := json.Marshal(discussion)
			if err != nil {
				return nil, fmt.Errorf("failed to marshal discussion: %w", err)
			}

			return mcp.NewToolResultText(string(out)), nil
		}
}

func GetDiscussionComments(getGQLClient GetGQLClientFn, t translations.TranslationHelperFunc) (tool mcp.Tool, handler server.ToolHandlerFunc) {
	return mcp.NewTool("get_discussion_comments",
			mcp.WithDescription(t("TOOL_GET_DISCUSSION_COMMENTS_DESCRIPTION", "Get comments from a discussion")),
			mcp.WithToolAnnotation(mcp.ToolAnnotation{
				Title:        t("TOOL_GET_DISCUSSION_COMMENTS_USER_TITLE", "Get discussion comments"),
				ReadOnlyHint: ToBoolPtr(true),
			}),
			mcp.WithString("owner", mcp.Required(), mcp.Description(DescRepositoryOwner)),
			mcp.WithString("repo", mcp.Required(), mcp.Description(DescRepositoryName)),
			mcp.WithNumber("discussionNumber", mcp.Required(), mcp.Description("Discussion Number")),
		),
		func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			// Decode params
			var params struct {
				Owner            string
				Repo             string
				DiscussionNumber int32
			}
			if err := mapstructure.Decode(request.Params.Arguments, &params); err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}

			client, err := getGQLClient(ctx)
			if err != nil {
				return mcp.NewToolResultError(fmt.Sprintf(ErrFailedToGetGitHubGQLClient, err)), nil
			}

			var q struct {
				Repository struct {
					Discussion struct {
						Comments struct {
							Nodes []struct {
								Body githubv4.String
							}
						} `graphql:"comments(first:100)"`
					} `graphql:"discussion(number: $discussionNumber)"`
				} `graphql:"repository(owner: $owner, name: $repo)"`
			}
			vars := map[string]interface{}{
				"owner":            githubv4.String(params.Owner),
				"repo":             githubv4.String(params.Repo),
				"discussionNumber": githubv4.Int(params.DiscussionNumber),
			}
			if err := client.Query(ctx, &q, vars); err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			var comments []*github.IssueComment
			for _, c := range q.Repository.Discussion.Comments.Nodes {
				comments = append(comments, &github.IssueComment{Body: github.Ptr(string(c.Body))})
			}

			out, err := json.Marshal(comments)
			if err != nil {
				return nil, fmt.Errorf("failed to marshal comments: %w", err)
			}

			return mcp.NewToolResultText(string(out)), nil
		}
}

func ListDiscussionCategories(getGQLClient GetGQLClientFn, t translations.TranslationHelperFunc) (tool mcp.Tool, handler server.ToolHandlerFunc) {
	return mcp.NewTool("list_discussion_categories",
			mcp.WithDescription(t("TOOL_LIST_DISCUSSION_CATEGORIES_DESCRIPTION", "List discussion categories with their id and name, for a repository")),
			mcp.WithToolAnnotation(mcp.ToolAnnotation{
				Title:        t("TOOL_LIST_DISCUSSION_CATEGORIES_USER_TITLE", "List discussion categories"),
				ReadOnlyHint: ToBoolPtr(true),
			}),
			mcp.WithString("owner",
				mcp.Required(),
				mcp.Description(DescRepositoryOwner),
			),
			mcp.WithString("repo",
				mcp.Required(),
				mcp.Description(DescRepositoryName),
			),
			mcp.WithNumber("first",
				mcp.Description("Number of categories to return per page (min 1, max 100)"),
				mcp.Min(1),
				mcp.Max(100),
			),
			mcp.WithNumber("last",
				mcp.Description("Number of categories to return from the end (min 1, max 100)"),
				mcp.Min(1),
				mcp.Max(100),
			),
			mcp.WithString("after",
				mcp.Description("Cursor for pagination, use the 'after' field from the previous response"),
			),
			mcp.WithString("before",
				mcp.Description("Cursor for pagination, use the 'before' field from the previous response"),
			),
		),
		func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			params, err := parseDiscussionCategoriesParams(request)
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}

			if err := validatePaginationParams(params); err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}

			client, err := getGQLClient(ctx)
			if err != nil {
				return mcp.NewToolResultError(fmt.Sprintf(ErrFailedToGetGitHubGQLClient, err)), nil
			}

			categories, err := fetchDiscussionCategories(ctx, client, params.Owner, params.Repo)
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}

			out, err := json.Marshal(categories)
			if err != nil {
				return nil, fmt.Errorf("failed to marshal discussion categories: %w", err)
			}
			return mcp.NewToolResultText(string(out)), nil
		}
}

func parseListDiscussionsParams(request mcp.CallToolRequest) (owner, repo, category string, err error) {
	owner, err = RequiredParam[string](request, "owner")
	if err != nil {
		return "", "", "", err
	}
	repo, err = RequiredParam[string](request, "repo")
	if err != nil {
		return "", "", "", err
	}
	category, err = OptionalParam[string](request, "category")
	if err != nil {
		return "", "", "", err
	}
	return owner, repo, category, nil
}

func fetchDiscussions(ctx context.Context, client *githubv4.Client, owner, repo, category string) ([]*github.Issue, error) {
	var categoryID *githubv4.ID
	if category != "" {
		id := githubv4.ID(category)
		categoryID = &id
	}

	if categoryID != nil {
		return fetchDiscussionsWithCategory(ctx, client, owner, repo, *categoryID)
	}
	return fetchAllDiscussions(ctx, client, owner, repo)
}

func fetchDiscussionsWithCategory(ctx context.Context, client *githubv4.Client, owner, repo string, categoryID githubv4.ID) ([]*github.Issue, error) {
	var query struct {
		Repository struct {
			Discussions struct {
				Nodes []struct {
					Number    githubv4.Int
					Title     githubv4.String
					CreatedAt githubv4.DateTime
					Category  struct {
						Name githubv4.String
					} `graphql:"category"`
					URL githubv4.String `graphql:"url"`
				}
			} `graphql:"discussions(first: 100, categoryId: $categoryId)"`
		} `graphql:"repository(owner: $owner, name: $repo)"`
	}
	vars := map[string]interface{}{
		"owner":      githubv4.String(owner),
		"repo":       githubv4.String(repo),
		"categoryId": categoryID,
	}
	if err := client.Query(ctx, &query, vars); err != nil {
		return nil, err
	}

	return mapDiscussionNodes(query.Repository.Discussions.Nodes), nil
}

func fetchAllDiscussions(ctx context.Context, client *githubv4.Client, owner, repo string) ([]*github.Issue, error) {
	var query struct {
		Repository struct {
			Discussions struct {
				Nodes []struct {
					Number    githubv4.Int
					Title     githubv4.String
					CreatedAt githubv4.DateTime
					Category  struct {
						Name githubv4.String
					} `graphql:"category"`
					URL githubv4.String `graphql:"url"`
				}
			} `graphql:"discussions(first: 100)"`
		} `graphql:"repository(owner: $owner, name: $repo)"`
	}
	vars := map[string]interface{}{
		"owner": githubv4.String(owner),
		"repo":  githubv4.String(repo),
	}
	if err := client.Query(ctx, &query, vars); err != nil {
		return nil, err
	}

	return mapDiscussionNodes(query.Repository.Discussions.Nodes), nil
}

func mapDiscussionNodes(nodes []struct {
	Number    githubv4.Int
	Title     githubv4.String
	CreatedAt githubv4.DateTime
	Category  struct {
		Name githubv4.String
	} `graphql:"category"`
	URL githubv4.String `graphql:"url"`
}) []*github.Issue {
	var discussions []*github.Issue
	for _, n := range nodes {
		di := &github.Issue{
			Number:    github.Ptr(int(n.Number)),
			Title:     github.Ptr(string(n.Title)),
			HTMLURL:   github.Ptr(string(n.URL)),
			CreatedAt: &github.Timestamp{Time: n.CreatedAt.Time},
			Labels: []*github.Label{
				{
					Name: github.Ptr(fmt.Sprintf(CategoryPrefix, string(n.Category.Name))),
				},
			},
		}
		discussions = append(discussions, di)
	}
	return discussions
}

type discussionCategoriesParams struct {
	Owner  string
	Repo   string
	First  int32
	Last   int32
	After  string
	Before string
}

func parseDiscussionCategoriesParams(request mcp.CallToolRequest) (*discussionCategoriesParams, error) {
	var params discussionCategoriesParams
	if err := mapstructure.Decode(request.Params.Arguments, &params); err != nil {
		return nil, err
	}
	return &params, nil
}

func validatePaginationParams(params *discussionCategoriesParams) error {
	if params.First != 0 && params.Last != 0 {
		return fmt.Errorf("only one of 'first' or 'last' may be specified")
	}
	if params.After != "" && params.Before != "" {
		return fmt.Errorf("only one of 'after' or 'before' may be specified")
	}
	if params.After != "" && params.Last != 0 {
		return fmt.Errorf("'after' cannot be used with 'last'. Did you mean to use 'before' instead?")
	}
	if params.Before != "" && params.First != 0 {
		return fmt.Errorf("'before' cannot be used with 'first'. Did you mean to use 'after' instead?")
	}
	return nil
}

func fetchDiscussionCategories(ctx context.Context, client *githubv4.Client, owner, repo string) ([]map[string]string, error) {
	var q struct {
		Repository struct {
			DiscussionCategories struct {
				Nodes []struct {
					ID   githubv4.ID
					Name githubv4.String
				}
			} `graphql:"discussionCategories(first: 100)"`
		} `graphql:"repository(owner: $owner, name: $repo)"`
	}
	vars := map[string]interface{}{
		"owner": githubv4.String(owner),
		"repo":  githubv4.String(repo),
	}
	if err := client.Query(ctx, &q, vars); err != nil {
		return nil, err
	}

	var categories []map[string]string
	for _, c := range q.Repository.DiscussionCategories.Nodes {
		categories = append(categories, map[string]string{
			"id":   fmt.Sprint(c.ID),
			"name": string(c.Name),
		})
	}
	return categories, nil
}
