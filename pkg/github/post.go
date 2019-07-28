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
		Status: status,
		plugin: p,
	}, nil
}

// PostPending updates pull request status
func (p *Plugin) PostPending() (force.Action, error) {
	s := Status{State: StatePending}
	if err := s.CheckAndSetDefaults(); err != nil {
		return nil, trace.Wrap(err)
	}
	return &PostStatusAction{
		Status: s,
		plugin: p,
	}, nil
}

// PostResult
func (p *Plugin) PostResult() (force.Action, error) {
	s := Status{State: StatePending}
	if err := s.CheckAndSetDefaults(); err != nil {
		return nil, trace.Wrap(err)
	}
	return &PostStatusAction{
		GetStatusFromContext: true,
		Status:               s,
		plugin:               p,
	}, nil
}

type PostStatusAction struct {
	GetStatusFromContext bool
	Status
	plugin *Plugin
}

func (p *PostStatusAction) Run(ctx force.ExecutionContext) (force.ExecutionContext, error) {
	event, ok := ctx.Event().(*RepoEvent)
	if !ok {
		// it should be possible to execute post status
		// in the standalone mode given all the parameters
		return nil, trace.BadParameter("PostStatus can only be executed with Watch")
	}

	log := force.Log(ctx)

	status := p.Status
	if p.GetStatusFromContext {
		err := force.Error(ctx)
		if err == nil {
			status.State = StateSuccess
			status.Description = "CI executed successfully"
		} else {
			status.State = StateFailure
			status.Description = err.Error()
		}
		log.Debugf("Getting status from execution context: %v.", status)
	}
	commitRef := event.PR.LastCommit.OID

	_, _, err := p.plugin.client.V3.Repositories.CreateStatus(
		ctx,
		p.plugin.client.Owner,
		p.plugin.client.Repository,
		commitRef,
		&github.RepoStatus{
			State:       github.String(status.State),
			TargetURL:   github.String(log.URL(ctx)),
			Description: github.String(status.Description),
			Context:     github.String(status.Context),
		},
	)

	log.Debugf("Posted %v -> %v.", status, err)

	return nil, trace.Wrap(err)
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
