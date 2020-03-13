package main

import (
	"github.com/gravitational/force"
	"github.com/gravitational/force/pkg/github"
	"github.com/gravitational/force/pkg/runner"
)

func main() {
	// In this example, the action is only triggered
	// if PR comment does not match regular expression
	runner.Setup(
		github.Setup(github.Config{
			TokenFile: runner.ExitWithoutEnv("GITHUB_ACCESS_TOKEN_FILE"),
		})).
		Watch(github.PullRequests(github.Source{
			Repo: "gravitational/force",
			Trigger: github.Trigger{
				SkipPattern: `*.skip ci.*`,
			},
		})).
		Run(github.PostStatusOf(func(ctx force.ExecutionContext) error {
			log := force.Log(ctx)
			// More advanced skip logic can be implemented if necessary
			// based on the title or any other PR metadata
			title := ctx.Event().(*github.PullRequestEvent).PullRequest.Title
			log.Infof("Triggered process run for %v, title %v", ctx.Event(), title)
			return nil
		}))
}
