package main

import (
	"os"

	"github.com/gravitational/force/pkg/runner"

	_ "github.com/gravitational/force/internal/unshare"
	"gopkg.in/alecthomas/kingpin.v2"
)

func main() {
	runner.Reexec()
	ctx := runner.SetupSignalHandlers()

	var debug bool
	app := kingpin.New("force", "Force is simple CI/CD tool")
	app.Flag("debug", "Turn on debugging level").Short('d').BoolVar(&debug)

	webC := app.Command("web", "Start a web server")
	webListenAddr := "127.0.0.1:8087"
	webC.Arg("listen-addr", "Web Server listen address").Default(webListenAddr).StringVar(&webListenAddr)

	var publishCfg publishConfig
	publishC := app.Command("publish", "Build and publish a force program. Run it from current force repo. force publish tool/force/main.go gcr.io/kubeadm-167321/force")
	publishC.Arg("program", "Path to directory existing go program, e.g. ./tool/force").Required().ExistingDirVar(&publishCfg.program)
	publishC.Arg("repo", "Set the repo to publish").Required().StringVar(&publishCfg.repo)
	publishC.Flag("runc", "Set runc version to build").Default("1.0.0-rc8").StringVar(&publishCfg.runcVer)
	publishC.Flag("go", "Set go version to build").Default("1.13.1").StringVar(&publishCfg.goVer)
	publishC.Flag("registry-server", "Docker registry server").Default("gcr.io").StringVar(&publishCfg.builderCfg.Server)
	publishC.Flag("registry-username", "Docker registry username").Envar("REGISTRY_USERNAME").StringVar(&publishCfg.builderCfg.Username)
	publishC.Flag("registry-secret-file", "Docker registry secret file").Envar("REGISTRY_SECRET").StringVar(&publishCfg.builderCfg.SecretFile)

	cmd, err := app.Parse(os.Args[1:])
	runner.ExitIf(err)

	err = runner.InitLogger(debug)
	runner.ExitIf(err)

	switch cmd {
	case webC.FullCommand():
		err := runWebServer(ctx, webListenAddr, "./fixtures/cert.pem", "./fixtures/key.pem")
		runner.ExitIf(err)
	case publishC.FullCommand():
		err := publish(ctx, publishCfg)
		runner.ExitIf(err)
	}
}
