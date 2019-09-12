package github

import (
	"fmt"
	"strings"

	"github.com/gravitational/force"

	"github.com/google/go-github/github"
	"github.com/gravitational/trace"
)

// PostStatus updates pull request status
func (p *Plugin) PostStatus(status Status) (force.Action, error) {
	if err := status.CheckAndSetDefaults(); err != nil {
		return nil, trace.Wrap(err)
	}
	return &PostStatusAction{
		status: status,
		plugin: p,
	}, nil
}

// PostStatusOf executes inner action and posts result of it's execution
// to github
func (p *Plugin) PostStatusOf(actions ...force.Action) (force.Action, error) {
	return &PostStatusOfAction{
		actions: actions,
		plugin:  p,
	}, nil
}

// PostStatusAction posts github status
type PostStatusAction struct {
	status Status
	plugin *Plugin
	repo   Repository
}

// Run posts github status
func (p *PostStatusAction) Run(ctx force.ExecutionContext) error {
	event, ok := ctx.Event().(*RepoEvent)
	if !ok {
		// it should be possible to execute post status
		// in the standalone mode given all the parameters
		return trace.BadParameter("PostStatus can only be executed with Watch")
	}
	repo := event.Source.Repo

	log := force.Log(ctx)
	commitRef := event.PullRequest.LastCommit.OID

	_, _, err := p.plugin.client.V3.Repositories.CreateStatus(
		ctx,
		repo.Owner,
		repo.Name,
		commitRef,
		&github.RepoStatus{
			State:       github.String(p.status.State),
			TargetURL:   github.String(log.URL(ctx)),
			Description: github.String(p.status.Description),
			Context:     github.String(p.status.Context),
		},
	)

	log.Debugf("Posted %v -> %v.", p.status, err)

	return trace.Wrap(err)
}

// MarshalCode marshals the action into code representation
func (p *PostStatusAction) MarshalCode(ctx force.ExecutionContext) ([]byte, error) {
	return force.NewFnCall(p.plugin.PostStatus, p.status).MarshalCode(ctx)
}

// PostStatusOfAction executes an action and posts its status
// to the github
type PostStatusOfAction struct {
	plugin  *Plugin
	actions []force.Action
}

func (p *PostStatusOfAction) Run(ctx force.ExecutionContext) error {
	// First, post pending status
	pending := Status{State: StatePending}
	if err := pending.CheckAndSetDefaults(); err != nil {
		return trace.Wrap(err)
	}
	postPending := &PostStatusAction{
		status: pending,
		plugin: p.plugin,
	}

	// run the inner action
	err := postPending.Run(ctx)
	if err != nil {
		return trace.Wrap(err)
	}

	// Post result of the execution of all actions in a sequence
	result := Status{State: StateSuccess, Description: "CI executed successfully"}
	if err := result.CheckAndSetDefaults(); err != nil {
		return trace.Wrap(err)
	}
	err = force.Sequence(p.actions...).Run(ctx)
	if err != nil {
		result.State = StateFailure
		result.Description = err.Error()
	}
	postResult := &PostStatusAction{
		status: result,
		plugin: p.plugin,
	}
	return trace.NewAggregate(err, postResult.Run(ctx))
}

// MarshalCode marshals the action into code representation
func (p *PostStatusOfAction) MarshalCode(ctx force.ExecutionContext) ([]byte, error) {
	call := &force.FnCall{
		Fn: p.plugin.PostStatusOf,
	}
	for i := range p.actions {
		call.Args = append(call.Args, p.actions[i])
	}
	return call.MarshalCode(ctx)
}

type Status struct {
	// State is a PR state
	State string
	// URL is a url of this web app, force should provide a web interface
	URL string
	// Description is an optional description
	Description string
	// Context is a special label that differentiates this application
	Context string
}

func (s Status) String() string {
	return fmt.Sprintf("Status(state=%v, url=%v, description=%v)", s.State, s.URL, s.Description)
}

func (s *Status) CheckAndSetDefaults() error {
	if s.State == "" {
		return trace.BadParameter("provide Status{State: } value")
	}
	var found bool
	for _, allowed := range allowedStates {
		if s.State == allowed {
			found = true
			break
		}
	}
	if s.Context == "" {
		s.Context = DefaultContext
	}
	if !found {
		return trace.BadParameter("%q is not a valid states, use one of %v", s.State, strings.Join(allowedStates, ","))
	}
	return nil
}

const (
	StateSuccess = "success"
	StatePending = "pending"
	StateFailure = "failure"
	StateError   = "error"

	DefaultContext = "Force CI"
)

var allowedStates = []string{StateSuccess, StatePending, StateFailure, StateError}
