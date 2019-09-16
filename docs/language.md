# Processes, Channels and Actions

`force` is a command line tool that processes `G` files.

Every `G` file starts with a `Process(Spec{})` section that specifies one or several `Run`
actions triggered by events received by channel specified in the `Watch` section.

Here is the process `watch-and-build`:

{go * ./examples/watch/G}

`Watch: Files("*.go")` specifies the channel that continuosly watches file
changes matching the glob `*.go` and generates events.

Every time the event is generated, the `Command` action runs the shell
command `go install -mod=vendor -v github.com/gravitational/force/tool/force`.

## Syntax

Force users [Go language](https://golang.org) language [grammar](https://golang.org/ref/spec),
however it does not fully implement golang language specification, however
functions and function calls, variables and assignments use the same notation.

## Sequences

Force can execute sequences of actions triggered by single event using `func`
construct:

{go * ./examples/func/G}

## Parallel Actions

Force can run actions in parallel using `Parallel` functon:

{go * ./examples/parallel/G}

`Parallel` launches all actions in parallel, collects the results. It succeeds
when all actions succeed or fails if any of the actions fail.


## Variables

`G` scripts support immutable variables (the ones that after declared,
can't be changed).

{go * ./examples/cleanup/G}

At the moment, only `string`, `bool` and `int` variables are supported.

## Deferred actions

In the previous examples, temporary directory was created but not removed.
To execute actions at the end of the sequence (regardless of whether
it was successfull or not) use the `Defer` action:


{go * ./examples/cleanup/cleanup.force}

## Lambda functions and includes

Force supports simple lambda functions. Lambda function definition
looks like a variable declaration.

```go
GlobalLog := func(message string) {
    Infof("this is a message: %v", message)
}
```

To define this or other global functions visible to the main force script,
in file `lib.force`, write:

{go * ./examples/includes/lib.force}

Reference the function in the `G` file:

{go * ./examples/includes/G}

Run the force using the `--include` directive:

```bash
$ force --include=lib.force
```

## Exiting and Environment

Most of the time force scripts are running continuosly, however
sometimes it is helpful to exit, use `Exit` action. This action
will cause force progam to exit with the success or error depending
on whether the previous last action has failed or succeeded.

{go * ./examples/exit/G}

## Marshaling code

Sometimes you need to run a part of the force script remotely - inside a kubernetes job,
or using SSH call on the cloud server.

`Marshal` action helps to marshal (and verify) the code to a string variable:

{go * ./examples/marshal/G}

The code inside `Marhsal` function is not evaluated by default, however it is possible
to pass variables to the remote call using `Unquote`:

{go * ./examples/marshal/unquote.force}


The resulting code will evaluate the variable `localUser` and substitute
it in the code:

```go
func(){
    log.Infof("Caller: %v", "bob")
}
```

Here is a full example of sending code to another force process for
execution:

{go * ./examples/marshal/rpc.force}

## Plugins

Force language is extended using plugins system. Special `setup.force` file
could be placed alongside `G` file to setup a plugin:

{go * ./examples/plugins/setup.force}
