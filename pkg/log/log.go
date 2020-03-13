package log

import (
	"fmt"
	"io"
	"io/ioutil"
	"net/url"
	"os"

	"github.com/gravitational/force"
	"github.com/gravitational/force/pkg/log/stack"

	"github.com/gravitational/trace"
	log "github.com/sirupsen/logrus"
)

//Namespace is a wrapper around string to namespace a variable in the context
type Namespace string

// Key is a name of the plugin variable
const Key = Namespace("log")

const (
	KeySetup        = "Setup"
	KeyConfig       = "Config"
	TypeStackdriver = "stackdriver"
	TypeStdout      = "stdout"
)

// Output is a logging output
type Output struct {
	// Type is a logging type, currently supported
	// is stackdriver and stdout
	Type string
	// CredentialsFile is a path to credentials file,
	// used in case of stackdriver plugin
	CredentialsFile string
	// Credentials is a string with creds
	Credentials string
}

// Config is a log configuration
type Config struct {
	// Level is a debugging level
	Level string
	// Outputs is a list of logging outputs
	Outputs []Output
}

// CheckAndSetDefaults checks and sets default values
func (cfg *Config) CheckAndSetDefaults() error {
	if cfg.Level == "" {
		cfg.Level = log.InfoLevel.String()
	} else {
		_, err := log.ParseLevel(cfg.Level)
		if err != nil {
			return trace.Wrap(err)
		}
	}
	for i, o := range cfg.Outputs {
		switch o.Type {
		case TypeStackdriver:
			if o.Credentials == "" && o.CredentialsFile == "" {
				return trace.BadParameter(
					"provide Credentials or CredentialsFile in LoggingConfig when using %q logging,"+
						" read https://cloud.google.com/logging/docs/agent/authorization for more details.", o.Type)
			}
			if o.Credentials != "" && o.CredentialsFile != "" {
				return trace.BadParameter("provide either Credentials or CredentialsFile, not both for %q logger", o.Type)
			}
			if o.CredentialsFile != "" {
				data, err := ioutil.ReadFile(o.CredentialsFile)
				if err != nil {
					return trace.Wrap(trace.ConvertSystemError(err), "could not read credentials file")
				}
				o.Credentials = string(data)
			}
		case TypeStdout:
		default:
			return trace.BadParameter("unsupported %q, supported are: %q, %q", o.Type, TypeStackdriver, TypeStdout)
		}
		cfg.Outputs[i] = o
	}
	return nil
}

// Plugin is a new logging plugin
type Plugin struct {
	cfg    Config
	logger force.Logger
}

// NewLogger generates a new logger for a process
func (p *Plugin) NewLogger() force.Logger {
	if p.logger != nil {
		return p.logger
	}
	return &Logger{plugin: p, FieldLogger: log.StandardLogger()}
}

type Logger struct {
	log.FieldLogger
	plugin *Plugin
}

func (l *Logger) Writer() io.WriteCloser {
	return Writer(&MultilineWriter{logger: l})
}

// WithError returns a logger bound to an error
func (l *Logger) WithError(err error) force.Logger {
	return &Logger{
		FieldLogger: l.FieldLogger.WithError(err),
		plugin:      l.plugin,
	}
}

func (l *Logger) URL(ctx force.ExecutionContext) string {
	for _, o := range l.plugin.cfg.Outputs {
		if o.Type == TypeStackdriver {
			u, err := url.Parse("https://console.cloud.google.com/logs/viewer")
			if err != nil {
				log.Errorf("Failed to parse %v", err)
				return ""
			}
			q := u.Query()
			// filter by force unique job id
			q.Set("advancedFilter",
				fmt.Sprintf("labels.%v=%v", force.KeyID, ctx.ID()))
			// last 24 hours
			q.Set("interval", "P1D")
			u.RawQuery = q.Encode()
			return u.String()
		}
	}
	return ""
}

// AddFields adds fields to the logger
func (l *Logger) AddFields(fields map[string]interface{}) force.Logger {
	fieldLogger := l.FieldLogger.WithFields(fields)
	return &Logger{FieldLogger: fieldLogger, plugin: l.plugin}
}

// Plugin sets up build plugin
func Setup(cfg Config) force.SetupFunc {
	return func(group force.Group) error {
		if err := cfg.CheckAndSetDefaults(); err != nil {
			return trace.Wrap(err)
		}
		level, err := log.ParseLevel(cfg.Level)
		if err != nil {
			return trace.Wrap(err)
		}
		if group.IsDebug() {
			level = log.DebugLevel
		}
		if level >= log.DebugLevel {
			trace.SetDebug(true)
		}
		var loggers []force.Logger
		plugin := &Plugin{
			cfg: cfg,
		}
		for _, o := range cfg.Outputs {
			switch o.Type {
			case TypeStackdriver:
				l := log.New()
				l.SetLevel(level)
				l.SetOutput(ioutil.Discard)
				h, err := stack.NewHook(stack.Config{
					Context: group.Context(),
					Creds:   []byte(o.Credentials),
				})
				if err != nil {
					return trace.Wrap(err)
				}
				l.AddHook(h)
				// disable line and file info in case if it's not debug
				var formatCaller func() string
				if level < log.DebugLevel {
					formatCaller = func() string { return "" }
				}
				l.SetFormatter(&trace.TextFormatter{
					DisableTimestamp: true,
					EnableColors:     false,
					FormatCaller:     formatCaller,
				})
				loggers = append(loggers, &Logger{FieldLogger: l, plugin: plugin})
			case TypeStdout:
				l := log.New()
				l.SetLevel(level)
				l.SetOutput(os.Stdout)
				l.SetFormatter(&TerminalFormatter{})
				loggers = append(loggers, &Logger{FieldLogger: l, plugin: plugin})
			default:
				return trace.BadParameter("unsupported %q, supported are: %q, %q", o.Type, TypeStackdriver, TypeStdout)
			}
		}
		plugin.logger = force.NewMultiLogger(loggers...)
		group.SetPlugin(Key, plugin)
		return nil
	}
}

// TerminalFormatter outputs message optimized for terminal
type TerminalFormatter struct{}

// Format formats the log message
func (*TerminalFormatter) Format(e *log.Entry) (data []byte, err error) {
	if e == nil {
		return nil, nil
	}
	return []byte(e.Message + "\n"), nil
}
