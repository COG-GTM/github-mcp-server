// Package security provides centralized security utilities for the GitHub MCP server.
// It includes functions for token detection, redaction, and URL sanitization to prevent
// credential leakage through logs, error messages, and API responses.
package security

import (
	"net/url"
	"regexp"
)

// Regular expressions for detecting sensitive data patterns
var (
	// BearerTokenPattern matches "Bearer <token>" patterns in various contexts
	BearerTokenPattern = regexp.MustCompile(`(?i)(Bearer\s+)[^\s"'\]},]+`)

	// AuthTokenJSONPattern matches "authorization_token":"<value>" or 'authorization_token':'<value>' patterns in JSON
	AuthTokenJSONPattern = regexp.MustCompile(`(?i)("authorization_token"\s*:\s*")[^"]*(")|('authorization_token'\s*:\s*')[^']*(')"`)

	// TokenQueryParamPattern matches common token query parameter names
	TokenQueryParamPattern = regexp.MustCompile(`(?i)(token|access_token|auth_token|api_key|apikey|secret|password)=`)
)

// RedactedTokenPlaceholder is the placeholder used when redacting tokens
const RedactedTokenPlaceholder = "****"

// RedactBearerToken redacts Bearer tokens from a string.
// It replaces "Bearer <token>" patterns with "Bearer ****".
func RedactBearerToken(input string) string {
	return BearerTokenPattern.ReplaceAllString(input, "${1}"+RedactedTokenPlaceholder)
}

// RedactAuthTokenJSON redacts authorization_token values from JSON strings.
// It replaces the token value with "****" while preserving the JSON structure.
func RedactAuthTokenJSON(input string) string {
	return AuthTokenJSONPattern.ReplaceAllString(input, "${1}"+RedactedTokenPlaceholder+"${2}${3}"+RedactedTokenPlaceholder+"${4}")
}

// RedactSensitiveData applies all redaction patterns to sanitize sensitive data.
// This is the primary function to use for sanitizing arbitrary text that may contain tokens.
func RedactSensitiveData(input string) string {
	result := RedactBearerToken(input)
	result = RedactAuthTokenJSON(result)
	return result
}

// SanitizeURL removes query parameters from URLs that may contain authentication tokens.
// This prevents token leakage when URLs are exposed in API responses or error messages.
func SanitizeURL(rawURL string) string {
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return "[URL redacted due to parsing error]"
	}

	// Remove all query parameters as they may contain tokens
	parsed.RawQuery = ""
	parsed.Fragment = ""

	return parsed.String()
}

// ContainsSensitiveData checks if a string contains patterns that look like sensitive data.
// This can be used for validation or logging decisions.
func ContainsSensitiveData(input string) bool {
	if BearerTokenPattern.MatchString(input) {
		return true
	}
	if AuthTokenJSONPattern.MatchString(input) {
		return true
	}
	if TokenQueryParamPattern.MatchString(input) {
		return true
	}
	return false
}

// SanitizeErrorMessage sanitizes an error message to remove any sensitive data.
// This should be used when constructing error messages that may be logged or returned to clients.
func SanitizeErrorMessage(message string) string {
	return RedactSensitiveData(message)
}
