package main

import (
	"flag"
	"os"
	"os/signal"
	"syscall"

	"github.com/cloudfoundry-incubator/cf-lager"
	"github.com/cloudfoundry-incubator/garden/server"
	"github.com/pivotal-golang/lager"

	"github.com/cloudfoundry-incubator/garden-windows/backend"
)

var containerGraceTime = flag.Duration(
	"containerGraceTime",
	0,
	"time after which to destroy idle containers",
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

	var cellIP = flag.String(
		"cellIP",
		"127.0.0.1",
		"IP address of the current cell, as exposed to the router",
	)

	var imageRepositoryLocation = flag.String(
		"containersLocation",
		"c:\\garden-windows\\containers",
		"Location where container images and other artifacts will be stored.",
	)

	cf_lager.AddFlags(flag.CommandLine)
	flag.Parse()

	logger, _ := cf_lager.New("garden-windows")
	logger.Info("Garden Windows started.")

	windowsContainerBackend, err := backend.NewWindowsContainerBackend(*imageRepositoryLocation, logger, *cellIP)
	if err != nil {
		logger.Fatal("Server Failed to Start", err)
		os.Exit(1)
	}

	gardenServer := server.New(*listenNetwork, *listenAddr, *containerGraceTime, windowsContainerBackend, logger)
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

		logger.Info("Garden Windows stopped")

		os.Exit(0)
	}()

	signal.Notify(signals, syscall.SIGINT, syscall.SIGTERM, syscall.SIGHUP)
	select {}
}
