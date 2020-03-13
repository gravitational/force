package main

import (
	"github.com/gravitational/force"
	"github.com/gravitational/force/pkg/github"
	"github.com/gravitational/force/pkg/runner"
)

func main() {
	// This example demonstrates a github watch that triggers
	// action on any pull request to master branch
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
		// Watch events on the channel
		Watch(github.PullRequests(github.Source{
			// Repo is a repository to watch
			Repo: "gravitational/force",
			// Default branch is master
			BranchPattern: "master",
			// Approval configures an approval flow
			Approval: github.Approval{
				// Requies sets the approval as required
				Required: true,
				// Teams is a list of github teams that can approve
				Teams: []string{"gravitational/devc", "gravitational/admins"},
			}})).
		// Run those actions
		Run(github.PostStatusOf(func(ctx force.ExecutionContext) error {
			log := force.Log(ctx)
			log.Infof("Triggered process run for %v", ctx.Event())
			return nil
		}))
}
