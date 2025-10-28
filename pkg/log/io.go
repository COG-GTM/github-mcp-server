package log

import (
	"io"
	"regexp"

	log "github.com/sirupsen/logrus"
)

// IOLogger is a wrapper around io.Reader and io.Writer that can be used
// to log the data being read and written from the underlying streams
type IOLogger struct {
	reader io.Reader
	writer io.Writer
	logger *log.Logger
}

var (
	githubPatRegex   = regexp.MustCompile(`ghp_[a-zA-Z0-9]{36}`)
	bearerTokenRegex = regexp.MustCompile(`Bearer [a-zA-Z0-9+/=]+`)
	authHeaderRegex  = regexp.MustCompile(`(?i)Authorization:\s*Bearer\s+[a-zA-Z0-9+/=]+`)
)

func sanitizeLogData(data []byte) []byte {
	sanitized := githubPatRegex.ReplaceAll(data, []byte("[REDACTED]"))
	sanitized = bearerTokenRegex.ReplaceAll(sanitized, []byte("Bearer [REDACTED]"))
	sanitized = authHeaderRegex.ReplaceAll(sanitized, []byte("Authorization: Bearer [REDACTED]"))
	return sanitized
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
func (l *IOLogger) Write(p []byte) (n int, err error) {
	if l.writer == nil {
		return 0, io.ErrClosedPipe
	}
	sanitized := sanitizeLogData(p)
	l.logger.Infof("[stdout]: sending %d bytes: %s", len(p), string(sanitized))
	return l.writer.Write(p)
}
