package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"log"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"

	"code.google.com/p/go-uuid/uuid"
	"github.com/Microsoft/hcsshim"
	"github.com/cloudfoundry-incubator/cf-lager"
	"github.com/cloudfoundry-incubator/garden/server"
	"github.com/pivotal-golang/lager"

	"github.com/cloudfoundry-incubator/garden-windows/backend"
	"github.com/cloudfoundry-incubator/garden-windows/windows_containers"
)

var containerGraceTime = flag.Duration(
	"containerGraceTime",
	0,
	"time after which to destroy idle containers",
)

//func dir(id string) string {
//	return filepath.Join(d.info.HomeDir, filepath.Base(id))
//}

//func resolveId(id string) (string, error) {
//	content, err := ioutil.ReadFile(filepath.Join(d.dir(id), "layerId"))
//	if os.IsNotExist(err) {
//		return id, nil
//	} else if err != nil {
//		return "", err
//	}
//	return string(content), nil
//}

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

	cf_lager.AddFlags(flag.CommandLine)
	flag.Parse()

	logger, _ := cf_lager.New("garden-windows")
	logger.Info("Garden Windows started.", lager.Data{
		"info": filepath.IsAbs("c:\\windows\\system32\\"),
	})

	// hacking code until return

	imageRepositoryLocation := "c:\\garden-windows\\containers"
	sharedBaseImageName := "WindowsServerCore"
	driverInfo := windows_containers.NewDriverInfo(imageRepositoryLocation)
	containerId := uuid.New()

	b, err := backend.NewWindowsContainerBackend(imageRepositoryLocation, logger, *cellIP)
	if err != nil {
		log.Fatal(err)
	}

	sharedBaseImage, err := windows_containers.GetSharedBaseImageByName(sharedBaseImageName)
	if err != nil {
		log.Fatal(err)
	}
	log.Println(sharedBaseImage.GetId())
	log.Println(sharedBaseImage.Path)

	baseImageMountPath, err := hcsshim.GetLayerMountPath(driverInfo, sharedBaseImage.GetId())
	if err != nil {
		log.Fatal(err)
	}
	log.Println(baseImageMountPath)

	layerChain := []string{baseImageMountPath}

	// Try to cleanup after ourselves
	defer func() {
		hcsshim.ShutdownComputeSystem(containerId)
		hcsshim.TerminateComputeSystem(containerId)
		hcsshim.DeactivateLayer(driverInfo, containerId)
		hcsshim.DestroyLayer(driverInfo, containerId)
	}()

	err = hcsshim.CreateSandboxLayer(driverInfo, containerId, layerChain[0], layerChain)
	if err != nil {
		log.Fatal(err)
	}

	rootfs, err := b.GetRootFSForID(containerId, layerChain)

	layerFolderPath := b.Dir(containerId)
	log.Println(layerFolderPath)

	cu := &windows_containers.ContainerInit{
		SystemType:              "Container",
		Name:                    containerId,
		Owner:                   windows_containers.DefaultOwner,
		IsDummy:                 false,
		VolumePath:              rootfs,          //c.Rootfs,
		IgnoreFlushesDuringBoot: true,            //c.FirstStart,
		LayerFolderPath:         layerFolderPath, //c.LayerFolder,
		Layers:                  []windows_containers.Layer{},
	}

	for i := 0; i < len(layerChain); i++ {
		_, filename := filepath.Split(layerChain[i])
		g, err := hcsshim.NameToGuid(filename)
		if err != nil {
			log.Fatal(err)
		}
		cu.Layers = append(cu.Layers, windows_containers.Layer{
			ID:   g.ToString(),
			Path: layerChain[i],
		})
	}

	configurationb, err := json.Marshal(cu)
	if err != nil {
		log.Fatal(err)
	}

	configuration := string(configurationb)

	err = hcsshim.CreateComputeSystem(containerId, configuration)

	if err != nil {
		log.Fatal(err)
	}

	err = hcsshim.StartComputeSystem(containerId)
	if err != nil {
		log.Fatal(err)
	}

	createProcessParms := hcsshim.CreateProcessParams{
		EmulateConsole:   false,
		WorkingDirectory: "c:\\",
		ConsoleSize:      [2]int{80, 120},
		CommandLine:      "cmd /c dir",
	}

	pid, _, stdout, _, err := hcsshim.CreateProcessInComputeSystem(containerId, false, true, false, createProcessParms)
	if err != nil {
		log.Fatal(err)
	}
	log.Println(pid)

	buf := new(bytes.Buffer)
	buf.ReadFrom(stdout)

	exitCode, err := hcsshim.WaitForProcessInComputeSystem(containerId, pid)
	if err != nil {
		log.Fatal(err)
	}

	log.Println(exitCode)

	s := buf.String()
	log.Println(s)

	return

	windowsContainerBackend, err := backend.NewWindowsContainerBackend("c:\\workspace\\prison", logger, *cellIP)
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
