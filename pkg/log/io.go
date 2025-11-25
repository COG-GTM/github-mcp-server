package log

import (
	"io"
	"regexp"

	log "github.com/sirupsen/logrus"
)

var (
	// Token patterns for sanitization
	// GitHub Personal Access Tokens (classic and fine-grained)
	ghpTokenPattern = regexp.MustCompile(`ghp_[a-zA-Z0-9]{36}`)
	ghoTokenPattern = regexp.MustCompile(`gho_[a-zA-Z0-9]{36}`)
	ghuTokenPattern = regexp.MustCompile(`ghu_[a-zA-Z0-9]{36}`)
	ghsTokenPattern = regexp.MustCompile(`ghs_[a-zA-Z0-9]{36}`)
	ghrTokenPattern = regexp.MustCompile(`ghr_[a-zA-Z0-9]{36}`)
	// Bearer tokens in various formats
	bearerTokenPattern = regexp.MustCompile(`Bearer\s+[a-zA-Z0-9_\-\.]+`)
	// Authorization header values
	authHeaderPattern = regexp.MustCompile(`(?i)(authorization["\s:]+)(Bearer\s+)?[a-zA-Z0-9_\-\.]+`)
)

// sanitizeLogData redacts sensitive tokens from log data to prevent credential leakage
func sanitizeLogData(data []byte) []byte {
	sanitized := string(data)

	// Redact GitHub tokens
	sanitized = ghpTokenPattern.ReplaceAllString(sanitized, "[REDACTED]")
	sanitized = ghoTokenPattern.ReplaceAllString(sanitized, "[REDACTED]")
	sanitized = ghuTokenPattern.ReplaceAllString(sanitized, "[REDACTED]")
	sanitized = ghsTokenPattern.ReplaceAllString(sanitized, "[REDACTED]")
	sanitized = ghrTokenPattern.ReplaceAllString(sanitized, "[REDACTED]")

	// Redact Bearer tokens
	sanitized = bearerTokenPattern.ReplaceAllString(sanitized, "Bearer [REDACTED]")

	// Redact Authorization header values while preserving structure
	sanitized = authHeaderPattern.ReplaceAllString(sanitized, "${1}[REDACTED]")

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
