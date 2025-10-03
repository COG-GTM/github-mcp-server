package github

const (
	ErrMarshalResponse  = "failed to marshal response: %w"
	ErrGetGitHubClient  = "failed to get GitHub client: %w"
	ErrReadResponseBody = "failed to read response body: %w"
	RepoURIPrefix       = "repo://"
)
