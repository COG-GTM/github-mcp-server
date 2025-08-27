package github

const (
	ErrFailedToGetGitHubClient  = "failed to get GitHub client: %w"
	ErrFailedToMarshalResponse  = "failed to marshal response: %w"
	ErrFailedToReadResponseBody = "failed to read response body: %w"
)

const (
	DescRepositoryOwner   = "Repository owner"
	DescRepositoryName    = "Repository name"
	DescPullRequestNumber = "Pull request number"
)
