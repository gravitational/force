package slack

import (
	"context"
	"fmt"
	"reflect"
	"time"

	"github.com/gravitational/force"
	"github.com/gravitational/trace"
)

// NewListen returns new listen
type NewListen struct {
}

// NewInstance returns a function creating new watchers
func (n *NewListen) NewInstance(group force.Group) (force.Group, interface{}) {
	return group, func(cmd interface{}) (force.Channel, error) {
		var command Command
		if err := force.EvalInto(force.EmptyContext(), cmd, &command); err != nil {
			return nil, trace.Wrap(err)
		}
		structType, err := generateStructType(command)
		if err != nil {
			return nil, trace.Wrap(err)
		}
		values, err := generateEmptyValues(command, structType)
		if err != nil {
			return nil, trace.Wrap(err)
		}
		group.AddDefinition(force.KeyEvent, ChatEvent{
			Values: values,
		})
		pluginI, ok := group.GetPlugin(Key)
		if !ok {
			return nil, trace.NotFound("slack plugin is not initialized, use slack.Setup to initialize it")
		}
		return &Listener{
			plugin:     pluginI.(*Plugin),
			valuesType: structType,
			command:    command,
			// TODO(klizhentas): queues have to be configurable
			eventsC: make(chan force.Event, 1024),
		}, nil
	}
}

// Listener is chat listener
type Listener struct {
	plugin     *Plugin
	command    Command
	valuesType reflect.Type
	eventsC    chan force.Event
}

// String returns user friendly representation of the watcher
func (r *Listener) String() string {
	return fmt.Sprintf("Listener()")
}

// MarshalCode marshals things to code
func (r *Listener) MarshalCode(ctx force.ExecutionContext) ([]byte, error) {
	// TODO: klizhentas add
	return nil, nil
}

// Start starts watch on a repo
func (r *Listener) Start(pctx context.Context) error {
	dialog := Dialog{Commands: []Command{r.command}}
	parser, err := NewParser(dialog)
	if err != nil {
		return trace.Wrap(err)
	}
	log := force.Log(pctx)
	bot := newBot(botConfig{
		client:     r.plugin.client,
		parser:     parser,
		eventsC:    r.eventsC,
		log:        log,
		dialog:     dialog,
		valuesType: r.valuesType,
	})
	go func() {
		err := bot.Listen(pctx)
		if err != nil {
			log.WithError(err).Errorf("Chat bot exited.")
		}
	}()
	return nil
}

// Events returns events stream with commands
func (r *Listener) Events() <-chan force.Event {
	return r.eventsC
}

// Done returns channel closed when repository watcher is closed
func (r *Listener) Done() <-chan struct{} {
	return nil
}

// ChatEvent is event
type ChatEvent struct {
	created time.Time
	convo   *conversation
	Values  interface{}
}

// generateStruct generates struct prototype
func generateStructType(command Command) (reflect.Type, error) {
	structFields := make([]reflect.StructField, 0, len(command.Fields))
	for _, field := range command.Fields {
		val := field.Value.DefaultValue()
		outType, err := force.ConvertTypeToAST(reflect.TypeOf(val))
		if err != nil {
			return nil, trace.Wrap(err)
		}
		structFields = append(structFields, reflect.StructField{
			Name: force.Capitalize(field.Name),
			Type: outType,
		})
	}
	return reflect.StructOf(structFields), nil
}

func generateEmptyValues(command Command, structType reflect.Type) (interface{}, error) {
	structValPtr := reflect.New(structType)
	structVal := structValPtr.Elem()
	for _, field := range command.Fields {
		_, exists := structType.FieldByName(force.Capitalize(field.Name))
		if !exists {
			return nil, trace.BadParameter("field %v is not found", force.Capitalize(field.Name))
		}
		outVal, err := force.ConvertValueToAST(field.Value.DefaultValue())
		if err != nil {
			return nil, trace.Wrap(err)
		}
		structField := structVal.FieldByName(force.Capitalize(field.Name))
		iface, ok := structField.Interface().(force.Converter)
		if ok {
			if structField.Type().Kind() != reflect.Ptr {
				converted, err := iface.Convert(outVal)
				if err != nil {
					return nil, trace.Wrap(err)
				}
				structField.Set(reflect.ValueOf(converted))
			}
		} else {
			structField.Set(force.Zero(reflect.ValueOf(outVal)))
		}
	}
	return structVal.Interface(), nil
}

// generateValues generates event spec populated with fields
func generateValues(command Command, structType reflect.Type, values map[string]interface{}) (interface{}, error) {
	structValPtr := reflect.New(structType)
	structVal := structValPtr.Elem()
	for _, field := range command.Fields {
		_, exists := structType.FieldByName(force.Capitalize(field.Name))
		if !exists {
			return nil, trace.BadParameter("field %v is not found", force.Capitalize(field.Name))
		}
		val, ok := values[field.Name]
		if !ok {
			val = field.Value.DefaultValue()
		}
		outVal, err := force.ConvertValueToAST(val)
		if err != nil {
			return nil, trace.Wrap(err)
		}
		field := structVal.FieldByName(force.Capitalize(field.Name))
		iface, ok := field.Interface().(force.Converter)
		if ok {
			if field.Type().Kind() != reflect.Ptr {
				converted, err := iface.Convert(outVal)
				if err != nil {
					return nil, trace.Wrap(err)
				}
				field.Set(reflect.ValueOf(converted))
			}
		} else {
			field.Set(reflect.ValueOf(outVal))
		}
	}
	return structVal.Interface(), nil
}

// Created returns a time when the event was originated
func (c *ChatEvent) Created() time.Time {
	return c.created
}

// AddMetadata adds metadata to the logger
// and the context, such as commit id and PR number
func (r *ChatEvent) AddMetadata(ctx force.ExecutionContext) {
	logger := force.Log(ctx)
	//logger = logger.AddFields()
	force.SetLog(ctx, logger)
	// Those variables can be set, as they are defined by
	// PullRequests in a separate scope
	ctx.SetValue(force.ContextKey(force.KeyEvent), *r)
}

func (r *ChatEvent) String() string {
	return fmt.Sprintf("chat event")
}
