package github

const (
	ErrFailedToGetGitHubClient   = "failed to get GitHub client: %w"
	ErrFailedToMarshalResponse   = "failed to marshal response: %w"
	ErrFailedToReadResponseBody  = "failed to read response body: %w"
	ErrFailedToCreateResourceURI = "failed to create resource URI: %w"
	ErrFailedToGetPullRequest    = "failed to get pull request"
	ErrFailedToGetCurrentUser    = "failed to get current user"
	ErrFailedToGetLatestReview   = "failed to get latest review for current user"
	ErrFailedToGetCommit         = "failed to get commit: %s"
	ErrFailedToGetGQLClient      = "failed to get GitHub GQL client: %v"
	ErrContextMissingErrors      = "context does not contain GitHubCtxErrors"
	ErrResourceNotFound          = "resource not found"
	ErrFailedToGetRawClient      = "failed to get GitHub raw content client"
	ErrFailedToGetRawContent     = "failed to get raw repository content"
	ErrFailedToReadResponseBody2 = "failed to read response body"
	ErrRawContentNotFound        = "raw content not found"

	DescRepositoryOwner = "Repository owner"
	DescRepositoryName  = "Repository name"
	DescSortOrder       = "Sort order"
	DescPerPage         = "The number of results per page (max 100)"
	DescPageNumber      = "The page number of the results to fetch"
	DescRunID           = "The unique identifier of the workflow run"

	RepoURIPrefix  = "repo://"
	ReadOnlyMode   = "read-only"
	CategoryPrefix = "category:%s"
	RefsHeadsMain  = "refs/heads/main"
)
