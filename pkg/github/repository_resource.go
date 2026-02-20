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

func parseResourceArgs(args map[string]any) (owner, repo, path string, err error) {
	o, ok := args["owner"].([]string)
	if !ok || len(o) == 0 {
		return "", "", "", errors.New("owner is required")
	}
	r, ok := args["repo"].([]string)
	if !ok || len(r) == 0 {
		return o[0], "", "", errors.New("repo is required")
	}
	p, ok := args["path"].([]string)
	if ok {
		path = strings.Join(p, "/")
	}
	return o[0], r[0], path, nil
}

func resolveResourceRef(ctx context.Context, getClient GetClientFn, args map[string]any, owner, repo string) (*raw.ContentOpts, error) {
	rawOpts := &raw.ContentOpts{}

	if sha, ok := args["sha"].([]string); ok && len(sha) > 0 {
		rawOpts.SHA = sha[0]
	}
	if branch, ok := args["branch"].([]string); ok && len(branch) > 0 {
		rawOpts.Ref = "refs/heads/" + branch[0]
	}
	if tag, ok := args["tag"].([]string); ok && len(tag) > 0 {
		rawOpts.Ref = "refs/tags/" + tag[0]
	}
	if prNumber, ok := args["prNumber"].([]string); ok && len(prNumber) > 0 {
		githubClient, err := getClient(ctx)
		if err != nil {
			return nil, fmt.Errorf("failed to get GitHub client: %w", err)
		}
		prNum, err := strconv.Atoi(prNumber[0])
		if err != nil {
			return nil, fmt.Errorf("invalid pull request number: %w", err)
		}
		pr, _, err := githubClient.PullRequests.Get(ctx, owner, repo, prNum)
		if err != nil {
			return nil, fmt.Errorf("failed to get pull request: %w", err)
		}
		sha := pr.GetHead().GetSHA()
		rawOpts.SHA = sha
	}
	return rawOpts, nil
}

func inferMIMEType(path string, headerContentType string) string {
	ext := filepath.Ext(path)
	if ext == ".md" {
		return "text/markdown"
	}
	if headerContentType != "" {
		return headerContentType
	}
	return mime.TypeByExtension(ext)
}

func buildResourceResponse(uri, mimeType string, content []byte) []mcp.ResourceContents {
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
		owner, repo, path, err := parseResourceArgs(request.Params.Arguments)
		if err != nil {
			return nil, err
		}

		if path == "" || strings.HasSuffix(path, "/") {
			return nil, fmt.Errorf("directories are not supported: %s", path)
		}

		rawOpts, err := resolveResourceRef(ctx, getClient, request.Params.Arguments, owner, repo)
		if err != nil {
			return nil, err
		}

		rawClient, err := getRawClient(ctx)
		if err != nil {
			return nil, fmt.Errorf("failed to get GitHub raw content client: %w", err)
		}

		resp, err := rawClient.GetRawContent(ctx, owner, repo, path, rawOpts)
		defer func() { _ = resp.Body.Close() }()

		switch {
		case err != nil:
			return nil, fmt.Errorf("failed to get raw content: %w", err)
		case resp.StatusCode == http.StatusOK:
			content, err := io.ReadAll(resp.Body)
			if err != nil {
				return nil, fmt.Errorf("failed to read file content: %w", err)
			}
			mimeType := inferMIMEType(path, resp.Header.Get("Content-Type"))
			return buildResourceResponse(request.Params.URI, mimeType, content), nil
		case resp.StatusCode != http.StatusNotFound:
			body, err := io.ReadAll(resp.Body)
			if err != nil {
				return nil, fmt.Errorf("failed to read response body: %w", err)
			}
			return nil, fmt.Errorf("failed to fetch raw content: %s", string(body))
		default:
			return nil, errors.New("404 Not Found")
		}
	}
}
