# Processes, Channels and Actions

`force` is a command line tool that processes `G` files.

Every `G` file starts with a `Process(Spec{})` section that specifies one, or several `Run`
actions. Each `Run` action is triggered by events received over various channels.

Here is the process `watch-and-build`:

{go * ./examples/watch/G}

`Watch: Files("*.go")` specifies the channel that continuosly watches file
changes matching the glob `*.go` and generates events.

Every time the event is generated, the `Command` action runs the shell
command `go install -mod=vendor -v github.com/gravitational/force/tool/force`.

## Syntax

Force uses the [Go language](https://golang.org) [grammar](https://golang.org/ref/spec),
but it does not fully implement the Go language specification. However functions and function
calls, variables and assignments use the same notation.

## Sequences

Force can execute sequences of actions triggered by a single event using the `func`
construct:

{go * ./examples/func/G}

## Parallel Actions

Force can run actions in parallel using the `Parallel` function:

{go * ./examples/parallel/G}

`Parallel` launches all actions in parallel and collects the results. It succeeds
when all actions succeed or fails if any of the actions fail.


## Variables

`G` scripts support immutable variables (i.e. variables that can't be changed
after being declared).

{go * ./examples/cleanup/G}

At the moment, only `string`, `bool` and `int` variables are supported.

## Conditionals

The `If` action runs another action, if the first predicate matches:

{go * ./examples/conditionals/G}

## Deferred actions

In the previous examples, a temporary directory was created but not removed.
To execute actions at the end of a sequence (regardless of whether
it was successfull or not) use the `Defer` action:


{go * ./examples/cleanup/cleanup.force}

## Lambda functions and includes

Force supports simple lambda functions. Lambda function definitions
look like variable declarations.

```go
GlobalLog := func(message string) {
    Infof("this is a message: %v", message)
}
```

To define this, or other global functions visible to the `G` script,
write the following in the `lib.force` file:

{go * ./examples/includes/lib.force}

Then, reference the function in the `G` file:

{go * ./examples/includes/G}

And run Force using the `--include` directive:

```bash
$ force --include=lib.force
```

## Exiting and Environment

Most of the time, Force scripts are running continuously. However
sometimes it is helpful to exit explicitly. The `Exit` action achieves that.
This action will cause Force to exit with a success or error status depending
on the last action it performed.

{go * ./examples/exit/G}

## Marshaling code

Sometimes you need to run part of a Force script remotely - for example inside a Kubernetes job,
or using SSH calls on a cloud server.

The `Marshal` action helps to marshal (and verify) code to a string variable:

{go * ./examples/marshal/G}

The code inside a `Marhsal` function is not evaluated by default, however it is possible
to pass variables to the remote call by using `Unquote`:

{go * ./examples/marshal/unquote.force}


The resulting code will evaluate the variable `localUser` and substitute
it in the code:

```go
func(){
    log.Infof("Caller: %v", "bob")
}
```

Here is a full example of sending code to another Force process for
execution:

{go * ./examples/marshal/rpc.force}

## Plugins and Setup

The Force language can be extended using plugins. A special `setup.force` file
can be placed alongside the main `G` file to setup a plugin:

{go * ./examples/plugins/setup.force}

`force` will auto detect the `setup.force` and apply its configuration:

```bash
$ force
Detected setup.force
```

The `--setup` flag can be used to specify a custom setup file location:

```bash
$ force --setup=../plugins/setup.force
```
