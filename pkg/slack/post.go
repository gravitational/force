package slack

import (
	"fmt"

	"github.com/gravitational/force"

	"github.com/gravitational/trace"
)

// NewPostStatusOf returns a function that wraps underlying action
// and tracks the result, posting the result back
type NewPostStatusOf struct {
}

// NewInstance returns a function creating new post status actions
func (n *NewPostStatusOf) NewInstance(group force.Group) (force.Group, interface{}) {
	// PostStatusOf creates a sequence, that's why it has to create a new lexical
	// scope (as sequence expects one to be created)
	scope := force.WithLexicalScope(group)
	return scope, func(inner ...force.Action) (force.Action, error) {
		pluginI, ok := group.GetPlugin(Key)
		if !ok {
			return nil, trace.NotFound("slack plugin is not initialized, use slack.Setup to initialize it")
		}
		return &PostStatusOfAction{
			plugin:  pluginI.(*Plugin),
			actions: inner,
		}, nil
	}
}

// PostStatusOfAction executes an action and posts its status to slack
type PostStatusOfAction struct {
	plugin  *Plugin
	actions []force.Action
}

func (p *PostStatusOfAction) Run(ctx force.ExecutionContext) error {
	event, ok := ctx.Event().(*ChatEvent)
	if !ok {
		// it should be possible to execute post status
		// in the standalone mode given all the parameters
		return trace.BadParameter("slack.PostStatusOf can only be executed with Listen")
	}
	log := force.Log(ctx)
	if err := event.convo.sendMessage(
		fmt.Sprintf(":shipit: Started action, check logs at %v.", log.URL(ctx))); err != nil {
		return trace.Wrap(err)
	}

	err := force.Sequence(p.actions...).Run(ctx)
	if err != nil {
		if err2 := event.convo.sendMessage(
			fmt.Sprintf(":collision: Action failed with %v.", err)); err != nil {
			return trace.NewAggregate(err, err2)
		}
	}

	if err := event.convo.sendMessage(
		fmt.Sprintf(":heavy_check_mark: Action completed successfully.")); err != nil {
		return trace.Wrap(err)
	}

	return nil
}

// MarshalCode marshals the action into code representation
func (p *PostStatusOfAction) MarshalCode(ctx force.ExecutionContext) ([]byte, error) {
	call := &force.FnCall{
		Package: string(Key),
		Fn:      KeyPostStatusOf,
	}
	for i := range p.actions {
		call.Args = append(call.Args, p.actions[i])
	}
	return call.MarshalCode(ctx)
}
