# Quickstart

**Hello, world!**

`force` is a command line tool that processes `G` files.

Create your first `Hello, world!` file named `G`:

```go
Process(Spec{
    Run: Command(`echo "hello, world!"`),
})
```

To process the file, run `force`:

```bash
$ force
INFO             Process planet-1 triggered by Oneshot(time=2019-09-15 23:37:37.181326936 +0000 UTC)
INFO [PLANET-1]  hello, world! id:48c69283 proc:planet-1
INFO [PLANET-1]  Process planet-1 completed successfully in 1.086956ms. id:48c69283 proc:planet-1
```

To exit `force` type `Ctrl-C`.

Every `G` file starts with a `Process(Spec{})` section that specifies one or several `Run`
actions triggered by events received by channels.

Our example did not specify any event, so force used a default `Oneshot()` channel.

The example above is equivalent to:

```go
Process(Spec{
    Watch: Oneshot(),
    Run: Command(`echo "hello, world!"`),
})
```

