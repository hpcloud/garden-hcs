package main

import (
	"flag"
	"os"
	"os/signal"
	"syscall"

	"github.com/cloudfoundry-incubator/cf-lager"
	"github.com/cloudfoundry-incubator/garden-windows/backend"
	"github.com/cloudfoundry-incubator/garden/server"
	"github.com/pivotal-golang/lager"
)

var containerGraceTime = flag.Duration(
	"containerGraceTime",
	0,
	"time after which to destroy idle containers",
)
var containerizerURL = flag.String(
	"containerizerURL",
	"http://127.0.0.1",
	"URL for the Containerizer container server",
)

func main() {
	defaultListNetwork := "tcp"
	defaultListAddr := "0.0.0.0:58008"

	if os.Getenv("PORT") != "" {
		defaultListNetwork = "tcp"
		defaultListAddr = "0.0.0.0:" + os.Getenv("PORT")
	}

	var listenNetwork = flag.String(
		"listenNetwork",
		defaultListNetwork,
		"how to listen on the address (unix, tcp, etc.)",
	)

	var listenAddr = flag.String(
		"listenAddr",
		defaultListAddr,
		"address to listen on",
	)

	cf_lager.AddFlags(flag.CommandLine)
	flag.Parse()

	logger, _ := cf_lager.New("garden-windows")
	logger.Info("Pelerinul e viu")

	prisonBackend, err := backend.NewPrisonBackend("c:\\workspace\\prison", logger)
	if err != nil {
		logger.Fatal("Server Failed to Start", err)
		os.Exit(1)
	}

	gardenServer := server.New(*listenNetwork, *listenAddr, *containerGraceTime, prisonBackend, logger)
	err = gardenServer.Start()
	if err != nil {
		logger.Fatal("Server Failed to Start", err)
		os.Exit(1)
	}

	logger.Info("started", lager.Data{
		"network": *listenNetwork,
		"addr":    *listenAddr,
	})

	signals := make(chan os.Signal, 1)

	go func() {
		<-signals
		gardenServer.Stop()
		os.Exit(0)
	}()

	signal.Notify(signals, syscall.SIGINT, syscall.SIGTERM, syscall.SIGHUP)
	select {}
}
