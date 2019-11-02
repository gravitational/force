// TODO(klizhentas) how to define global inputs and use them?
input := Input("name", "Value")

// watch a github PR and trigger a build process
Process(Spec{
	Name: "build",
	Watch: Github("github.com/gravitational/teleport"),
	Run: Build("quay.io/gravitational/teleport"),
})

// should be possible to trigger this process manually using CLI
// (assuming force.go is in the current dir)
// 
// $ force build
// 
// or, if there is just one target
// 
// $ force

// watch a github PR and trigger a shell script
Process(Spec{
	Watch: Github("github.com/gravitational/teleport"),
	Run: Shell(`
         git pull github.com/gravitational/teleport
         make install
    `),
})

// External event generator, it generates events
// every time someone sends a pull request to the master branch
pullRequest := GithubPR("github.com/gravitational/teleport")

// trigger two parallel processes based on the same PR
p1 := Process(Spec{
	Watch: pullRequest,	
	Run: Build("quay.io/gravitational/teleport"),
})

p2 := Process(Spec{
	Watch: pullRequest,
	Run: Slack("post notification", self.status()),
})

// Wait for two events and perform another action or trigger another process
Wait(p1, p2).Success(Event()).Failure(Process())

// Schedule a sequence of actions as a part of the same process
Process(Spec{
	Watch: pullRequest,
	Run: Sequence(
		Build("quay.io/gravitational/teleport"),
		Slack("post notification", self.status()),
	),
})

// Schedule two parallel actions as a part of the same process
Process(Spec{
	Watch: pullRequest,
	Run: Parallel(
		Build("quay.io/gravitational/teleport"),
		Slack("post notification", self.status()),
	).OnSuccess(broadcast),
})


// Run two process in parallel and trigger another process
// an event on success once both are completed
Wait(p1, p2)
