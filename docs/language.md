# Processes, Channels and Actions

`force` is a command line tool that processes `G` files.

To specify a continuosuly running process `.force` file defines a `Process(Spec{})` section
that specifies a `Watch` and a `Run` sections:

* `Watch` section subscribes to a source of events - a `Channel`.
* `Run` section specifies actions that are triggered every time an event is received.

The process `watch-and-build` in the following example,
starts the process that recompiles `force` binary every time a file is changed:

{go * ./docs/snippets/watch.force}

`Watch: Files("*.go")` subscribes to a channel that continuosly watches file
changes matching the glob `*.go` and generates events.

Every time the event is generated, the `Command` action runs the shell
command `go install -mod=vendor -v github.com/gravitational/force/tool/force`.

## Syntax

Force uses the [Go language](https://golang.org) [grammar](https://golang.org/ref/spec),
but it does not fully implement the Go language specification. However functions and function
calls, variable declarations use the same notation.

## Types

The supported types are `string` (in "double quotes"), `int` as in `1,2,3` and
bool `true` and `false`.

Force also supports anonymous structs:

`struct{StringVar string, IntVar int, BoolVar bool}`

structs field names should start with upper case.

Force is a statically typed language, and will verify that types match.

## Sequences and functions

Force can execute sequences of actions triggered by a single event using the `func`
construct:

{go * ./docs/snippets/func.force}

## Parallel Actions

Force can run actions in parallel using the `Parallel` function:

{go * ./docs/snippets/parallel.force}

`Parallel` launches all actions in parallel and collects the results. It succeeds
when all actions succeed or fails if any of the actions fail.

## Variables

Force's scripts support immutable variables (i.e. variables that can't be changed
after being declared).

{go * ./docs/snippets/vars.force}

At the moment, only `string`, `bool` and `int` variables are supported, including
lists: `[]string`, `[]bool` and `[]int`.

## Deferred actions

In the previous example, a temporary directory was created but not removed.
To execute actions at the end of a sequence (regardless of whether
it was successfull or not) use the `Defer` function:

{go * ./docs/snippets/cleanup.force}

## Conditionals

The `If` function runs another if the first predicate matches:

{go * ./docs/snippets/if.force}

**Conditionals expressions**

`If` evaluates to one of it's expressions, it means that the result of the `If`
could be assigned to a variable:

{go * ./docs/snippets/ifvar.force}

## Lambda (inline) functions and includes

Force supports simple anonymous inline functions (convenionally called `lambda` functions).
Lambda function definitions look like variable declarations:

{go * ./docs/snippets/lambda.force}

**Struct parameters**

Functions can receive anonymous structs as parameters, to pass a no-name struct as an argument,
one can use `_` expression. This expression tells `force` to detect a struct type
from function signature and create a struct with appropriate field:

{go * ./docs/snippets/lambdastruct.force}

let's simplify our original `watch` example with `_` variable:

{go * ./docs/snippets/watch_.force}

**Function values**

In Force, functions do not have a return instruction, instead they are
expressions that evaluate to the last statement. In this example,
the function `messageFunc` evaluates to string variable:

{go * ./docs/snippets/lambdaexpr.force}

Let's rewrite `If` expression from the example above to use function
expressions:

{go * ./docs/snippets/ifexpr.force}

## Organizing code with includes

Let's create a file with library functions: `lib.force`:

{go * ./docs/snippets/include/lib.force}

Then, include and reference the function in the `main.force`:

{go * ./docs/snippets/include/main.force}

```bash
$ force main.force
this is a message: log this line
```

## Exiting and Environment

Most of the time, `.force` `Process` scripts are running continuously watching events and running actions:

However sometimes it is helpful to exit explicitly. The `Exit` action achieves that.
This action will cause Force to exit with a success or error status depending
on the last action it performed.

{go * ./docs/snippets/exit.force}

This is equivalent of using lambda function instead of `Process`:

{go * ./docs/snippets/exitshort.force}

## Distributed Execution using Marshal

Sometimes one needs to run part of a Force script remotely - for example inside a Kubernetes job,
or using SSH calls on a cloud server.

The `Marshal` function serializes and verifies code to a string variable:

{go * ./docs/snippets/marshal.force}

```bash
$ force marshal.force
INFO [PLANET-1]  "Code: func(){log.Infof(\"Hello, world!\")\n}"
```

The code inside a `Marhsal` function is not evaluated by default, however it is possible
to pass variables to the remote call by using `Unquote`:

{go * ./docs/snippets/unquote.force}

```bash
$ force unqote force
INFO [PLANET-1]  "Code: func(){log.Infof(\"Caller: %v\", \"alice\")\n}"
```

The resulting code will evaluate the variable `localUser` and substitute
it in the code. This is equivalent of marshaling the following code:

```go
func(){
    log.Infof("Caller: %v", "alice")
}
```

Here is a full example of sending code to another Force process for
execution:

{go * ./docs/snippets/rpc.force}

## Plugins and Setup

Force scripts can be extended using plugins. A special `setup.force` file
can be placed alongside the main `.force` file to setup a plugin:

{go * ./docs/snippets/setup.force}

Force auto detects the `setup.force` in the current directory
and applies its configuration:

```bash
$ force -d hello.force
Detected setup.force
```

The `--setup` flag can be used to specify a custom setup file location:

```bash
$ force --setup=../plugins/setup.force hello.force
```
