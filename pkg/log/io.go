package log

import (
	"io"
	"regexp"

	log "github.com/sirupsen/logrus"
)

// tokenPattern defines a pattern for detecting and redacting sensitive tokens
type tokenPattern struct {
	pattern     *regexp.Regexp
	replacement string
}

// sensitivePatterns contains all patterns for detecting sensitive tokens in log data.
// Using a table-driven approach for maintainability and extensibility.
var sensitivePatterns = []tokenPattern{
	// GitHub Personal Access Tokens (classic and fine-grained): ghp_, gho_, ghu_, ghs_, ghr_
	{regexp.MustCompile(`gh[pousr]_[a-zA-Z0-9]{36}`), "[REDACTED]"},
	// Bearer tokens in various formats
	{regexp.MustCompile(`Bearer\s+[a-zA-Z0-9_\-\.]+`), "Bearer [REDACTED]"},
	// Authorization header values (preserves header structure)
	{regexp.MustCompile(`(?i)(authorization["\s:]+)(Bearer\s+)?[a-zA-Z0-9_\-\.]+`), "${1}[REDACTED]"},
}

// sanitizeLogData redacts sensitive tokens from log data to prevent credential leakage.
// It applies all patterns defined in sensitivePatterns to detect and redact tokens.
func sanitizeLogData(data []byte) []byte {
	sanitized := string(data)
	for _, p := range sensitivePatterns {
		sanitized = p.pattern.ReplaceAllString(sanitized, p.replacement)
	}
	return []byte(sanitized)
}

// IOLogger is a wrapper around io.Reader and io.Writer that can be used
// to log the data being read and written from the underlying streams
type IOLogger struct {
	reader io.Reader
	writer io.Writer
	logger *log.Logger
}

// NewIOLogger creates a new IOLogger instance
func NewIOLogger(r io.Reader, w io.Writer, logger *log.Logger) *IOLogger {
	return &IOLogger{
		reader: r,
		writer: w,
		logger: logger,
	}
}

// Read reads data from the underlying io.Reader and logs it.
// The logged data is sanitized to prevent credential leakage.
func (l *IOLogger) Read(p []byte) (n int, err error) {
	if l.reader == nil {
		return 0, io.EOF
	}
	n, err = l.reader.Read(p)
	if n > 0 {
		sanitized := sanitizeLogData(p[:n])
		l.logger.Infof("[stdin]: received %d bytes: %s", n, string(sanitized))
	}
	return n, err
}

// Write writes data to the underlying io.Writer and logs it.
// The logged data is sanitized to prevent credential leakage.
func (l *IOLogger) Write(p []byte) (n int, err error) {
	if l.writer == nil {
		return 0, io.ErrClosedPipe
	}
	sanitized := sanitizeLogData(p)
	l.logger.Infof("[stdout]: sending %d bytes: %s", len(p), string(sanitized))
	return l.writer.Write(p)
}
