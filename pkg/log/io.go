package log

import (
	"io"
	"regexp"

	log "github.com/sirupsen/logrus"
)

// Regular expressions for detecting sensitive data patterns
var (
	// Matches "Bearer <token>" patterns in various contexts
	bearerTokenPattern = regexp.MustCompile(`(?i)(Bearer\s+)[^\s"'\]},]+`)
	// Matches "authorization_token":"<value>" or 'authorization_token':'<value>' patterns in JSON
	authTokenJSONPattern = regexp.MustCompile(`(?i)("authorization_token"\s*:\s*")[^"]*(")|('authorization_token'\s*:\s*')[^']*(')"`)
)

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

// sanitizeData redacts sensitive information from data before logging.
// It replaces Bearer tokens and authorization_token values with masked versions
// to prevent credential leakage in log files.
func sanitizeData(data []byte) []byte {
	sanitized := string(data)

	// Redact Bearer tokens: "Bearer <token>" -> "Bearer ****"
	sanitized = bearerTokenPattern.ReplaceAllString(sanitized, "${1}****")

	// Redact authorization_token values in JSON
	sanitized = authTokenJSONPattern.ReplaceAllString(sanitized, "${1}****${2}${3}****${4}")

	return []byte(sanitized)
}

// Read reads data from the underlying io.Reader and logs it.
// Sensitive data such as Bearer tokens and authorization_token values are redacted before logging.
func (l *IOLogger) Read(p []byte) (n int, err error) {
	if l.reader == nil {
		return 0, io.EOF
	}
	n, err = l.reader.Read(p)
	if n > 0 {
		// Sanitize data before logging to prevent credential leakage
		sanitized := sanitizeData(p[:n])
		l.logger.Infof("[stdin]: received %d bytes: %s", n, string(sanitized))
	}
	return n, err
}

// Write writes data to the underlying io.Writer and logs it.
// Sensitive data such as Bearer tokens and authorization_token values are redacted before logging.
func (l *IOLogger) Write(p []byte) (n int, err error) {
	if l.writer == nil {
		return 0, io.ErrClosedPipe
	}
	// Sanitize data before logging to prevent credential leakage
	sanitized := sanitizeData(p)
	l.logger.Infof("[stdout]: sending %d bytes: %s", len(p), string(sanitized))
	return l.writer.Write(p)
}
