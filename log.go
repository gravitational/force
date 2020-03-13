package force

import (
	"context"
	"fmt"
	"io"
	"os"

	"github.com/gravitational/trace"
)

// Logger is an interface to the logger
type Logger interface {
	// WithError returns a logger bound to an error
	WithError(error) Logger
	Debugf(format string, args ...interface{})
	Infof(format string, args ...interface{})
	Warningf(format string, args ...interface{})
	Errorf(format string, args ...interface{})
	// URL returns a URL for viewing the logs
	// associated with this execution context
	URL(ExecutionContext) string
	// AddFields adds fields to the logger
	AddFields(fields map[string]interface{}) Logger
	// Writer returns writer
	Writer() io.WriteCloser
}

// SetLog adds a logger to the exectuion context
func SetLog(ctx ExecutionContext, log Logger) {
	ctx.SetValue(KeyLog, log)
}

// Log is a helper function that returns log
func Log(ctx context.Context) Logger {
	out := ctx.Value(KeyLog)
	if out == nil {
		// this is a fallback so we can avoid loosing logs
		return &wrapper{}
	}
	l, ok := out.(Logger)
	if !ok {
		// this is a fallback so we can avoid loosing logs
		fmt.Printf("Unsupported log type in the context %v.", out)
		return &wrapper{}
	}
	return l
}

type wrapper struct {
}

func (w *wrapper) URL(ExecutionContext) string {
	return ""
}

// AddFields adds fields to the logger
func (w *wrapper) AddFields(fields map[string]interface{}) Logger {
	return &wrapper{}
}

func (w *wrapper) WithError(err error) Logger {
	return w
}

type NopWriteCloser struct {
	io.Writer
}

func (n *NopWriteCloser) Close() error {
	return nil
}

func (w *wrapper) Writer() io.WriteCloser {
	return &NopWriteCloser{Writer: os.Stdout}
}

func (w *wrapper) Debugf(format string, args ...interface{}) {
	fmt.Sprintf("DEBU %v %v\n", format, args)
}
func (w *wrapper) Infof(format string, args ...interface{}) {
	fmt.Sprintf("INFO %v %v\n", format, args)
}

func (w *wrapper) Warningf(format string, args ...interface{}) {
	fmt.Sprintf("WARN %v %v\n", format, args)
}

func (w *wrapper) Errorf(format string, args ...interface{}) {
	fmt.Sprintf("ERRO %v %v\n", format, args)
}

var defaultWrapper = &wrapper{}

func Debugf(format string, args ...interface{}) {
	defaultWrapper.Debugf(format, args)
}
func Infof(format string, args ...interface{}) {
	defaultWrapper.Debugf(format, args)
}

func Warningf(format string, args ...interface{}) {
	defaultWrapper.Warningf(format, args)
}

func Errorf(format string, args ...interface{}) {
	defaultWrapper.Errorf(format, args)
}

// NewMultiLogger creates a new instance of logger
// that outputs to multiple loggers
func NewMultiLogger(loggers ...Logger) Logger {
	return &MultiLogger{
		loggers: loggers,
	}
}

// NewMultiWriteCloser
func NewMultiWriteCloser(writeClosers ...io.WriteCloser) io.WriteCloser {
	writers := make([]io.Writer, len(writeClosers))
	for i := range writeClosers {
		writers[i] = writeClosers[i]
	}
	return &MultiWriteCloser{
		writeClosers: writeClosers,
		Writer:       io.MultiWriter(writers...),
	}
}

type MultiWriteCloser struct {
	writeClosers []io.WriteCloser
	io.Writer
}

func (m *MultiWriteCloser) Close() error {
	var errors []error
	for _, c := range m.writeClosers {
		err := c.Close()
		if err != nil {
			errors = append(errors, err)
		}
	}
	return trace.NewAggregate(errors...)
}

// MultiLogger iterates over multiple loggers
type MultiLogger struct {
	loggers []Logger
}

func (m *MultiLogger) Writer() io.WriteCloser {
	writeClosers := make([]io.WriteCloser, len(m.loggers))
	for i := range m.loggers {
		writeClosers[i] = m.loggers[i].Writer()
	}
	return NewMultiWriteCloser(writeClosers...)
}

// WithError returns a logger bound to an error
func (m *MultiLogger) WithError(err error) Logger {
	out := &MultiLogger{
		loggers: make([]Logger, len(m.loggers)),
	}
	for i := range m.loggers {
		out.loggers[i] = m.loggers[i].WithError(err)
	}
	return out
}

func (m *MultiLogger) Debugf(format string, args ...interface{}) {
	for _, l := range m.loggers {
		l.Debugf(format, args...)
	}
}

func (m *MultiLogger) Infof(format string, args ...interface{}) {
	for _, l := range m.loggers {
		l.Infof(format, args...)
	}
}

func (m *MultiLogger) Warningf(format string, args ...interface{}) {
	for _, l := range m.loggers {
		l.Warningf(format, args...)
	}
}

func (m *MultiLogger) Errorf(format string, args ...interface{}) {
	for _, l := range m.loggers {
		l.Errorf(format, args...)
	}
}

// URL returns a URL for viewing the logs
// associated with this execution context
func (m *MultiLogger) URL(ctx ExecutionContext) string {
	for _, l := range m.loggers {
		out := l.URL(ctx)
		if out != "" {
			return out
		}
	}
	return ""
}

// AddFields adds fields to the logger
func (m *MultiLogger) AddFields(fields map[string]interface{}) Logger {
	out := &MultiLogger{
		loggers: make([]Logger, len(m.loggers)),
	}
	for i := range m.loggers {
		out.loggers[i] = m.loggers[i].AddFields(fields)
	}
	return out
}
