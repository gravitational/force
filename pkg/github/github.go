package github

import (
	"context"
	"fmt"
	"sort"
	"time"

	"github.com/gravitational/force"
	"github.com/gravitational/trace"
	log "github.com/sirupsen/logrus"
)

func Github(src Source) (force.Channel, error) {
	if err := src.CheckAndSetDefaults(); err != nil {
		return nil, trace.Wrap(err)
	}
	client, err := NewGithubClient(context.TODO(), src)
	if err != nil {
		return nil, trace.Wrap(err)
	}
	return &RepoWatcher{
		Source: src,
		client: client,
		// TODO(klizhentas): queues have to be configurable
		eventsC: make(chan force.Event, 1024),
	}, nil
}

type RepoWatcher struct {
	Source  Source
	client  *GithubClient
	eventsC chan force.Event
}

func (r *RepoWatcher) String() string {
	return fmt.Sprintf("RepoWatcher(%v/%v)", r.client.Owner, r.client.Repository)
}

func (r *RepoWatcher) Start(pctx context.Context) error {
	go r.pollRepo(pctx)
	return nil
}

func (r *RepoWatcher) pollRepo(ctx context.Context) {
	var afterDate time.Time
	for {
		select {
		case <-ctx.Done():
			return
		case <-time.After(5 * time.Second):
			pulls, err := r.updatedPullRequests(afterDate)
			if err != nil {
				log.Warningf("Pull request check failed: %v", trace.DebugReport(err))
				continue
			}
			if len(pulls) == 0 {
				continue
			}
			afterDate = pulls[len(pulls)-1].LastUpdated()
			for _, pr := range pulls {
				event := &RepoEvent{PR: pr}
				select {
				case r.eventsC <- event:
					log.Infof("-> %v", event)
				case <-ctx.Done():
					return
				}
			}
		}
	}
}

// updatedPullRequests returns all pull requests updated after given date
func (r *RepoWatcher) updatedPullRequests(afterDate time.Time) (PullRequests, error) {
	var updatedPulls PullRequests

	pulls, err := r.client.GetOpenPullRequests()
	if err != nil {
		return nil, trace.Wrap(err)
	}

	for _, pr := range pulls {
		if pr.PullRequestObject.BaseRefName != r.Source.Branch {
			continue
		}
		if !pr.LastUpdated().After(afterDate) {
			continue
		}
		updatedPulls = append(updatedPulls, pr)
	}

	// Sort the commits by date
	sort.Sort(updatedPulls)

	return updatedPulls, nil
}

func (r *RepoWatcher) Events() <-chan force.Event {
	return r.eventsC
}

func (r *RepoWatcher) Done() <-chan struct{} {
	return nil
}

type RepoEvent struct {
	PR PullRequest
}

func (r *RepoEvent) String() string {
	return fmt.Sprintf("RepoEvent(PR=%v, commit=%v, last updated=%v, comment=%q)",
		r.PR.Number, r.PR.LastCommit, r.PR.LastUpdated(), r.PR.LastComment)
}
