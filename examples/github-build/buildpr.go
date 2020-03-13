package main

import (
	"fmt"
	"io/ioutil"
	"os"

	"github.com/gravitational/force"
	"github.com/gravitational/force/pkg/builder"
	"github.com/gravitational/force/pkg/git"
	"github.com/gravitational/force/pkg/github"
	"github.com/gravitational/force/pkg/runner"

	"github.com/gravitational/trace"

	// this is required to run local runc builds
	_ "github.com/gravitational/force/internal/unshare"
)

func main() {
	loggingCredentials := runner.ExitWithoutEnv("GOOGLE_CREDENTIALS")

	// Reexec is required for rootless builds
	runner.Reexec()
	runner.Setup(
		// Github is a setup of the github plugin valid in the
		// context of this group, all calls to github methods will be using
		// this syntax
		github.Setup(github.Config{
			// Token is a github access token
			// passed to all callers in the group
			TokenFile: runner.ExitWithoutEnv("GITHUB_ACCESS_TOKEN_FILE"),
		}),

		// Git sets up git client for cloning repositories
		git.Setup(git.Config{
			PrivateKeyFile: runner.ExitWithoutEnv("GIT_PRIVATE_KEY_FILE"),
			KnownHostsFile: runner.ExitWithoutEnv("GIT_KNOWN_HOSTS_FILE"),
		}),

		// Builder configures docker builder
		builder.Setup(builder.Config{
			// Logs into quay io server
			Server: "gcr.io",
			// Username is a username to login with the registry server
			// TODO: think how to best check for defined values?
			Username: runner.ExitWithoutEnv("REGISTRY_USERNAME"),
			// SecretFile is a registry password
			SecretFile: runner.ExitWithoutEnv("REGISTRY_SECRET"),
		})).
		// specify process name
		Name("force-ci").
		// Watch events on the channel
		Watch(github.PullRequests(github.Source{
			// Repo is a repository to watch
			Repo: "gravitational/force",
			// Default branch match pattern is master
			BranchPattern: "^master|branch/.*$",
			// Approval configures an approval flow
			Approval: github.Approval{
				// Requies sets the approval as required
				Required: true,
				// Teams is a list of github teams who can approve PR test
				// or auto trigger pull request if they submit it.
				Teams: []string{"gravitational/devc", "gravitational/admins"},
			},
		})).
		// Run those actions
		Run(github.PostStatusOf(func(ctx force.ExecutionContext) error {
			event, ok := ctx.Event().(*github.PullRequestEvent)
			if !ok {
				return trace.BadParameter("unexpected event type: %T", ctx.Event())
			}
			// Create temporary directory "repo"
			repo, err := ioutil.TempDir("", "")
			if err != nil {
				return err
			}
			defer os.RemoveAll(repo)

			// Clone clones git repository into temp dir
			err = git.Clone(ctx, git.Repo{
				URL:  "git@github.com:gravitational/force.git",
				Into: repo,
				// Commit is a commit variable defined by pull request watch,
				// the problem is that there is no namespacing here
				Hash: event.Commit,
			})

			if err != nil {
				return err
			}

			// Image is an image name to build
			image := fmt.Sprintf(`gcr.io/kubeadm-167321/example:%v`, event.Commit)
			// Runtime is a go runtime to build
			goRuntime := "go1.12.1"
			// Build builds dockerfile and tags it in the local storage
			err = builder.Build(ctx, builder.Image{
				// Set build context to the cloned repository
				Context: repo,
				// Dockerfile is a dockerfile to build (from current dir),
				Dockerfile: "./Dockerfile",
				// Tag is the tag to build - here, as you see, we need to reimplement
				// Sprintf and every other method that works with our vars
				Tag: image,
				// Secrets are build secrets exposed to docker
				// container during the run
				Secrets: []builder.Secret{
					{
						ID:   "logging-creds",
						File: loggingCredentials,
					},
				},
				// Args are build arguments
				Args: []builder.Arg{
					{
						// FORCE_ID is a force run ID
						Key: "GO_RUNTIME",
						Val: goRuntime,
					},
				},
			})

			// Push the built image
			err = builder.Push(ctx, builder.Image{Tag: image})
			if err != nil {
				return err
			}
			// Prune the build cache
			builder.Prune(ctx)
			return nil
		}))
}
