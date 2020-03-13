package log

import (
	"bufio"
	"io"
	"regexp"
	"strings"

	"github.com/gravitational/force"
	"github.com/gravitational/trace"
)

// LineWriter represents line writers
// that process lines by accumulating them
// in case of multi line
type LineWriter interface {
	// WriteLine writes a line of text
	WriteLine(line string) error
}

// Writer returns a writer that ouptuts everything to logger
func Writer0(logger force.Logger) io.WriteCloser {
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
				force.Errorf("Error while reading from writer: %v.", err)
			}
		}
	}()
	return writer
}

const (
	MultilineStateInit         = iota
	MultilineStateAccumulating = iota
)

type MultilineWriter struct {
	lines  []string
	logger force.Logger
	state  int
}

func (m *MultilineWriter) flush() {
	m.logger.Infof(strings.Join(m.lines, "\n"))
	m.logger.Infof("--flush---")
	m.lines = nil
	m.state = MultilineStateInit
}

func (m *MultilineWriter) push(line string) {
	m.lines = append(m.lines, line)
}

func (m *MultilineWriter) WriteLine(line string) error {
	switch m.state {
	case MultilineStateAccumulating:
		m.push(line)
		if len(m.lines) > 100 {
			m.flush()
		}
		if matched, _ := regexp.MatchString(`^\s\s`, line); !matched {
			m.flush()
		}
		return nil
	case MultilineStateInit:
		if matched, _ := regexp.MatchString(`^--- FAIL:\s+(?P<test>[^\(\)]+)\s\((?P<duration>\d\.\d+)s\)`, line); matched {
			m.flush()
			m.state = MultilineStateAccumulating
			m.push(line)
		} else {
			m.push(line)
			m.flush()
		}
		return nil
	default:
		return trace.BadParameter("unsupported state: %v", m.state)
	}
}

//
func Writer(lineWriter LineWriter) io.WriteCloser {
	reader, writer := io.Pipe()
	go func() {
		defer reader.Close()
		scanner := bufio.NewScanner(reader)
		for scanner.Scan() {
			err := lineWriter.WriteLine(scanner.Text())
			if err != nil {
				force.Errorf("Failed to write line: %v.", err)
			}
		}
		if err := scanner.Err(); err != nil {
			if err != io.EOF {
				force.Errorf("Error while reading from writer: %v.", err)
			}
		}
	}()
	return writer
}
