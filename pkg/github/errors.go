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
	
	DescriptionRepositoryOwner = "Repository owner"
	DescriptionRepositoryName  = "Repository name"
)





func MarshalResponse(data interface{}) (*mcp.CallToolResult, error) {
	result, err := json.Marshal(data)
	if err != nil {
		return nil, fmt.Errorf(errFailedToMarshalResponse, err)
	}
	return mcp.NewToolResultText(string(result)), nil
}
