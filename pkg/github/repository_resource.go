package github

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/github/github-mcp-server/pkg/raw"
	"github.com/github/github-mcp-server/pkg/translations"
	"github.com/google/go-github/v72/github"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

// GetRepositoryResourceContent defines the resource template and handler for getting repository content.
func GetRepositoryResourceContent(getClient GetClientFn, getRawClient raw.GetRawClientFn, t translations.TranslationHelperFunc) (mcp.ResourceTemplate, server.ResourceTemplateHandlerFunc) {
	return mcp.NewResourceTemplate(
			"repo://{owner}/{repo}/contents{/path*}", // Resource template
			t("RESOURCE_REPOSITORY_CONTENT_DESCRIPTION", "Repository Content"),
		),
		RepositoryResourceContentsHandler(getClient, getRawClient)
}

// GetRepositoryResourceBranchContent defines the resource template and handler for getting repository content for a branch.
func GetRepositoryResourceBranchContent(getClient GetClientFn, getRawClient raw.GetRawClientFn, t translations.TranslationHelperFunc) (mcp.ResourceTemplate, server.ResourceTemplateHandlerFunc) {
	return mcp.NewResourceTemplate(
			"repo://{owner}/{repo}/refs/heads/{branch}/contents{/path*}", // Resource template
			t("RESOURCE_REPOSITORY_CONTENT_BRANCH_DESCRIPTION", "Repository Content for specific branch"),
		),
		RepositoryResourceContentsHandler(getClient, getRawClient)
}

// GetRepositoryResourceCommitContent defines the resource template and handler for getting repository content for a commit.
func GetRepositoryResourceCommitContent(getClient GetClientFn, getRawClient raw.GetRawClientFn, t translations.TranslationHelperFunc) (mcp.ResourceTemplate, server.ResourceTemplateHandlerFunc) {
	return mcp.NewResourceTemplate(
			"repo://{owner}/{repo}/sha/{sha}/contents{/path*}", // Resource template
			t("RESOURCE_REPOSITORY_CONTENT_COMMIT_DESCRIPTION", "Repository Content for specific commit"),
		),
		RepositoryResourceContentsHandler(getClient, getRawClient)
}

// GetRepositoryResourceTagContent defines the resource template and handler for getting repository content for a tag.
func GetRepositoryResourceTagContent(getClient GetClientFn, getRawClient raw.GetRawClientFn, t translations.TranslationHelperFunc) (mcp.ResourceTemplate, server.ResourceTemplateHandlerFunc) {
	return mcp.NewResourceTemplate(
			"repo://{owner}/{repo}/refs/tags/{tag}/contents{/path*}", // Resource template
			t("RESOURCE_REPOSITORY_CONTENT_TAG_DESCRIPTION", "Repository Content for specific tag"),
		),
		RepositoryResourceContentsHandler(getClient, getRawClient)
}

// GetRepositoryResourcePrContent defines the resource template and handler for getting repository content for a pull request.
func GetRepositoryResourcePrContent(getClient GetClientFn, getRawClient raw.GetRawClientFn, t translations.TranslationHelperFunc) (mcp.ResourceTemplate, server.ResourceTemplateHandlerFunc) {
	return mcp.NewResourceTemplate(
			"repo://{owner}/{repo}/refs/pull/{prNumber}/head/contents{/path*}", // Resource template
			t("RESOURCE_REPOSITORY_CONTENT_PR_DESCRIPTION", "Repository Content for specific pull request"),
		),
		RepositoryResourceContentsHandler(getClient, getRawClient)
}

// resourceRequestParams holds the extracted parameters from a resource request
type resourceRequestParams struct {
	owner string
	repo  string
	path  string
}

// extractRequiredStringArg extracts a required string argument from the request
func extractRequiredStringArg(args map[string]interface{}, key string) (string, error) {
	val, ok := args[key].([]string)
	if !ok || len(val) == 0 {
		return "", fmt.Errorf("%s is required", key)
	}
	return val[0], nil
}

// extractOptionalStringArg extracts an optional string argument from the request
func extractOptionalStringArg(args map[string]interface{}, key string) (string, bool) {
	val, ok := args[key].([]string)
	if !ok || len(val) == 0 {
		return "", false
	}
	return val[0], true
}

// extractResourceParams extracts owner, repo, and path from the request arguments
func extractResourceParams(args map[string]interface{}) (*resourceRequestParams, error) {
	owner, err := extractRequiredStringArg(args, "owner")
	if err != nil {
		return nil, err
	}
	repo, err := extractRequiredStringArg(args, "repo")
	if err != nil {
		return nil, err
	}
	path := ""
	if p, ok := args["path"].([]string); ok {
		path = strings.Join(p, "/")
	}
	return &resourceRequestParams{owner: owner, repo: repo, path: path}, nil
}

// setRefOptions sets the ref options based on the request arguments
func setRefOptions(args map[string]interface{}, opts *github.RepositoryContentGetOptions, rawOpts *raw.ContentOpts) {
	if sha, ok := extractOptionalStringArg(args, "sha"); ok {
		opts.Ref = sha
		rawOpts.SHA = sha
	}
	if branch, ok := extractOptionalStringArg(args, "branch"); ok {
		opts.Ref = "refs/heads/" + branch
		rawOpts.Ref = "refs/heads/" + branch
	}
	if tag, ok := extractOptionalStringArg(args, "tag"); ok {
		opts.Ref = "refs/tags/" + tag
		rawOpts.Ref = "refs/tags/" + tag
	}
}

// handlePRRef fetches the PR and sets the SHA for the ref options
func handlePRRef(ctx context.Context, args map[string]interface{}, owner, repo string, getClient GetClientFn, opts *github.RepositoryContentGetOptions, rawOpts *raw.ContentOpts) error {
	prNumberStr, ok := extractOptionalStringArg(args, "prNumber")
	if !ok {
		return nil
	}
	githubClient, err := getClient(ctx)
	if err != nil {
		return fmt.Errorf("failed to get GitHub client: %w", err)
	}
	prNum, err := strconv.Atoi(prNumberStr)
	if err != nil {
		return fmt.Errorf("invalid pull request number: %w", err)
	}
	pr, _, err := githubClient.PullRequests.Get(ctx, owner, repo, prNum)
	if err != nil {
		return fmt.Errorf("failed to get pull request: %w", err)
	}
	sha := pr.GetHead().GetSHA()
	rawOpts.SHA = sha
	opts.Ref = sha
	return nil
}

// determineMimeType determines the MIME type for a file based on extension and response headers
func determineMimeType(path string, contentType string) string {
	ext := filepath.Ext(path)
	if ext == ".md" {
		return "text/markdown"
	}
	if contentType != "" {
		return contentType
	}
	return mime.TypeByExtension(ext)
}

// buildResourceContents builds the appropriate resource contents based on MIME type
func buildResourceContents(uri string, mimeType string, content []byte) []mcp.ResourceContents {
	if strings.HasPrefix(mimeType, "text") || strings.HasPrefix(mimeType, "application") {
		return []mcp.ResourceContents{
			mcp.TextResourceContents{
				URI:      uri,
				MIMEType: mimeType,
				Text:     string(content),
			},
		}
	}
	return []mcp.ResourceContents{
		mcp.BlobResourceContents{
			URI:      uri,
			MIMEType: mimeType,
			Blob:     base64.StdEncoding.EncodeToString(content),
		},
	}
}

// handleRawContentResponse processes the raw content response and returns resource contents
func handleRawContentResponse(resp *http.Response, uri string, path string) ([]mcp.ResourceContents, error) {
	if resp.StatusCode == http.StatusOK {
		mimeType := determineMimeType(path, resp.Header.Get("Content-Type"))
		content, err := io.ReadAll(resp.Body)
		if err != nil {
			return nil, fmt.Errorf("failed to read file content: %w", err)
		}
		return buildResourceContents(uri, mimeType, content), nil
	}
	if resp.StatusCode != http.StatusNotFound {
		body, err := io.ReadAll(resp.Body)
		if err != nil {
			return nil, fmt.Errorf("failed to read response body: %w", err)
		}
		return nil, fmt.Errorf("failed to fetch raw content: %s", string(body))
	}
	return nil, errors.New("404 Not Found")
}

// RepositoryResourceContentsHandler returns a handler function for repository content requests.
func RepositoryResourceContentsHandler(getClient GetClientFn, getRawClient raw.GetRawClientFn) func(ctx context.Context, request mcp.ReadResourceRequest) ([]mcp.ResourceContents, error) {
	return func(ctx context.Context, request mcp.ReadResourceRequest) ([]mcp.ResourceContents, error) {
		params, err := extractResourceParams(request.Params.Arguments)
		if err != nil {
			return nil, err
		}

		if params.path == "" || strings.HasSuffix(params.path, "/") {
			return nil, fmt.Errorf("directories are not supported: %s", params.path)
		}

		opts := &github.RepositoryContentGetOptions{}
		rawOpts := &raw.ContentOpts{}

		setRefOptions(request.Params.Arguments, opts, rawOpts)

		if err := handlePRRef(ctx, request.Params.Arguments, params.owner, params.repo, getClient, opts, rawOpts); err != nil {
			return nil, err
		}

		rawClient, err := getRawClient(ctx)
		if err != nil {
			return nil, fmt.Errorf("failed to get GitHub raw content client: %w", err)
		}

		resp, err := rawClient.GetRawContent(ctx, params.owner, params.repo, params.path, rawOpts)
		if err != nil {
			return nil, fmt.Errorf("failed to get raw content: %w", err)
		}
		defer func() {
			_ = resp.Body.Close()
		}()

		return handleRawContentResponse(resp, request.Params.URI, params.path)
	}
}
