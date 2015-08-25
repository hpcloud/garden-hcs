package main

import (
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

	imageRepositoryLocation := "c:\\garden-windows\\mytest"
	sharedBaseImageName := "WindowsServerCore"
	driverInfo := windows_containers.NewDriverInfo(imageRepositoryLocation)
	layerId := "windows-garden-rootfs"

	sharedBaseImage, err := windows_containers.GetSharedBaseImageByName(sharedBaseImageName)

	if err != nil {
		log.Fatal(err)
	}

	mp, err := hcsshim.GetLayerMountPath(driverInfo, sharedBaseImage.GetId())
	if err != nil {
		log.Fatal(err)
	}

	log.Println(mp)

	err = hcsshim.CreateLayer(driverInfo, layerId, sharedBaseImage.GetId())
	if err != nil {
		log.Fatal(err)
	}

	wgrfsmp, err := hcsshim.GetLayerMountPath(driverInfo, layerId)
	if err != nil {
		log.Fatal(err)
	}
	log.Println(wgrfsmp)

	name := uuid.New()

	cu := &windows_containers.ContainerInit{
		SystemType:              "Container",
		Name:                    name,
		Owner:                   windows_containers.DefaultOwner,
		IsDummy:                 false,
		VolumePath:              `\\?\Volume{bd05d44f-0000-0000-0000-100000000000}\`, //c.Rootfs,
		IgnoreFlushesDuringBoot: true,                                                //c.FirstStart,
		LayerFolderPath:         mp,                                                  //c.LayerFolder,
	}

	//	configuration := fmt.Sprintf(configurationTemplate, name, mp, imageRepositoryLocation, layerId, mp)

	configurationb, err := json.Marshal(cu)
	if err != nil {
		log.Fatal(err)
	}

	configuration := string(configurationb)

	err = hcsshim.CreateComputeSystem(name, configuration)

	if err != nil {
		log.Fatal(err)
	}

	return

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
