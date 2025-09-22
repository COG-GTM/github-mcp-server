package github

import (
	"context"
	"fmt"
	"github.com/google/go-github/v72/github"
	"github.com/mark3labs/mcp-go/mcp"
)

const (
	ErrFailedToGetGitHubClient    = "failed to get GitHub client: %w"
	ErrFailedToGetGitHubGQLClient = "failed to get GitHub GQL client: %w"
	
	ErrFailedToMarshalResponse = "failed to marshal response: %w"
	ErrFailedToReadResponseBody = "failed to read response body: %w"
	
	ErrFailedToGetCommit = "failed to get commit: %s"
	ErrFailedToCreateResourceURI = "failed to create resource URI: %w"
	
	ErrFailedToGetPullRequest = "failed to get pull request"
	ErrFailedToGetCurrentUser = "failed to get current user"
	ErrFailedToGetLatestReview = "failed to get latest review for current user"
	
	ParamRepositoryOwner = "Repository owner"
	ParamRepositoryName = "Repository name"
	ParamPullRequestNumber = "Pull request number"
	ParamWorkflowRunID = "The unique identifier of the workflow run"
	ParamResultsPerPage = "The number of results per page (max 100)"
	ParamPageNumber = "The page number of the results to fetch"
	ParamSortOrder = "Sort order"
	
	RepoURIPrefix = "repo://"
	
	MainBranchRef = "refs/heads/main"
)

func WithOwnerRepo() mcp.ToolOption {
	return func(tool *mcp.Tool) {
		mcp.WithString("owner", mcp.Required(), mcp.Description(ParamRepositoryOwner))(tool)
		mcp.WithString("repo", mcp.Required(), mcp.Description(ParamRepositoryName))(tool)
	}
}



func WithPaginationPerPage() mcp.ToolOption {
	return func(tool *mcp.Tool) {
		mcp.WithNumber("per_page", mcp.Description(ParamResultsPerPage))(tool)
		mcp.WithNumber("page", mcp.Description(ParamPageNumber))(tool)
	}
}

func WithWorkflowRunID() mcp.ToolOption {
	return mcp.WithNumber("run_id", mcp.Required(), mcp.Description(ParamWorkflowRunID))
}

func WithSortOrder() mcp.ToolOption {
	return mcp.WithString("order", mcp.Description(ParamSortOrder), mcp.Enum("asc", "desc"))
}

func ValidateOwnerRepo(request mcp.CallToolRequest) (owner, repo string, err error) {
	owner, err = RequiredParam[string](request, "owner")
	if err != nil {
		return "", "", err
	}
	repo, err = RequiredParam[string](request, "repo")
	if err != nil {
		return "", "", err
	}
	return owner, repo, nil
}

func GetClientWithError(ctx context.Context, getClient func(context.Context) (*github.Client, error)) (*github.Client, error) {
	client, err := getClient(ctx)
	if err != nil {
		return nil, fmt.Errorf(ErrFailedToGetGitHubClient, err)
	}
	return client, nil
}
