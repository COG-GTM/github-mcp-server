package log

import (
	"io"
	"regexp"

	log "github.com/sirupsen/logrus"
)

var (
	bearerTokenPattern        = regexp.MustCompile(`Bearer\s+[A-Za-z0-9_\-\.]+`)
	authorizationTokenPattern = regexp.MustCompile(`("authorization_token"\s*:\s*")([^"]+)(")`)
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

func (l *IOLogger) sanitizeData(data []byte) []byte {
	sanitized := bearerTokenPattern.ReplaceAll(data, []byte("Bearer ****"))
	sanitized = authorizationTokenPattern.ReplaceAll(sanitized, []byte(`$1****$3`))
	return sanitized
}

// Read reads data from the underlying io.Reader and logs it.
func (l *IOLogger) Read(p []byte) (n int, err error) {
	if l.reader == nil {
		return 0, io.EOF
	}
	n, err = l.reader.Read(p)
	if n > 0 {
		sanitized := l.sanitizeData(p[:n])
		l.logger.Infof("[stdin]: received %d bytes: %s", n, string(sanitized))
	}
	return n, err
}

// Write writes data to the underlying io.Writer and logs it.
func (l *IOLogger) Write(p []byte) (n int, err error) {
	if l.writer == nil {
		return 0, io.ErrClosedPipe
	}
	sanitized := l.sanitizeData(p)
	l.logger.Infof("[stdout]: sending %d bytes: %s", len(p), string(sanitized))
	return l.writer.Write(p)
}
