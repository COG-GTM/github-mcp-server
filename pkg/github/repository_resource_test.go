package github

import (
	"context"
	"net/http"
	"net/url"
	"testing"

	"github.com/github/github-mcp-server/pkg/raw"
	"github.com/github/github-mcp-server/pkg/translations"
	"github.com/google/go-github/v72/github"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/migueleliasweb/go-github-mock/src/mock"
	"github.com/stretchr/testify/require"
)

const (
	testContentType     = "Content-Type"
	testMIMEImagePNG    = "image/png"
	testMIMETextMD      = "text/markdown"
	testREADMEPath      = "README.md"
	testRepoContent     = "# Test Repository\n\nThis is a test repository."
)

func Test_repositoryResourceContentsHandler(t *testing.T) {
	base, _ := url.Parse("https://raw.example.com/")
	tests := []struct {
		name           string
		mockedClient   *http.Client
		requestArgs    map[string]any
		expectError    string
		expectedResult any
	}{
		{
			name: "missing owner",
			mockedClient: mock.NewMockedHTTPClient(
				mock.WithRequestMatchHandler(
					raw.GetRawReposContentsByOwnerByRepoByPath,
					http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
						w.Header().Set(testContentType, testMIMEImagePNG)
						// as this is given as a png, it will return the content as a blob
						_, err := w.Write([]byte(testRepoContent))
						require.NoError(t, err)
					}),
				),
			),
			requestArgs: map[string]any{},
			expectError: "owner is required",
		},
		{
			name: "missing repo",
			mockedClient: mock.NewMockedHTTPClient(
				mock.WithRequestMatchHandler(
					raw.GetRawReposContentsByOwnerByRepoByBranchByPath,
					http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
						w.Header().Set(testContentType, testMIMEImagePNG)
						// as this is given as a png, it will return the content as a blob
						_, err := w.Write([]byte(testRepoContent))
						require.NoError(t, err)
					}),
				),
			),
			requestArgs: map[string]any{
				"owner": []string{"owner"},
			},
			expectError: "repo is required",
		},
		{
			name: "successful blob content fetch",
			mockedClient: mock.NewMockedHTTPClient(
				mock.WithRequestMatchHandler(
					raw.GetRawReposContentsByOwnerByRepoByPath,
					http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
						w.Header().Set(testContentType, testMIMEImagePNG)
						_, err := w.Write([]byte(testRepoContent))
						require.NoError(t, err)
					}),
				),
			),
			requestArgs: map[string]any{
				"owner": []string{"owner"},
				"repo":  []string{"repo"},
				"path":  []string{"data.png"},
			},
			expectedResult: []mcp.BlobResourceContents{{
				Blob:     "IyBUZXN0IFJlcG9zaXRvcnkKClRoaXMgaXMgYSB0ZXN0IHJlcG9zaXRvcnku",
				MIMEType: testMIMEImagePNG,
				URI:      "",
			}},
		},
		{
			name: "successful text content fetch (HEAD)",
			mockedClient: mock.NewMockedHTTPClient(
				mock.WithRequestMatchHandler(
					raw.GetRawReposContentsByOwnerByRepoByPath,
					http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
						w.Header().Set(testContentType, testMIMETextMD)
						_, err := w.Write([]byte(testRepoContent))
						require.NoError(t, err)
					}),
				),
			),
			requestArgs: map[string]any{
				"owner": []string{"owner"},
				"repo":  []string{"repo"},
				"path":  []string{testREADMEPath},
			},
			expectedResult: []mcp.TextResourceContents{{
				Text:     testRepoContent,
				MIMEType: testMIMETextMD,
				URI:      "",
			}},
		},
		{
			name: "successful text content fetch (branch)",
			mockedClient: mock.NewMockedHTTPClient(
				mock.WithRequestMatchHandler(
					raw.GetRawReposContentsByOwnerByRepoByBranchByPath,
					http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
						w.Header().Set(testContentType, testMIMETextMD)
						_, err := w.Write([]byte(testRepoContent))
						require.NoError(t, err)
					}),
				),
			),
			requestArgs: map[string]any{
				"owner":  []string{"owner"},
				"repo":   []string{"repo"},
				"path":   []string{testREADMEPath},
				"branch": []string{"main"},
			},
			expectedResult: []mcp.TextResourceContents{{
				Text:     testRepoContent,
				MIMEType: testMIMETextMD,
				URI:      "",
			}},
		},
		{
			name: "successful text content fetch (tag)",
			mockedClient: mock.NewMockedHTTPClient(
				mock.WithRequestMatchHandler(
					raw.GetRawReposContentsByOwnerByRepoByTagByPath,
					http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
						w.Header().Set(testContentType, testMIMETextMD)
						_, err := w.Write([]byte(testRepoContent))
						require.NoError(t, err)
					}),
				),
			),
			requestArgs: map[string]any{
				"owner": []string{"owner"},
				"repo":  []string{"repo"},
				"path":  []string{testREADMEPath},
				"tag":   []string{"v1.0.0"},
			},
			expectedResult: []mcp.TextResourceContents{{
				Text:     testRepoContent,
				MIMEType: testMIMETextMD,
				URI:      "",
			}},
		},
		{
			name: "successful text content fetch (sha)",
			mockedClient: mock.NewMockedHTTPClient(
				mock.WithRequestMatchHandler(
					raw.GetRawReposContentsByOwnerByRepoBySHAByPath,
					http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
						w.Header().Set(testContentType, testMIMETextMD)
						_, err := w.Write([]byte(testRepoContent))
						require.NoError(t, err)
					}),
				),
			),
			requestArgs: map[string]any{
				"owner": []string{"owner"},
				"repo":  []string{"repo"},
				"path":  []string{testREADMEPath},
				"sha":   []string{"abc123"},
			},
			expectedResult: []mcp.TextResourceContents{{
				Text:     testRepoContent,
				MIMEType: testMIMETextMD,
				URI:      "",
			}},
		},
		{
			name: "successful text content fetch (pr)",
			mockedClient: mock.NewMockedHTTPClient(
				mock.WithRequestMatchHandler(
					mock.GetReposPullsByOwnerByRepoByPullNumber,
					http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
						w.Header().Set(testContentType, "application/json")
						_, err := w.Write([]byte(`{"head": {"sha": "abc123"}}`))
						require.NoError(t, err)
					}),
				),
				mock.WithRequestMatchHandler(
					raw.GetRawReposContentsByOwnerByRepoBySHAByPath,
					http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
						w.Header().Set(testContentType, testMIMETextMD)
						_, err := w.Write([]byte(testRepoContent))
						require.NoError(t, err)
					}),
				),
			),
			requestArgs: map[string]any{
				"owner":    []string{"owner"},
				"repo":     []string{"repo"},
				"path":     []string{testREADMEPath},
				"prNumber": []string{"42"},
			},
			expectedResult: []mcp.TextResourceContents{{
				Text:     testRepoContent,
				MIMEType: testMIMETextMD,
				URI:      "",
			}},
		},
		{
			name: "content fetch fails",
			mockedClient: mock.NewMockedHTTPClient(
				mock.WithRequestMatchHandler(
					mock.GetReposContentsByOwnerByRepoByPath,
					http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
						w.WriteHeader(http.StatusNotFound)
						_, _ = w.Write([]byte(`{"message": "Not Found"}`))
					}),
				),
			),
			requestArgs: map[string]any{
				"owner":  []string{"owner"},
				"repo":   []string{"repo"},
				"path":   []string{"nonexistent.md"},
				"branch": []string{"main"},
			},
			expectError: "404 Not Found",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			client := github.NewClient(tc.mockedClient)
			mockRawClient := raw.NewClient(client, base)
			handler := RepositoryResourceContentsHandler((stubGetClientFn(client)), stubGetRawClientFn(mockRawClient))

			request := mcp.ReadResourceRequest{
				Params: struct {
					URI       string         `json:"uri"`
					Arguments map[string]any `json:"arguments,omitempty"`
				}{
					Arguments: tc.requestArgs,
				},
			}

			resp, err := handler(context.TODO(), request)

			if tc.expectError != "" {
				require.ErrorContains(t, err, tc.expectError)
				return
			}

			require.NoError(t, err)
			require.ElementsMatch(t, resp, tc.expectedResult)
		})
	}
}

func Test_GetRepositoryResourceContent(t *testing.T) {
	mockRawClient := raw.NewClient(github.NewClient(nil), &url.URL{})
	tmpl, _ := GetRepositoryResourceContent(nil, stubGetRawClientFn(mockRawClient), translations.NullTranslationHelper)
	require.Equal(t, "repo://{owner}/{repo}/contents{/path*}", tmpl.URITemplate.Raw())
}

func Test_GetRepositoryResourceBranchContent(t *testing.T) {
	mockRawClient := raw.NewClient(github.NewClient(nil), &url.URL{})
	tmpl, _ := GetRepositoryResourceBranchContent(nil, stubGetRawClientFn(mockRawClient), translations.NullTranslationHelper)
	require.Equal(t, "repo://{owner}/{repo}/refs/heads/{branch}/contents{/path*}", tmpl.URITemplate.Raw())
}
func Test_GetRepositoryResourceCommitContent(t *testing.T) {
	mockRawClient := raw.NewClient(github.NewClient(nil), &url.URL{})
	tmpl, _ := GetRepositoryResourceCommitContent(nil, stubGetRawClientFn(mockRawClient), translations.NullTranslationHelper)
	require.Equal(t, "repo://{owner}/{repo}/sha/{sha}/contents{/path*}", tmpl.URITemplate.Raw())
}

func Test_GetRepositoryResourceTagContent(t *testing.T) {
	mockRawClient := raw.NewClient(github.NewClient(nil), &url.URL{})
	tmpl, _ := GetRepositoryResourceTagContent(nil, stubGetRawClientFn(mockRawClient), translations.NullTranslationHelper)
	require.Equal(t, "repo://{owner}/{repo}/refs/tags/{tag}/contents{/path*}", tmpl.URITemplate.Raw())
}
