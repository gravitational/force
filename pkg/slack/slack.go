package slack

import (
	"reflect"
	"time"

	"github.com/gravitational/force"
	"github.com/gravitational/trace"
	"github.com/nlopes/slack"
)

// Scope returns a new scope with all the functions and structs
// defined, this is the entrypoint into plugin as far as force is concerned
func Scope() (force.Group, error) {
	scope := force.WithLexicalScope(nil)
	err := force.ImportStructsIntoAST(scope,
		reflect.TypeOf(Config{}),
		reflect.TypeOf(Command{}),
		reflect.TypeOf(StringsEnum{}),
		reflect.TypeOf(String{}),
	)
	if err != nil {
		return nil, trace.Wrap(err)
	}
	scope.AddDefinition(KeyListen, &NewListen{})
	scope.AddDefinition(force.StructName(reflect.TypeOf(Setup{})), &Setup{})
	scope.AddDefinition(KeyPostStatusOf, &NewPostStatusOf{})
	return scope, nil
}

//Namespace is a wrapper around string to namespace a variable in the context
type Namespace string

// Key is a name of the plugin variable
const Key = Namespace("slack")

const (
	KeySetup        = "Setup"
	KeyConfig       = "Config"
	KeyListen       = "Listen"
	KeyPostStatusOf = "PostStatusOf"
)

// Config is a slack configuration
type Config struct {
	// Token is a slack token
	Token string
}

func (cfg *Config) CheckAndSetDefaults() error {
	if cfg.Token == "" {
		return trace.BadParameter("set slack.Config{Token:``} parameter")
	}
	return nil
}

// Plugin is a new plugin
type Plugin struct {
	// start is a plugin start time
	start  time.Time
	cfg    Config
	client *slack.Client
}

// Setup creates new plugin instances
type Setup struct {
	cfg interface{}
}

// NewInstance returns function creating new client bound to the process group
// and registers plugin variable
func (n *Setup) NewInstance(group force.Group) (force.Group, interface{}) {
	return group, func(cfg interface{}) (force.Action, error) {
		return &Setup{
			cfg: cfg,
		}, nil
	}
}

// Run sets up git plugin for the process group
func (n *Setup) Run(ctx force.ExecutionContext) error {
	var cfg Config
	if err := force.EvalInto(ctx, n.cfg, &cfg); err != nil {
		return trace.Wrap(err)
	}
	if err := cfg.CheckAndSetDefaults(); err != nil {
		return trace.Wrap(err)
	}
	client := slack.New(
		cfg.Token,
		//		slack.OptionDebug(trace.IsDebug()),
		//		slack.OptionLog(stdlog.New(os.Stdout, "slack-bot: ", stdlog.Lshortfile|stdlog.LstdFlags)),
	)
	plugin := &Plugin{cfg: cfg, start: time.Now().UTC(), client: client}
	ctx.Process().Group().SetPlugin(Key, plugin)
	return nil
}

// MarshalCode marshals plugin code to representation
func (n *Setup) MarshalCode(ctx force.ExecutionContext) ([]byte, error) {
	call := &force.FnCall{
		Package: string(Key),
		FnName:  KeySetup,
		Args:    []interface{}{n.cfg},
	}
	return call.MarshalCode(ctx)
}
