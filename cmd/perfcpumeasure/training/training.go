package training

type Training struct {
	// Site information
	SiteID uint64 `json:"siteid"` // Site identification
	Host   uint64 `json:"host"`   // Host identification

	// Training data
	Busy  uint `json:"busy"`  // Percent busy
	Units uint `json:"units"` // Units ran
}

type DiskMapper struct {
	// Site information
	SiteID uint64 `json:"siteid"` // Site identification
	Host   uint64 `json:"host"`   // Host identification

	// Disk mapping
	DeviceName string `json:"devicename"` // Device name, e.g. sda1
	MountPoint string `json:"mountpoint"` // Mount point, e.g. /replay/sda1
}
