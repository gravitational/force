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
		seq, err := force.Sequence(inner...)
		if err != nil {
			return nil, trace.Wrap(err)
		}
		return &PostStatusOfAction{
			plugin:  pluginI.(*Plugin),
			actions: inner,
			seq:     seq,
		}, nil
	}
}

// PostStatusOfAction executes an action and posts its status to slack
type PostStatusOfAction struct {
	plugin  *Plugin
	seq     force.ScopeAction
	actions []force.Action
}

func (p *PostStatusOfAction) Type() interface{} {
	return p.seq.Type()
}

func (p *PostStatusOfAction) Eval(ctx force.ExecutionContext) (interface{}, error) {
	event, ok := ctx.Event().(*ChatEvent)
	if !ok {
		// it should be possible to execute post status
		// in the standalone mode given all the parameters
		return nil, trace.BadParameter("slack.PostStatusOf can only be executed with Listen")
	}
	log := force.Log(ctx)
	if err := event.convo.sendMessage(
		fmt.Sprintf(":shipit: Started action, check logs at %v.", log.URL(ctx))); err != nil {
		return nil, trace.Wrap(err)
	}

	out, err := p.seq.Eval(ctx)
	if err != nil {
		if err2 := event.convo.sendMessage(
			fmt.Sprintf(":collision: Action failed with %v.", err)); err != nil {
			return nil, trace.NewAggregate(err, err2)
		}
	}

	if err := event.convo.sendMessage(
		fmt.Sprintf(":heavy_check_mark: Action completed successfully.")); err != nil {
		return nil, trace.Wrap(err)
	}

	return out, nil
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
