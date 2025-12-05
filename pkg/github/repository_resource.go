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

// extractStringArg extracts a string argument from the request arguments
func extractStringArg(args map[string]any, key string) (string, bool) {
	v, ok := args[key].([]string)
	if !ok || len(v) == 0 {
		return "", false
	}
	return v[0], true
}

// extractPathArg extracts and joins path parts from the request arguments
func extractPathArg(args map[string]any) string {
	p, ok := args["path"].([]string)
	if !ok {
		return ""
	}
	return strings.Join(p, "/")
}

// resolveRefOptions resolves the ref options based on the request arguments
func resolveRefOptions(ctx context.Context, args map[string]any, owner, repo string, getClient GetClientFn) (*github.RepositoryContentGetOptions, *raw.ContentOpts, error) {
	opts := &github.RepositoryContentGetOptions{}
	rawOpts := &raw.ContentOpts{}

	if sha, ok := extractStringArg(args, "sha"); ok {
		opts.Ref = sha
		rawOpts.SHA = sha
		return opts, rawOpts, nil
	}

	if branch, ok := extractStringArg(args, "branch"); ok {
		opts.Ref = "refs/heads/" + branch
		rawOpts.Ref = "refs/heads/" + branch
		return opts, rawOpts, nil
	}

	if tag, ok := extractStringArg(args, "tag"); ok {
		opts.Ref = "refs/tags/" + tag
		rawOpts.Ref = "refs/tags/" + tag
		return opts, rawOpts, nil
	}

	if prNumberStr, ok := extractStringArg(args, "prNumber"); ok {
		githubClient, err := getClient(ctx)
		if err != nil {
			return nil, nil, fmt.Errorf("failed to get GitHub client: %w", err)
		}
		prNum, err := strconv.Atoi(prNumberStr)
		if err != nil {
			return nil, nil, fmt.Errorf("invalid pull request number: %w", err)
		}
		pr, _, err := githubClient.PullRequests.Get(ctx, owner, repo, prNum)
		if err != nil {
			return nil, nil, fmt.Errorf("failed to get pull request: %w", err)
		}
		sha := pr.GetHead().GetSHA()
		rawOpts.SHA = sha
		opts.Ref = sha
	}

	return opts, rawOpts, nil
}

// determineMimeType determines the MIME type for a file based on extension and response headers
func determineMimeType(path string, contentTypeHeader string) string {
	ext := filepath.Ext(path)
	if ext == ".md" {
		return "text/markdown"
	}
	if contentTypeHeader != "" {
		return contentTypeHeader
	}
	return mime.TypeByExtension(ext)
}

// buildResourceContents builds the appropriate resource contents based on MIME type
func buildResourceContents(uri, mimeType string, content []byte) []mcp.ResourceContents {
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

// RepositoryResourceContentsHandler returns a handler function for repository content requests.
func RepositoryResourceContentsHandler(getClient GetClientFn, getRawClient raw.GetRawClientFn) func(ctx context.Context, request mcp.ReadResourceRequest) ([]mcp.ResourceContents, error) {
	return func(ctx context.Context, request mcp.ReadResourceRequest) ([]mcp.ResourceContents, error) {
		args := request.Params.Arguments

		owner, ok := extractStringArg(args, "owner")
		if !ok {
			return nil, errors.New("owner is required")
		}

		repo, ok := extractStringArg(args, "repo")
		if !ok {
			return nil, errors.New("repo is required")
		}

		path := extractPathArg(args)
		if path == "" || strings.HasSuffix(path, "/") {
			return nil, fmt.Errorf("directories are not supported: %s", path)
		}

		_, rawOpts, err := resolveRefOptions(ctx, args, owner, repo, getClient)
		if err != nil {
			return nil, err
		}

		rawClient, err := getRawClient(ctx)
		if err != nil {
			return nil, fmt.Errorf("failed to get GitHub raw content client: %w", err)
		}

		resp, err := rawClient.GetRawContent(ctx, owner, repo, path, rawOpts)
		if err != nil {
			return nil, fmt.Errorf("failed to get raw content: %w", err)
		}
		defer func() { _ = resp.Body.Close() }()

		if resp.StatusCode == http.StatusOK {
			content, err := io.ReadAll(resp.Body)
			if err != nil {
				return nil, fmt.Errorf("failed to read file content: %w", err)
			}
			mimeType := determineMimeType(path, resp.Header.Get("Content-Type"))
			return buildResourceContents(request.Params.URI, mimeType, content), nil
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
}
