package force

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"strings"
)

// Logger is an interface to the logger
type Logger interface {
	Debugf(format string, args ...interface{})
	Infof(format string, args ...interface{})
	Warningf(format string, args ...interface{})
	Errorf(format string, args ...interface{})
	// URL returns a URL for viewing the logs
	// associated with this execution context
	URL(ExecutionContext) string
	// AddFields adds fields to the logger
	AddFields(fields map[string]interface{}) Logger
}

// WithLog adds a logger to the exectuion context
func WithLog(ctx ExecutionContext, log Logger) ExecutionContext {
	return ctx.WithValue(KeyLog, log)
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

// Writer returns a writer that ouptuts everything to logger
func Writer(logger Logger) io.WriteCloser {
	reader, writer := io.Pipe()
	go func() {
		defer reader.Close()
		scanner := bufio.NewScanner(reader)
		for scanner.Scan() {
			text := strings.TrimSpace(scanner.Text())
			if text != "" {
				logger.Infof("%v", text)
			}
		}
		if err := scanner.Err(); err != nil {
			if err != io.EOF {
				Errorf("Error while reading from writer: %v.", err)
			}
		}
	}()

	return writer
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
