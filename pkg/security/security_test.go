package security

import (
	"testing"
)

func TestRedactBearerToken(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "simple bearer token",
			input:    "Bearer ghp_abc123xyz",
			expected: "Bearer ****",
		},
		{
			name:     "bearer token in header",
			input:    `Authorization: Bearer ghp_abc123xyz`,
			expected: `Authorization: Bearer ****`,
		},
		{
			name:     "bearer token case insensitive",
			input:    "BEARER token123",
			expected: "BEARER ****",
		},
		{
			name:     "no bearer token",
			input:    "This is a normal string",
			expected: "This is a normal string",
		},
		{
			name:     "bearer token in JSON",
			input:    `{"Authorization": "Bearer ghp_secret123"}`,
			expected: `{"Authorization": "Bearer ****"}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := RedactBearerToken(tt.input)
			if result != tt.expected {
				t.Errorf("RedactBearerToken(%q) = %q, want %q", tt.input, result, tt.expected)
			}
		})
	}
}

func TestSanitizeURL(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "URL with token query param",
			input:    "https://api.github.com/logs?token=secret123&other=value",
			expected: "https://api.github.com/logs",
		},
		{
			name:     "URL without query params",
			input:    "https://api.github.com/repos/owner/repo",
			expected: "https://api.github.com/repos/owner/repo",
		},
		{
			name:     "URL with fragment",
			input:    "https://api.github.com/logs#section",
			expected: "https://api.github.com/logs",
		},
		{
			name:     "URL with query and fragment",
			input:    "https://api.github.com/logs?token=secret#section",
			expected: "https://api.github.com/logs",
		},
		{
			name:     "invalid URL",
			input:    "://invalid",
			expected: "[URL redacted due to parsing error]",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := SanitizeURL(tt.input)
			if result != tt.expected {
				t.Errorf("SanitizeURL(%q) = %q, want %q", tt.input, result, tt.expected)
			}
		})
	}
}

func TestContainsSensitiveData(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected bool
	}{
		{
			name:     "contains bearer token",
			input:    "Bearer ghp_secret123",
			expected: true,
		},
		{
			name:     "contains token query param",
			input:    "https://api.github.com?token=secret",
			expected: true,
		},
		{
			name:     "no sensitive data",
			input:    "This is a normal string",
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ContainsSensitiveData(tt.input)
			if result != tt.expected {
				t.Errorf("ContainsSensitiveData(%q) = %v, want %v", tt.input, result, tt.expected)
			}
		})
	}
}

func TestRedactSensitiveData(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "bearer token",
			input:    "Authorization: Bearer ghp_secret123",
			expected: "Authorization: Bearer ****",
		},
		{
			name:     "no sensitive data",
			input:    "Normal text without secrets",
			expected: "Normal text without secrets",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := RedactSensitiveData(tt.input)
			if result != tt.expected {
				t.Errorf("RedactSensitiveData(%q) = %q, want %q", tt.input, result, tt.expected)
			}
		})
	}
}
