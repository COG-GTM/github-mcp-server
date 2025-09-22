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
	ParamWorkflowID = "The workflow ID or workflow file name"
	ParamResultsPerPage = "The number of results per page (max 100)"
	ParamPageNumber = "The page number of the results to fetch"
	ParamSortOrder = "Sort order"
	
	RepoURIPrefix = "repo://"
	
	MainBranchRef = "refs/heads/main"
)

func ToBoolPtr(b bool) *bool {
	return &b
}

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

func WithWorkflowID() mcp.ToolOption {
	return mcp.WithString("workflow_id", mcp.Required(), mcp.Description(ParamWorkflowID))
}

func WithSortOrder() mcp.ToolOption {
	return mcp.WithString("order", mcp.Description(ParamSortOrder), mcp.Enum("asc", "desc"))
}

func CreateBasicTool(name, description, userTitle string, readOnly bool, additionalParams ...mcp.ToolOption) mcp.Tool {
	options := []mcp.ToolOption{
		mcp.WithDescription(description),
		mcp.WithToolAnnotation(mcp.ToolAnnotation{
			Title:        userTitle,
			ReadOnlyHint: ToBoolPtr(readOnly),
		}),
		WithOwnerRepo(),
	}
	options = append(options, additionalParams...)
	return mcp.NewTool(name, options...)
}

func CreateWorkflowTool(name, description, userTitle string, additionalParams ...mcp.ToolOption) mcp.Tool {
	options := []mcp.ToolOption{
		mcp.WithDescription(description),
		mcp.WithToolAnnotation(mcp.ToolAnnotation{
			Title:        userTitle,
			ReadOnlyHint: ToBoolPtr(true),
		}),
		WithOwnerRepo(),
		WithWorkflowRunID(),
	}
	options = append(options, additionalParams...)
	return mcp.NewTool(name, options...)
}

func CreateWorkflowIDTool(name, description, userTitle string, additionalParams ...mcp.ToolOption) mcp.Tool {
	options := []mcp.ToolOption{
		mcp.WithDescription(description),
		mcp.WithToolAnnotation(mcp.ToolAnnotation{
			Title:        userTitle,
			ReadOnlyHint: ToBoolPtr(true),
		}),
		WithOwnerRepo(),
		WithWorkflowID(),
	}
	options = append(options, additionalParams...)
	return mcp.NewTool(name, options...)
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

func ValidateWorkflowParams(request mcp.CallToolRequest) (owner, repo string, runID int64, err error) {
	owner, repo, err = ValidateOwnerRepo(request)
	if err != nil {
		return "", "", 0, err
	}
	runIDInt, err := RequiredInt(request, "run_id")
	if err != nil {
		return "", "", 0, err
	}
	return owner, repo, int64(runIDInt), nil
}

func ValidateWorkflowIDParams(request mcp.CallToolRequest) (owner, repo, workflowID string, err error) {
	owner, repo, err = ValidateOwnerRepo(request)
	if err != nil {
		return "", "", "", err
	}
	workflowID, err = RequiredParam[string](request, "workflow_id")
	if err != nil {
		return "", "", "", err
	}
	return owner, repo, workflowID, nil
}

func HandleStandardToolError(err error) (*mcp.CallToolResult, error) {
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	return nil, nil
}

func GetClientWithError(ctx context.Context, getClient func(context.Context) (*github.Client, error)) (*github.Client, error) {
	client, err := getClient(ctx)
	if err != nil {
		return nil, fmt.Errorf(ErrFailedToGetGitHubClient, err)
	}
	return client, nil
}
