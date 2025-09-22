package github

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
