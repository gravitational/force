package log

import (
	"fmt"
	"io/ioutil"
	"net/url"
	"os"
	"reflect"

	"github.com/gravitational/force"
	"github.com/gravitational/force/pkg/log/stack"

	"github.com/gravitational/trace"
	log "github.com/sirupsen/logrus"
)

// Scope returns a new scope with all the functions and structs
// defined, this is the entrypoint into plugin as far as force is concerned
func Scope() (force.Group, error) {
	scope := force.WithLexicalScope(nil)
	err := force.ImportStructsIntoAST(scope,
		reflect.TypeOf(Config{}),
		reflect.TypeOf(Output{}),
	)
	if err != nil {
		return nil, trace.Wrap(err)
	}
	scope.AddDefinition(force.FunctionName(Infof), &force.NopScope{Func: Infof})
	scope.AddDefinition(force.StructName(reflect.TypeOf(Setup{})), &Setup{})
	return scope, nil
}

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
	cfg Config
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

// Log returns action that sets up log plugin
func Log(cfg interface{}) (force.Action, error) {
	return &Setup{
		cfg: cfg,
	}, nil
}

// Setup creates new instances of plugins
type Setup struct {
	cfg interface{}
}

// NewInstance returns a new instance of a plugin bound to group
func (n *Setup) NewInstance(group force.Group) (force.Group, interface{}) {
	return group, Log
}

// MarshalCode marshals plugin setup to code
func (n *Setup) MarshalCode(ctx force.ExecutionContext) ([]byte, error) {
	call := force.FnCall{
		Package: string(Key),
		FnName:  KeySetup,
		Args:    []interface{}{n.cfg},
	}
	return call.MarshalCode(ctx)
}

// Run sets up logging plugin for the instance group
func (n *Setup) Run(ctx force.ExecutionContext) error {
	var cfg Config
	if err := force.EvalInto(ctx, n.cfg, &cfg); err != nil {
		return trace.Wrap(err)
	}
	if err := cfg.CheckAndSetDefaults(); err != nil {
		return trace.Wrap(err)
	}
	level, err := log.ParseLevel(cfg.Level)
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
	for _, o := range cfg.Outputs {
		switch o.Type {
		case TypeStackdriver:
			h, err := stack.NewHook(stack.Config{
				Context: group.Context(),
				Creds:   []byte(o.Credentials),
			})
			if err != nil {
				return trace.Wrap(err)
			}
			log.AddHook(h)
		case TypeStdout:
			hasTerminal = trace.IsTerminal(os.Stdout)
			log.SetOutput(os.Stdout)
		default:
			return trace.BadParameter("unsupported %q, supported are: %q, %q", o.Type, TypeStackdriver, TypeStdout)
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
		cfg: cfg,
	}
	group.SetPlugin(Key, p)
	return nil
}

// Infof returns an action that logs in info
func Infof(format force.StringVar, args ...interface{}) force.Action {
	return &InfofAction{
		format: format,
		args:   args,
	}
}

type InfofAction struct {
	format force.StringVar
	args   []interface{}
}

func (s *InfofAction) Run(ctx force.ExecutionContext) error {
	log := force.Log(ctx)
	format, err := force.EvalString(ctx, s.format)
	if err != nil {
		return trace.Wrap(err)
	}
	evalArgs := make([]interface{}, len(s.args))
	for i := range s.args {
		evalArgs[i], err = force.Eval(ctx, s.args[i])
		if err != nil {
			// use as is without eval
			evalArgs[i] = err.Error()
		}
	}
	log.Infof(format, evalArgs...)
	return nil
}

// MarshalCode marshals the action into code representation
func (s *InfofAction) MarshalCode(ctx force.ExecutionContext) ([]byte, error) {
	call := &force.FnCall{
		Package: string(Key),
		Fn:      Infof,
		Args:    []interface{}{s.format},
	}
	call.Args = append(call.Args, s.args...)
	return call.MarshalCode(ctx)
}

func (s *InfofAction) String() string {
	return fmt.Sprintf("Infof()")
}
