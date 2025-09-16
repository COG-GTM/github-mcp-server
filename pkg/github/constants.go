package github

import (
	"encoding/json"
	"fmt"
	"io"

	"github.com/google/go-github/v72/github"
	"github.com/mark3labs/mcp-go/mcp"
)

const (
	ErrFailedToReadResponseBody = "failed to read response body: %w"
	ErrFailedToMarshalResponse  = "failed to marshal response: %w"
)

func HandleHTTPError(resp *github.Response, operation string) (*mcp.CallToolResult, error) {
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf(ErrFailedToReadResponseBody, err)
	}
	return mcp.NewToolResultError(fmt.Sprintf("%s: %s", operation, string(body))), nil
}

func MarshalJSONResponse(data interface{}) (*mcp.CallToolResult, error) {
	r, err := json.Marshal(data)
	if err != nil {
		return nil, fmt.Errorf(ErrFailedToMarshalResponse, err)
	}
	return mcp.NewToolResultText(string(r)), nil
}
