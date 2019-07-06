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
			versions, err := r.check(afterDate)
			if err != nil {
				log.Warningf("Pull request check failed: %v", err)
				continue
			}
			if len(versions) == 0 {
				log.Debugf("No new pull request versions: %v", err)
				continue
			}
			afterDate = versions[len(versions)-1].CommittedDate
			for _, version := range versions {
				event := &RepoEvent{Version: version}
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

// check returns all versions after given version
func (r *RepoWatcher) check(afterDate time.Time) (Versions, error) {
	var versions Versions

	pulls, err := r.client.ListOpenPullRequests()
	if err != nil {
		return nil, trace.Wrap(err, "failed to get last commits")
	}

	for _, p := range pulls {
		if p.PullRequestObject.BaseRefName != r.Source.Branch {
			continue
		}
		if !p.Tip.CommittedDate.Time.After(afterDate) {
			continue
		}
		versions = append(versions, NewVersion(p))
	}

	// Sort the commits by date
	sort.Sort(versions)

	return versions, nil
}

func (r *RepoWatcher) Events() <-chan force.Event {
	return r.eventsC
}

func (r *RepoWatcher) Done() <-chan struct{} {
	return nil
}

type RepoEvent struct {
	Version Version
}

func (r *RepoEvent) String() string {
	return fmt.Sprintf("RepoEvent(PR=%v, commit=%v, date=%v)",
		r.Version.PR, r.Version.Commit, r.Version.CommittedDate)
}
