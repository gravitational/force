package github

import (
	"fmt"
	"strings"

	"github.com/gravitational/force"

	"github.com/google/go-github/github"
	"github.com/gravitational/trace"
)

// PostStatusOf returns action that posts status of another action
func PostStatusOf(action force.ActionFunc) force.Action {
	return &PostStatusOfAction{
		action: action,
	}
}

// PostStatusAction posts github status
type PostStatusAction struct {
	status Status
	repo   Repository
	plugin *Plugin
}

// Run posts github status
func (p *PostStatusAction) Run(ctx force.ExecutionContext) error {
	event, ok := ctx.Event().(CommitGetter)
	if !ok {
		// it should be possible to execute post status
		// in the standalone mode given all the parameters
		return trace.BadParameter(
			"PostStatus can only be executed with github watch setup either with github.PullRequests or github.Commits")
	}
	repo, err := event.GetSource().Repository()
	if err != nil {
		return trace.Wrap(err)
	}

	log := force.Log(ctx)
	commitRef := event.GetCommit()

	if p.status.Context == "" {
		p.status.Context = ctx.Process().Name()
	}

	_, _, err = p.plugin.client.V3.Repositories.CreateStatus(
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

// PostStatusOfAction executes an action and posts its status
// to the github
type PostStatusOfAction struct {
	action force.Action
}

func (p *PostStatusOfAction) Run(ctx force.ExecutionContext) error {
	pluginI, ok := ctx.Process().Group().GetPlugin(Key)
	if !ok {
		return trace.NotFound("github plugin is not initialized, use github.Setup to initialize it")
	}
	plugin, ok := pluginI.(*Plugin)
	if !ok {
		return trace.NotFound("github plugin is not properly initialized, use github.Setup to initialize it")
	}

	// First, post pending status
	pending := Status{State: StatePending}
	if err := pending.CheckAndSetDefaults(); err != nil {
		return trace.Wrap(err)
	}
	postPending := &PostStatusAction{
		status: pending,
		plugin: plugin,
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
	err = p.action.Run(ctx)
	if err != nil {
		result.State = StateFailure
		result.Description = err.Error()
	}
	postResult := &PostStatusAction{
		status: result,
		plugin: plugin,
	}
	resultErr := postResult.Run(ctx)
	return trace.NewAggregate(err, resultErr)
}

// Status is a github status
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
