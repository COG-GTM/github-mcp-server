package github

import (
	"encoding/json"
	"fmt"

	"github.com/mark3labs/mcp-go/mcp"
)

const (
	errFailedToGetGitHubClient    = "failed to get GitHub client: %w"
	errFailedToMarshalResponse    = "failed to marshal response: %w"
	errFailedToGetGitHubGQLClient = "failed to get GitHub GQL client: %v"
	errFailedToListWorkflows      = "failed to list workflows: %w"
	errFailedToGetWorkflowRun     = "failed to get workflow run: %w"
	errFailedToRunWorkflow        = "failed to run workflow: %w"
	errFailedToSearchRepositories = "failed to search repositories with query '%s': %s"
	errFailedToSearchCode         = "failed to search code with query '%s': %s"
	errFailedToSearchUsers        = "failed to search users with query '%s': %s"
	errFailedToSearchOrgs         = "failed to search orgs with query '%s': %s"
	errFailedToListWorkflowJobs   = "failed to list workflow jobs: %w"
	errFailedToGetWorkflowRunLogs = "failed to get workflow run logs: %w"
	errFailedToRerunWorkflow      = "failed to rerun workflow: %w"
	errFailedToRerunFailedJobs    = "failed to rerun failed jobs: %w"
	errFailedToCancelWorkflowRun  = "failed to cancel workflow run: %w"
	errFailedToListArtifacts      = "failed to list artifacts: %w"
	errFailedToDownloadArtifact   = "failed to download artifact: %w"
	errFailedToDeleteWorkflowRunLogs = "failed to delete workflow run logs: %w"
	errFailedToGetWorkflowRunUsage   = "failed to get workflow run usage: %w"
)





func RequiredParam[T any](request mcp.CallToolRequest, key string) (T, error) {
	var zero T
	args := request.GetArguments()
	value, exists := args[key]
	if !exists {
		return zero, fmt.Errorf("missing required parameter: %s", key)
	}
	
	typedValue, ok := value.(T)
	if !ok {
		return zero, fmt.Errorf("parameter %s has incorrect type", key)
	}
	
	return typedValue, nil
}

func ValidateOwnerRepo(request mcp.CallToolRequest) (owner, repo string, result *mcp.CallToolResult) {
	owner, err := RequiredParam[string](request, "owner")
	if err != nil {
		return "", "", mcp.NewToolResultError(err.Error())
	}
	repo, err = RequiredParam[string](request, "repo")
	if err != nil {
		return "", "", mcp.NewToolResultError(err.Error())
	}
	return owner, repo, nil
}

func ValidateOwnerRepoIssue(request mcp.CallToolRequest) (owner, repo string, issueNumber int, result *mcp.CallToolResult) {
	owner, repo, result = ValidateOwnerRepo(request)
	if result != nil {
		return "", "", 0, result
	}
	issueNumber, err := RequiredParam[int](request, "issue_number")
	if err != nil {
		issueNumber, err = RequiredParam[int](request, "issueNumber")
		if err != nil {
			return "", "", 0, mcp.NewToolResultError(err.Error())
		}
	}
	return owner, repo, issueNumber, nil
}

func ValidateOwnerRepoPR(request mcp.CallToolRequest) (owner, repo string, pullNumber int, result *mcp.CallToolResult) {
	owner, repo, result = ValidateOwnerRepo(request)
	if result != nil {
		return "", "", 0, result
	}
	pullNumber, err := RequiredParam[int](request, "pull_number")
	if err != nil {
		pullNumber, err = RequiredParam[int](request, "pullNumber")
		if err != nil {
			return "", "", 0, mcp.NewToolResultError(err.Error())
		}
	}
	return owner, repo, pullNumber, nil
}

func MarshalResponse(data interface{}) (*mcp.CallToolResult, error) {
	result, err := json.Marshal(data)
	if err != nil {
		return nil, fmt.Errorf(errFailedToMarshalResponse, err)
	}
	return mcp.NewToolResultText(string(result)), nil
}
