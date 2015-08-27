package windows_containers

type SharedBaseImage struct {
	CreatedTime string `json:"CreatedTime"`
	Path        string `json:"Path"`
	Name        string `json:"Name"`
	Size        int    `json:"Size"`
}

type SharedBaseImages struct {
	Images []SharedBaseImage `json:"Images"`
}

// defaultContainerNAT is the default name of the container NAT device that is
// preconfigured on the server.
const DefaultContainerNAT = "ContainerNAT"

// defaultOwner is a tag passed to HCS to allow it to differentiate between
// container creator management stacks. We hard code "garden-windows" in the case
// of garden-windows.
const DefaultOwner = "garden-windows"

type Layer struct {
	ID   string
	Path string
}

type DefConfig struct {
	DefFile string
}

type PortBinding struct {
	Protocol     string
	InternalPort int
	ExternalPort int
}

type NatSettings struct {
	Name         string
	PortBindings []PortBinding
}

type NetworkConnection struct {
	NetworkName string
	// TODO Windows: Add Ip4Address string to this structure when hooked up in
	// docker CLI. This is present in the HCS JSON handler.
	EnableNat bool
	Nat       NatSettings
}
type networkSettings struct {
	MacAddress string
}

type Device struct {
	DeviceType string
	Connection interface{}
	Settings   interface{}
}

type ContainerInit struct {
	SystemType              string   // HCS requires this to be hard-coded to "Container"
	Name                    string   // Name of the container. We use the docker ID.
	Owner                   string   // The management platform that created this container
	IsDummy                 bool     // Used for development purposes.
	VolumePath              string   // Windows volume path for scratch space
	Devices                 []Device // Devices used by the container
	IgnoreFlushesDuringBoot bool     // Optimisation hint for container startup in Windows
	LayerFolderPath         string   // Where the layer folders are located
	Layers                  []Layer  // List of storage layers
}
