# Quickstart

**Hello, world!**

`force` is a command line tool that processes `.force` files.

Create your first script printing `Hello, world!` file named `hello.force`:

{go * ./docs/snippets/hello.force}

To process the file, run `force`:

```bash
$ force hello.force 
hello, world!
```

**Events**

To showcase `force's` ability to create event driven workflows, let's
create `ticker.force` that will print a tick every second:

{go * ./docs/snippets/ticker.force}

The script in `ticker.force` starts with a `Process(Spec{})` section. `Watch`
subscribes to event channel, in this case `Ticker` that generates events.

Every time the event is generated, one, or several `Run` actions are triggered:

```bash
# Press Ctrl-C to stop the script.
$ force ticker.force
Tick!
Tick!
```
