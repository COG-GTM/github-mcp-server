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
func parseResourceParams(request mcp.ReadResourceRequest) (owner, repo, path string, err error) {
	o, ok := request.Params.Arguments["owner"].([]string)
	if !ok || len(o) == 0 {
		return "", "", "", errors.New("owner is required")
	}
	owner = o[0]

	r, ok := request.Params.Arguments["repo"].([]string)
	if !ok || len(r) == 0 {
		return "", "", "", errors.New("repo is required")
	}
	repo = r[0]

	p, ok := request.Params.Arguments["path"].([]string)
	if ok {
		path = strings.Join(p, "/")
	}

	return owner, repo, path, nil
}

func extractRefOptions(request mcp.ReadResourceRequest, opts *github.RepositoryContentGetOptions, rawOpts *raw.ContentOpts) {
	if sha, ok := request.Params.Arguments["sha"].([]string); ok && len(sha) > 0 {
		opts.Ref = sha[0]
		rawOpts.SHA = sha[0]
	}

	if branch, ok := request.Params.Arguments["branch"].([]string); ok && len(branch) > 0 {
		opts.Ref = "refs/heads/" + branch[0]
		rawOpts.Ref = "refs/heads/" + branch[0]
	}

	if tag, ok := request.Params.Arguments["tag"].([]string); ok && len(tag) > 0 {
		opts.Ref = "refs/tags/" + tag[0]
		rawOpts.Ref = "refs/tags/" + tag[0]
	}
}

func resolvePRForResource(ctx context.Context, getClient GetClientFn, request mcp.ReadResourceRequest, owner, repo string, opts *github.RepositoryContentGetOptions, rawOpts *raw.ContentOpts) error {
	prNumber, ok := request.Params.Arguments["prNumber"].([]string)
	if !ok || len(prNumber) == 0 {
		return nil
	}

	githubClient, err := getClient(ctx)
	if err != nil {
		return fmt.Errorf("failed to get GitHub client: %w", err)
	}

	prNum, err := strconv.Atoi(prNumber[0])
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

func fetchResourceContent(ctx context.Context, getRawClient raw.GetRawClientFn, request mcp.ReadResourceRequest, owner, repo, path string, rawOpts *raw.ContentOpts) ([]mcp.ResourceContents, error) {
	rawClient, err := getRawClient(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get GitHub raw content client: %w", err)
	}

	resp, err := rawClient.GetRawContent(ctx, owner, repo, path, rawOpts)
	defer func() {
		_ = resp.Body.Close()
	}()

	if err != nil {
		return nil, fmt.Errorf("failed to get raw content: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		if resp.StatusCode == http.StatusNotFound {
			return nil, errors.New("404 Not Found")
		}
		body, err := io.ReadAll(resp.Body)
		if err != nil {
			return nil, fmt.Errorf("failed to read response body: %w", err)
		}
		return nil, fmt.Errorf("failed to fetch raw content: %s", string(body))
	}

	ext := filepath.Ext(path)
	mimeType := resp.Header.Get("Content-Type")
	if ext == ".md" {
		mimeType = "text/markdown"
	} else if mimeType == "" {
		mimeType = mime.TypeByExtension(ext)
	}

	content, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read file content: %w", err)
	}

	if strings.HasPrefix(mimeType, "text") || strings.HasPrefix(mimeType, "application") {
		return []mcp.ResourceContents{
			mcp.TextResourceContents{
				URI:      request.Params.URI,
				MIMEType: mimeType,
				Text:     string(content),
			},
		}, nil
	}

	return []mcp.ResourceContents{
		mcp.BlobResourceContents{
			URI:      request.Params.URI,
			MIMEType: mimeType,
			Blob:     base64.StdEncoding.EncodeToString(content),
		},
	}, nil
}

// RepositoryResourceContentsHandler returns a handler function for repository content requests.
func RepositoryResourceContentsHandler(getClient GetClientFn, getRawClient raw.GetRawClientFn) func(ctx context.Context, request mcp.ReadResourceRequest) ([]mcp.ResourceContents, error) {
	return func(ctx context.Context, request mcp.ReadResourceRequest) ([]mcp.ResourceContents, error) {
		owner, repo, path, err := parseResourceParams(request)
		if err != nil {
			return nil, err
		}

		opts := &github.RepositoryContentGetOptions{}
		rawOpts := &raw.ContentOpts{}

		extractRefOptions(request, opts, rawOpts)

		err = resolvePRForResource(ctx, getClient, request, owner, repo, opts, rawOpts)
		if err != nil {
			return nil, err
		}

		if path == "" || strings.HasSuffix(path, "/") {
			return nil, fmt.Errorf("directories are not supported: %s", path)
		}

		return fetchResourceContent(ctx, getRawClient, request, owner, repo, path, rawOpts)
	}
}
