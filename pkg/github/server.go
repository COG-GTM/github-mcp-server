package github

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"

	ghErrors "github.com/github/github-mcp-server/pkg/errors"
	"github.com/github/github-mcp-server/pkg/translations"
	"github.com/google/go-github/v72/github"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

// NewServer creates a new GitHub MCP server with the specified GH client and logger.

func NewServer(version string, opts ...server.ServerOption) *server.MCPServer {
	// Add default options
	defaultOpts := []server.ServerOption{
		server.WithToolCapabilities(true),
		server.WithResourceCapabilities(true, true),
		server.WithLogging(),
	}
	opts = append(defaultOpts, opts...)

	// Create a new MCP server
	s := server.NewMCPServer(
		"github-mcp-server",
		version,
		opts...,
	)
	return s
}

// OptionalParamOK is a helper function that can be used to fetch a requested parameter from the request.
// It returns the value, a boolean indicating if the parameter was present, and an error if the type is wrong.
func OptionalParamOK[T any](r mcp.CallToolRequest, p string) (value T, ok bool, err error) {
	// Check if the parameter is present in the request
	val, exists := r.GetArguments()[p]
	if !exists {
		// Not present, return zero value, false, no error
		return
	}

	// Check if the parameter is of the expected type
	value, ok = val.(T)
	if !ok {
		// Present but wrong type
		err = fmt.Errorf("parameter %s is not of type %T, is %T", p, value, val)
		ok = true // Set ok to true because the parameter *was* present, even if wrong type
		return
	}

	// Present and correct type
	ok = true
	return
}

// isAcceptedError checks if the error is an accepted error.
func isAcceptedError(err error) bool {
	var acceptedError *github.AcceptedError
	return errors.As(err, &acceptedError)
}

// RequiredParam is a helper function that can be used to fetch a requested parameter from the request.
// It does the following checks:
// 1. Checks if the parameter is present in the request.
// 2. Checks if the parameter is of the expected type.
// 3. Checks if the parameter is not empty, i.e: non-zero value
func RequiredParam[T comparable](r mcp.CallToolRequest, p string) (T, error) {
	var zero T

	// Check if the parameter is present in the request
	if _, ok := r.GetArguments()[p]; !ok {
		return zero, fmt.Errorf("missing required parameter: %s", p)
	}

	// Check if the parameter is of the expected type
	val, ok := r.GetArguments()[p].(T)
	if !ok {
		return zero, fmt.Errorf("parameter %s is not of type %T", p, zero)
	}

	if val == zero {
		return zero, fmt.Errorf("missing required parameter: %s", p)
	}

	return val, nil
}

// RequiredInt is a helper function that can be used to fetch a requested parameter from the request.
// It does the following checks:
// 1. Checks if the parameter is present in the request.
// 2. Checks if the parameter is of the expected type.
// 3. Checks if the parameter is not empty, i.e: non-zero value
func RequiredInt(r mcp.CallToolRequest, p string) (int, error) {
	v, err := RequiredParam[float64](r, p)
	if err != nil {
		return 0, err
	}
	return int(v), nil
}

// OptionalParam is a helper function that can be used to fetch a requested parameter from the request.
// It does the following checks:
// 1. Checks if the parameter is present in the request, if not, it returns its zero-value
// 2. If it is present, it checks if the parameter is of the expected type and returns it
func OptionalParam[T any](r mcp.CallToolRequest, p string) (T, error) {
	var zero T

	// Check if the parameter is present in the request
	if _, ok := r.GetArguments()[p]; !ok {
		return zero, nil
	}

	// Check if the parameter is of the expected type
	if _, ok := r.GetArguments()[p].(T); !ok {
		return zero, fmt.Errorf("parameter %s is not of type %T, is %T", p, zero, r.GetArguments()[p])
	}

	return r.GetArguments()[p].(T), nil
}

// OptionalIntParam is a helper function that can be used to fetch a requested parameter from the request.
// It does the following checks:
// 1. Checks if the parameter is present in the request, if not, it returns its zero-value
// 2. If it is present, it checks if the parameter is of the expected type and returns it
func OptionalIntParam(r mcp.CallToolRequest, p string) (int, error) {
	v, err := OptionalParam[float64](r, p)
	if err != nil {
		return 0, err
	}
	return int(v), nil
}

// OptionalIntParamWithDefault is a helper function that can be used to fetch a requested parameter from the request
// similar to optionalIntParam, but it also takes a default value.
func OptionalIntParamWithDefault(r mcp.CallToolRequest, p string, d int) (int, error) {
	v, err := OptionalIntParam(r, p)
	if err != nil {
		return 0, err
	}
	if v == 0 {
		return d, nil
	}
	return v, nil
}

// OptionalStringArrayParam is a helper function that can be used to fetch a requested parameter from the request.
// It does the following checks:
// 1. Checks if the parameter is present in the request, if not, it returns its zero-value
// 2. If it is present, iterates the elements and checks each is a string
func OptionalStringArrayParam(r mcp.CallToolRequest, p string) ([]string, error) {
	// Check if the parameter is present in the request
	if _, ok := r.GetArguments()[p]; !ok {
		return []string{}, nil
	}

	switch v := r.GetArguments()[p].(type) {
	case nil:
		return []string{}, nil
	case []string:
		return v, nil
	case []any:
		strSlice := make([]string, len(v))
		for i, v := range v {
			s, ok := v.(string)
			if !ok {
				return []string{}, fmt.Errorf("parameter %s is not of type string, is %T", p, v)
			}
			strSlice[i] = s
		}
		return strSlice, nil
	default:
		return []string{}, fmt.Errorf("parameter %s could not be coerced to []string, is %T", p, r.GetArguments()[p])
	}
}

// WithPagination returns a ToolOption that adds "page" and "perPage" parameters to the tool.
// The "page" parameter is optional, min 1. The "perPage" parameter is optional, min 1, max 100.
func WithPagination() mcp.ToolOption {
	return func(tool *mcp.Tool) {
		mcp.WithNumber("page",
			mcp.Description("Page number for pagination (min 1)"),
			mcp.Min(1),
		)(tool)

		mcp.WithNumber("perPage",
			mcp.Description("Results per page for pagination (min 1, max 100)"),
			mcp.Min(1),
			mcp.Max(100),
		)(tool)
	}
}

type PaginationParams struct {
	page    int
	perPage int
}

// OptionalPaginationParams returns the "page" and "perPage" parameters from the request,
// or their default values if not present, "page" default is 1, "perPage" default is 30.
// In future, we may want to make the default values configurable, or even have this
// function returned from `withPagination`, where the defaults are provided alongside
// the min/max values.
func OptionalPaginationParams(r mcp.CallToolRequest) (PaginationParams, error) {
	page, err := OptionalIntParamWithDefault(r, "page", 1)
	if err != nil {
		return PaginationParams{}, err
	}
	perPage, err := OptionalIntParamWithDefault(r, "perPage", 30)
	if err != nil {
		return PaginationParams{}, err
	}
	return PaginationParams{
		page:    page,
		perPage: perPage,
	}, nil
}

func MarshalledTextResult(v any) *mcp.CallToolResult {
	data, err := json.Marshal(v)
	if err != nil {
		return mcp.NewToolResultErrorFromErr("failed to marshal text result to json", err)
	}

	return mcp.NewToolResultText(string(data))
}

type ToolConfig struct {
	Name        string
	Description string
	Title       string
	ReadOnly    bool
	Parameters  []ParameterConfig
	Handler     ToolHandlerConfig
}

type ParameterConfig struct {
	Name        string
	Type        string
	Required    bool
	Description string
	Enum        []string
}

type ToolHandlerConfig struct {
	APICall           func(ctx context.Context, client *github.Client, params map[string]interface{}) (interface{}, *github.Response, error)
	ErrorPrefix       string
	ReturnGoErrors    bool // If true, return (nil, error) instead of (*mcp.CallToolResult, nil) for API errors
}

func CreateGitHubTool(getClient GetClientFn, _ translations.TranslationHelperFunc, config ToolConfig) (mcp.Tool, server.ToolHandlerFunc) {
	options := []mcp.ToolOption{
		mcp.WithDescription(config.Description),
		mcp.WithToolAnnotation(mcp.ToolAnnotation{
			Title:        config.Title,
			ReadOnlyHint: ToBoolPtr(config.ReadOnly),
		}),
	}

	for _, param := range config.Parameters {
		switch param.Type {
		case "string":
			paramOpts := []mcp.PropertyOption{mcp.Description(param.Description)}
			if param.Required {
				paramOpts = append(paramOpts, mcp.Required())
			}
			if len(param.Enum) > 0 {
				paramOpts = append(paramOpts, mcp.Enum(param.Enum...))
			}
			options = append(options, mcp.WithString(param.Name, paramOpts...))
		case "number":
			paramOpts := []mcp.PropertyOption{mcp.Description(param.Description)}
			if param.Required {
				paramOpts = append(paramOpts, mcp.Required())
			}
			options = append(options, mcp.WithNumber(param.Name, paramOpts...))
		case "boolean":
			paramOpts := []mcp.PropertyOption{mcp.Description(param.Description)}
			if param.Required {
				paramOpts = append(paramOpts, mcp.Required())
			}
			options = append(options, mcp.WithBoolean(param.Name, paramOpts...))
		}
	}

	if needsPagination(config.Name) {
		options = append(options, WithPagination())
	}

	handler := func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		params := make(map[string]interface{})
		for _, paramConfig := range config.Parameters {
			if paramConfig.Required {
				switch paramConfig.Type {
				case "string":
					value, err := RequiredParam[string](request, paramConfig.Name)
					if err != nil {
						return mcp.NewToolResultError(err.Error()), nil
					}
					params[paramConfig.Name] = value
				case "number":
					value, err := RequiredInt(request, paramConfig.Name)
					if err != nil {
						return mcp.NewToolResultError(err.Error()), nil
					}
					params[paramConfig.Name] = value
				case "boolean":
					value, err := RequiredParam[bool](request, paramConfig.Name)
					if err != nil {
						return mcp.NewToolResultError(err.Error()), nil
					}
					params[paramConfig.Name] = value
				}
			} else {
				switch paramConfig.Type {
				case "string":
					value, err := OptionalParam[string](request, paramConfig.Name)
					if err != nil {
						return mcp.NewToolResultError(err.Error()), nil
					}
					if value != "" {
						params[paramConfig.Name] = value
					}
				case "number":
					value, err := OptionalIntParam(request, paramConfig.Name)
					if err != nil {
						return mcp.NewToolResultError(err.Error()), nil
					}
					if value != 0 {
						params[paramConfig.Name] = value
					}
				case "boolean":
					value, err := OptionalParam[bool](request, paramConfig.Name)
					if err != nil {
						return mcp.NewToolResultError(err.Error()), nil
					}
					params[paramConfig.Name] = value
				}
			}
		}

		if needsPagination(config.Name) {
			if err := extractPaginationFromParams(request, params); err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
		}

		client, err := getClient(ctx)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("failed to get GitHub client: %v", err)), nil
		}

		result, resp, err := config.Handler.APICall(ctx, client, params)
		if err != nil {
			if config.Handler.ReturnGoErrors {
				return nil, fmt.Errorf("%s: %w", config.Handler.ErrorPrefix, err)
			}
			return ghErrors.NewGitHubAPIErrorResponse(ctx, config.Handler.ErrorPrefix, resp, err), nil
		}
		defer func() { _ = resp.Body.Close() }()

		if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
			body, err := io.ReadAll(resp.Body)
			if err != nil {
				return nil, fmt.Errorf(ErrFailedToReadResponseBody, err)
			}
			return mcp.NewToolResultError(fmt.Sprintf("%s: %s", config.Handler.ErrorPrefix, string(body))), nil
		}

		r, err := json.Marshal(result)
		if err != nil {
			return nil, fmt.Errorf(ErrFailedToMarshalResponse, err)
		}
		return mcp.NewToolResultText(string(r)), nil
	}

	return mcp.NewTool(config.Name, options...), handler
}

func needsPagination(toolName string) bool {
	paginatedTools := map[string]bool{
		"search_issues":               true,
		"list_issues":                 true,
		"list_commits":                true,
		"list_releases":               true,
		"list_tags":                   true,
		"search_repositories":         true,
		"search_users":                true,
		"search_organizations":        true,
		"list_notifications":          true,
		"list_pull_requests":          true,
		"search_pull_requests":        true,
		"list_pull_request_comments":  true,
	}
	return paginatedTools[toolName]
}

func extractPaginationFromParams(request mcp.CallToolRequest, params map[string]interface{}) error {
	paginationParams, err := OptionalPaginationParams(request)
	if err != nil {
		return err
	}
	if paginationParams.page > 0 {
		params["page"] = paginationParams.page
	}
	if paginationParams.perPage > 0 {
		params["perPage"] = paginationParams.perPage
	}
	return nil
}
