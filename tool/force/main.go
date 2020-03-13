package main

import (
	"fmt"
	"os"

	"github.com/gravitational/force/pkg/runner"

	"github.com/gravitational/trace"
	"gopkg.in/alecthomas/kingpin.v2"
)

func main() {
	ctx := runner.SetupSignalHandlers()

	var debug bool
	app := kingpin.New("force", "Force is simple CI/CD tool")
	app.Flag("debug", "Turn on debugging level").Short('d').BoolVar(&debug)

	webC := app.Command("web", "Start a web server")
	webListenAddr := "127.0.0.1:8087"
	webC.Arg("listen-addr", "Web Server listen address").Default(webListenAddr).StringVar(&webListenAddr)

	cmd, err := app.Parse(os.Args[1:])
	if err != nil {
		fmt.Printf("ERROR: %v", err)
		os.Exit(1)
	}

	if err := runner.InitLogger(debug); err != nil {
		fmt.Printf("Failed to init logger: %v", err)
		os.Exit(1)
	}

	switch cmd {
	case webC.FullCommand():
		err := runWebServer(ctx, webListenAddr, "./fixtures/cert.pem", "./fixtures/key.pem")
		if err != nil {
			if trace.IsDebug() {
				fmt.Fprintln(os.Stderr, trace.DebugReport(err))
			} else {
				fmt.Fprintln(os.Stderr, err.Error())
			}
			os.Exit(1)
		}
		return
	}
}
