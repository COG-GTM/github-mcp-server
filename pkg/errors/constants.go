package errors

// Error message constants to avoid string duplication (SonarQube rule go:S1192)
const (
	// ErrContextMissingGitHubCtxErrors is returned when the context does not contain GitHubCtxErrors
	ErrContextMissingGitHubCtxErrors = "context does not contain GitHubCtxErrors"

	// ErrFailedToGetGitHubClient is returned when getting the GitHub client fails
	ErrFailedToGetGitHubClient = "failed to get GitHub client"

	// ErrMissingRequiredParameter is a format string for missing required parameter errors
	ErrMissingRequiredParameter = "missing required parameter: %s"
)
