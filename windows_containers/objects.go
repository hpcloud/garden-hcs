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
