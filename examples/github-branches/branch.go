package main

import (
	"github.com/gravitational/force"
	"github.com/gravitational/force/pkg/github"
	"github.com/gravitational/force/pkg/runner"
)

// This example demonstrates a watcher set up to track
// any new commits on any branch in Github under path github
// and trigger the action
func main() {
	runner.Setup(
		// Github is a setup of the github plugin valid in the
		// context of this group, all calls to github methods will be using
		// this syntax
		github.Setup(github.Config{
			// Token is a github access token
			// passed to all callers in the group
			TokenFile: runner.ExitWithoutEnv("GITHUB_ACCESS_TOKEN_FILE"),
		})).
		// specify process name
		Name("force-ci").
		// Watch for any new commit in the branch
		Watch(github.Branches(github.Source{
			// Repo is a repository to watch
			Repo: "gravitational/force",
			Path: "docs/",
		})).
		// Run those actions
		Run(github.PostStatusOf(func(ctx force.ExecutionContext) error {
			event := ctx.Event().(*github.BranchEvent)
			force.Log(ctx).Infof("Got event, branch: %v, commit: %v.", event.Branch, event.Commit)
			return nil
		}))
}
