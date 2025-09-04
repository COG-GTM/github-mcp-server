package github

import (
	"context"

	"github.com/github/github-mcp-server/pkg/translations"
	"github.com/google/go-github/v72/github"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

// GetMe creates a tool to get details of the authenticated user.
func GetMe(getClient GetClientFn, t translations.TranslationHelperFunc) (mcp.Tool, server.ToolHandlerFunc) {
	config := ToolConfig{
		Name:        "get_me",
		Description: t("TOOL_GET_ME_DESCRIPTION", "Get details of the authenticated GitHub user. Use this when a request includes \"me\", \"my\". The output will not change unless the user changes their profile, so only call this once."),
		Title:       t("TOOL_GET_ME_USER_TITLE", "Get my user profile"),
		ReadOnly:    true,
		Parameters: []ParameterConfig{
			{Name: "reason", Type: "string", Required: false, Description: "Optional: the reason for requesting the user information"},
		},
		Handler: ToolHandlerConfig{
			APICall: func(ctx context.Context, client *github.Client, _ map[string]interface{}) (interface{}, *github.Response, error) {
				return client.Users.Get(ctx, "")
			},
			ErrorPrefix: "failed to get user",
		},
	}
	return CreateGitHubTool(getClient, t, config)
}
