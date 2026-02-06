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

const (
	descRepoOwner    = "Repository owner"
	descRepoName     = "Repository name"
	errGQLClientFmt  = "failed to get GitHub GQL client: %v"
	categoryLabelFmt = "category:%s"
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
				mcp.Description(descRepoOwner),
			),
			mcp.WithString("repo",
				mcp.Required(),
				mcp.Description(descRepoName),
			),
			mcp.WithString("category",
				mcp.Description("Optional filter by discussion category ID. If provided, only discussions with this category are listed."),
			),
		),
		func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			// Required params
			owner, err := RequiredParam[string](request, "owner")
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			repo, err := RequiredParam[string](request, "repo")
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}

			// Optional params
			category, err := OptionalParam[string](request, "category")
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}

			client, err := getGQLClient(ctx)
			if err != nil {
				return mcp.NewToolResultError(fmt.Sprintf(errGQLClientFmt, err)), nil
			}

			// If category filter is specified, use it as the category ID for server-side filtering
			var categoryID *githubv4.ID
			if category != "" {
				id := githubv4.ID(category)
				categoryID = &id
			}

			// Now execute the discussions query
			var discussions []*github.Issue
			if categoryID != nil {
				// Query with category filter (server-side filtering)
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
					"categoryId": *categoryID,
				}
				if err := client.Query(ctx, &query, vars); err != nil {
					return mcp.NewToolResultError(err.Error()), nil
				}

				// Map nodes to GitHub Issue objects
				for _, n := range query.Repository.Discussions.Nodes {
					di := newIssueFromNode(n.Number, n.Title, n.CreatedAt, n.Category.Name, n.URL)
					discussions = append(discussions, di)
				}
			} else {
				// Query without category filter
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
					return mcp.NewToolResultError(err.Error()), nil
				}

				// Map nodes to GitHub Issue objects
				for _, n := range query.Repository.Discussions.Nodes {
					di := newIssueFromNode(n.Number, n.Title, n.CreatedAt, n.Category.Name, n.URL)
					discussions = append(discussions, di)
				}
			}

			// Marshal and return
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
				mcp.Description(descRepoOwner),
			),
			mcp.WithString("repo",
				mcp.Required(),
				mcp.Description(descRepoName),
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
				return mcp.NewToolResultError(fmt.Sprintf(errGQLClientFmt, err)), nil
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
						Name: github.Ptr(fmt.Sprintf(categoryLabelFmt, string(d.Category.Name))),
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
			mcp.WithString("owner", mcp.Required(), mcp.Description(descRepoOwner)),
			mcp.WithString("repo", mcp.Required(), mcp.Description(descRepoName)),
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
				return mcp.NewToolResultError(fmt.Sprintf(errGQLClientFmt, err)), nil
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
				mcp.Description(descRepoOwner),
			),
			mcp.WithString("repo",
				mcp.Required(),
				mcp.Description(descRepoName),
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
			// Decode params
			var params struct {
				Owner  string
				Repo   string
				First  int32
				Last   int32
				After  string
				Before string
			}
			if err := mapstructure.Decode(request.Params.Arguments, &params); err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}

			// Validate pagination parameters
			if err := validatePaginationParams(params.First, params.Last, params.After, params.Before); err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}

			client, err := getGQLClient(ctx)
			if err != nil {
				return mcp.NewToolResultError(fmt.Sprintf(errGQLClientFmt, err)), nil
			}
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
				"owner": githubv4.String(params.Owner),
				"repo":  githubv4.String(params.Repo),
			}
			if err := client.Query(ctx, &q, vars); err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			var categories []map[string]string
			for _, c := range q.Repository.DiscussionCategories.Nodes {
				categories = append(categories, map[string]string{
					"id":   fmt.Sprint(c.ID),
					"name": string(c.Name),
				})
			}
			out, err := json.Marshal(categories)
			if err != nil {
				return nil, fmt.Errorf("failed to marshal discussion categories: %w", err)
			}
			return mcp.NewToolResultText(string(out)), nil
		}
}

func validatePaginationParams(first, last int32, after, before string) error {
	if first != 0 && last != 0 {
		return fmt.Errorf("only one of 'first' or 'last' may be specified")
	}
	if after != "" && before != "" {
		return fmt.Errorf("only one of 'after' or 'before' may be specified")
	}
	if after != "" && last != 0 {
		return fmt.Errorf("'after' cannot be used with 'last'. Did you mean to use 'before' instead?")
	}
	if before != "" && first != 0 {
		return fmt.Errorf("'before' cannot be used with 'first'. Did you mean to use 'after' instead?")
	}
	return nil
}

func newIssueFromNode(number githubv4.Int, title githubv4.String, createdAt githubv4.DateTime, categoryName githubv4.String, url githubv4.String) *github.Issue {
	return &github.Issue{
		Number:    github.Ptr(int(number)),
		Title:     github.Ptr(string(title)),
		HTMLURL:   github.Ptr(string(url)),
		CreatedAt: &github.Timestamp{Time: createdAt.Time},
		Labels: []*github.Label{
			{
				Name: github.Ptr(fmt.Sprintf(categoryLabelFmt, string(categoryName))),
			},
		},
	}
}
