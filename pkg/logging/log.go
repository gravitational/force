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

// Output is a logging output
type Output struct {
	// Type is a logging type, currently supported
	// is stackdriver and stdout
	Type force.StringVar
	// CredentialsFile is a path to credentials file,
	// used in case of stackdriver plugin
	CredentialsFile force.StringVar
	// Credentials is a string with creds
	Credentials force.StringVar
}

type evaluatedOutput struct {
	otype       string
	credentials string
}

// LogConfig is a log configuration
type LogConfig struct {
	// Level is a debugging level
	Level force.StringVar
	// Outputs is a list of logging outputs
	Outputs []Output
}

type evaluatedConfig struct {
	level   string
	outputs []evaluatedOutput
}

// CheckAndSetDefaults checks and sets default values
func (cfg *LogConfig) CheckAndSetDefaults(ctx force.ExecutionContext) (*evaluatedConfig, error) {
	ecfg := evaluatedConfig{}
	var err error
	if ecfg.level, err = force.EvalString(ctx, cfg.Level); err != nil {
		return nil, trace.Wrap(err)
	}
	if ecfg.level == "" {
		ecfg.level = log.InfoLevel.String()
	} else {
		_, err := log.ParseLevel(ecfg.level)
		if err != nil {
			return nil, trace.Wrap(err)
		}
	}

	for _, o := range cfg.Outputs {
		var out evaluatedOutput
		out.otype, err = force.EvalString(ctx, o.Type)
		if err != nil {
			return nil, trace.Wrap(err)
		}
		switch out.otype {
		case TypeStackdriver:
			credentials, err := force.EvalString(ctx, o.Credentials)
			if err != nil {
				return nil, trace.Wrap(err)
			}
			credentialsFile, err := force.EvalString(ctx, o.CredentialsFile)
			if err != nil {
				return nil, trace.Wrap(err)
			}
			if credentials == "" && credentialsFile == "" {
				return nil, trace.BadParameter(
					"provide Credentials or CredentialsFile in LoggingConfig when using %q logging,"+
						" read https://cloud.google.com/logging/docs/agent/authorization for more details.", o.Type)
			}
			if credentials != "" && credentialsFile != "" {
				return nil, trace.BadParameter("provide either Credentials or CredentialsFile, not both for %q logger", o.Type)
			}
			if credentialsFile != "" {
				data, err := ioutil.ReadFile(string(credentialsFile))
				if err != nil {
					return nil, trace.Wrap(trace.ConvertSystemError(err), "could not read credentials file")
				}
				credentials = string(data)
			}
			out.credentials = credentials
		case TypeStdout:
		default:
			return nil, trace.BadParameter("unsupported %q, supported are: %q, %q", o.Type, TypeStackdriver, TypeStdout)
		}
		ecfg.outputs = append(ecfg.outputs, out)
	}
	return &ecfg, nil
}

// Plugin is a new logging plugin
type Plugin struct {
	cfg evaluatedConfig
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
	for _, o := range l.plugin.cfg.outputs {
		if o.otype == TypeStackdriver {
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

// Log returns action that sets up log plugin
func Log(cfg LogConfig) (force.Action, error) {
	return &NewPlugin{
		cfg: cfg,
	}, nil
}

// NewPlugin creates new instances of plugins
type NewPlugin struct {
	cfg LogConfig
}

// NewInstance returns a new instance of a plugin bound to group
func (n *NewPlugin) NewInstance(group force.Group) (force.Group, interface{}) {
	return group, Log
}

// MarshalCode marshals plugin setup to code
func (n *NewPlugin) MarshalCode(ctx force.ExecutionContext) ([]byte, error) {
	return force.NewFnCall(ctx, n.cfg).MarshalCode(ctx)
}

// Run sets up logging plugin for the instance group
func (n *NewPlugin) Run(ctx force.ExecutionContext) error {
	cfg, err := n.cfg.CheckAndSetDefaults(ctx)
	if err != nil {
		return trace.Wrap(err)
	}
	level, err := log.ParseLevel(cfg.level)
	if err != nil {
		return trace.Wrap(err)
	}
	group := ctx.Process().Group()
	if group.IsDebug() {
		level = log.DebugLevel
	}
	if level >= log.DebugLevel {
		trace.SetDebug(true)
	}
	log.SetLevel(level)
	var hasTerminal bool
	for _, o := range cfg.outputs {
		switch o.otype {
		case TypeStackdriver:
			h, err := stack.NewHook(stack.Config{
				Context: group.Context(),
				Creds:   []byte(o.credentials),
			})
			if err != nil {
				return trace.Wrap(err)
			}
			log.AddHook(h)
		case TypeStdout:
			hasTerminal = trace.IsTerminal(os.Stdout)
			log.SetOutput(os.Stdout)
		default:
			return trace.BadParameter("unsupported %q, supported are: %q, %q", o.otype, TypeStackdriver, TypeStdout)
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
		cfg: *cfg,
	}
	group.SetPlugin(LoggingPlugin, p)
	return nil
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
			evalArgs[i] = err.Error()
		}
	}
	log.Infof(string(s.format), evalArgs...)
	return nil
}

// MarshalCode marshals the action into code representation
func (s *InfofAction) MarshalCode(ctx force.ExecutionContext) ([]byte, error) {
	call := &force.FnCall{
		Fn:   Infof,
		Args: []interface{}{s.format},
	}
	call.Args = append(call.Args, s.args...)
	return call.MarshalCode(ctx)
}

func (s *InfofAction) String() string {
	return fmt.Sprintf("Infof()")
}
