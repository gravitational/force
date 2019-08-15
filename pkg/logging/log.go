package logging

import (
	"fmt"
	"io/ioutil"
	"net/url"
	"os"

	"github.com/gravitational/force"
	"github.com/gravitational/force/pkg/logging/stack"

	"github.com/gravitational/trace"
	log "github.com/sirupsen/logrus"
)

// LoggingKey is a wrapper around string
// to namespace a variable
type LoggingKey string

// LoggingPlugin is a name of the github plugin variable
const LoggingPlugin = LoggingKey("logging")

const (
	TypeStackdriver = "stackdriver"
	TypeStdout      = "stdout"
)

type Config struct {
	// Level is a debugging level
	Level force.String
	// Outputs
	Outputs []Output
}

type Output struct {
	// Type is a logging type, currently supported
	// is stackdriver and stdout
	Type force.String
	// CredentialsFile is a path to credentials file,
	// used in case of stackdriver plugin
	CredentialsFile force.String
	// Credentials is a string with creds
	Credentials force.String
}

// CheckAndSetDefaults checks and sets default values
func (cfg *Config) CheckAndSetDefaults() error {
	if cfg.Level == "" {
		cfg.Level = force.String(log.InfoLevel.String())
	} else {
		_, err := log.ParseLevel(string(cfg.Level))
		if err != nil {
			return trace.Wrap(err)
		}
	}

	for i := range cfg.Outputs {
		o := &cfg.Outputs[i]
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
				data, err := ioutil.ReadFile(string(o.CredentialsFile))
				if err != nil {
					return trace.Wrap(trace.ConvertSystemError(err), "could not read credentials file")
				}
				o.Credentials = force.String(data)
			}
		case TypeStdout:
		default:
			return trace.BadParameter("unsupported %q, supported are: %q, %q", o.Type, TypeStackdriver, TypeStdout)
		}
	}

	return nil
}

// Plugin is a new logging plugin
type Plugin struct {
	Config
}

// NewLogger generates a new logger for a process
func (p *Plugin) NewLogger() force.Logger {
	return &Logger{FieldLogger: log.StandardLogger(), plugin: p}
}

type Logger struct {
	log.FieldLogger
	plugin *Plugin
}

// WithError returns a logger bound to an error
func (l *Logger) WithError(err error) force.Logger {
	return &Logger{
		FieldLogger: l.FieldLogger.WithError(err),
		plugin:      l.plugin,
	}
}

func (l *Logger) URL(ctx force.ExecutionContext) string {
	for _, o := range l.plugin.Outputs {
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

// NewPlugin creates new instances of plugins
type NewPlugin struct {
}

// NewInstance returns a new instance of a plugin bound to group
func (n *NewPlugin) NewInstance(group force.Group) (force.Group, interface{}) {
	return group, func(cfg Config) (*Plugin, error) {
		if err := cfg.CheckAndSetDefaults(); err != nil {
			return nil, trace.Wrap(err)
		}
		level, err := log.ParseLevel(string(cfg.Level))
		if err != nil {
			return nil, trace.Wrap(err)
		}
		if group.IsDebug() {
			level = log.DebugLevel
		}
		if level >= log.DebugLevel {
			trace.SetDebug(true)
		}
		log.SetLevel(level)
		var hasTerminal bool
		for _, o := range cfg.Outputs {
			switch o.Type {
			case TypeStackdriver:
				h, err := stack.NewHook(stack.Config{
					Context: group.Context(),
					Creds:   []byte(o.Credentials),
				})
				if err != nil {
					return nil, trace.Wrap(err)
				}
				log.AddHook(h)
			case TypeStdout:
				hasTerminal = trace.IsTerminal(os.Stdout)
				log.SetOutput(os.Stdout)
			default:
				return nil, trace.BadParameter("unsupported %q, supported are: %q, %q", o.Type, TypeStackdriver, TypeStdout)
			}
		}
		// disable line and file info in case if it's not debug
		var formatCaller func() string
		if level < log.DebugLevel {
			formatCaller = func() string { return "" }
		}
		log.SetFormatter(&trace.TextFormatter{
			DisableTimestamp: true,
			EnableColors:     hasTerminal,
			FormatCaller:     formatCaller,
		})
		p := &Plugin{
			Config: cfg,
		}
		group.SetPlugin(LoggingPlugin, p)
		return p, nil
	}
}

// Infof returns an action that logs in infor
func Infof(format force.String, args ...interface{}) force.Action {
	return &InfofAction{
		format: format,
		args:   args,
	}
}

type InfofAction struct {
	format force.String
	args   []interface{}
}

func (s *InfofAction) Run(ctx force.ExecutionContext) error {
	log := force.Log(ctx)
	evalArgs := make([]interface{}, len(s.args))
	var err error
	for i := range s.args {
		evalArgs[i], err = force.Eval(ctx, s.args[i])
		if err != nil {
			// use as is without eval
			evalArgs[i] = s.args[i]
		}
	}
	log.Infof(string(s.format), evalArgs...)
	return nil
}

func (s *InfofAction) String() string {
	return fmt.Sprintf("Infof()")
}
