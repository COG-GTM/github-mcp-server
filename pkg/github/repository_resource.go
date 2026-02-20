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

// RepositoryResourceContentsHandler returns a handler function for repository content requests.
func RepositoryResourceContentsHandler(getClient GetClientFn, getRawClient raw.GetRawClientFn) func(ctx context.Context, request mcp.ReadResourceRequest) ([]mcp.ResourceContents, error) {
	return func(ctx context.Context, request mcp.ReadResourceRequest) ([]mcp.ResourceContents, error) {
		owner, err := requiredStringArg(request.Params.Arguments, "owner")
		if err != nil {
			return nil, err
		}

		repo, err := requiredStringArg(request.Params.Arguments, "repo")
		if err != nil {
			return nil, err
		}

		p, _ := request.Params.Arguments["path"].([]string)
		path := strings.Join(p, "/")

		if path == "" || strings.HasSuffix(path, "/") {
			return nil, fmt.Errorf("directories are not supported: %s", path)
		}

		rawOpts, err := resolveContentRef(ctx, getClient, request.Params.Arguments, owner, repo)
		if err != nil {
			return nil, err
		}

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
			return handleNonOKResponse(resp)
		}

		return buildResourceContents(request.Params.URI, path, resp)
	}
}

func requiredStringArg(args map[string]any, key string) (string, error) {
	v, ok := args[key].([]string)
	if !ok || len(v) == 0 {
		return "", fmt.Errorf("%s is required", key)
	}
	return v[0], nil
}

func optionalStringArg(args map[string]any, key string) string {
	v, ok := args[key].([]string)
	if ok && len(v) > 0 {
		return v[0]
	}
	return ""
}

func resolveContentRef(ctx context.Context, getClient GetClientFn, args map[string]any, owner, repo string) (*raw.ContentOpts, error) {
	rawOpts := &raw.ContentOpts{}

	if sha := optionalStringArg(args, "sha"); sha != "" {
		rawOpts.SHA = sha
	}

	if branch := optionalStringArg(args, "branch"); branch != "" {
		rawOpts.Ref = "refs/heads/" + branch
	}

	if tag := optionalStringArg(args, "tag"); tag != "" {
		rawOpts.Ref = "refs/tags/" + tag
	}

	prNumber := optionalStringArg(args, "prNumber")
	if prNumber != "" {
		sha, err := resolvePRToSHA(ctx, getClient, owner, repo, prNumber)
		if err != nil {
			return nil, err
		}
		rawOpts.SHA = sha
	}

	return rawOpts, nil
}

func resolvePRToSHA(ctx context.Context, getClient GetClientFn, owner, repo, prNumber string) (string, error) {
	githubClient, err := getClient(ctx)
	if err != nil {
		return "", fmt.Errorf("failed to get GitHub client: %w", err)
	}
	prNum, err := strconv.Atoi(prNumber)
	if err != nil {
		return "", fmt.Errorf("invalid pull request number: %w", err)
	}
	pr, _, err := githubClient.PullRequests.Get(ctx, owner, repo, prNum)
	if err != nil {
		return "", fmt.Errorf("failed to get pull request: %w", err)
	}
	return pr.GetHead().GetSHA(), nil
}

func handleNonOKResponse(resp *http.Response) ([]mcp.ResourceContents, error) {
	if resp.StatusCode == http.StatusNotFound {
		return nil, errors.New("404 Not Found")
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}
	return nil, fmt.Errorf("failed to fetch raw content: %s", string(body))
}

func inferContentType(path string, contentTypeHeader string) string {
	ext := filepath.Ext(path)
	if ext == ".md" {
		return "text/markdown"
	}
	if contentTypeHeader != "" {
		return contentTypeHeader
	}
	return mime.TypeByExtension(ext)
}

func buildResourceContents(uri string, path string, resp *http.Response) ([]mcp.ResourceContents, error) {
	mimeType := inferContentType(path, resp.Header.Get("Content-Type"))

	content, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read file content: %w", err)
	}

	if strings.HasPrefix(mimeType, "text") || strings.HasPrefix(mimeType, "application") {
		return []mcp.ResourceContents{
			mcp.TextResourceContents{
				URI:      uri,
				MIMEType: mimeType,
				Text:     string(content),
			},
		}, nil
	}

	return []mcp.ResourceContents{
		mcp.BlobResourceContents{
			URI:      uri,
			MIMEType: mimeType,
			Blob:     base64.StdEncoding.EncodeToString(content),
		},
	}, nil
}
