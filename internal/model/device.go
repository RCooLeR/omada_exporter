package model

type DevicesInterface interface {
	GetType() string
}

// Device stores the common inventory and telemetry fields shared by Omada devices.
type Device struct {
	//Labels
	Mac             string `json:"mac"`
	Type            string `json:"type"`
	Subtype         string `json:"subtype"`
	Model           string `json:"model"`
	ShowModel       string `json:"showModel"`
	Version         string `json:"version"`
	HwVersion       string `json:"hwVersion"`
	FirmwareVersion string `json:"firmwareVersion"`
	IP              string `json:"ip"`
	Name            string `json:"name"`
	Status          int8   `json:"status"`
	//Fields
	Uptime         float64 `json:"uptimeLong"`
	MemUtilization float64 `json:"memUtil"`
	CpuUtilization float64 `json:"cpuUtil"`
	NeedUpgrade    bool    `json:"needUpgrade"`
	Download       float64 `json:"download"`
	Upload         float64 `json:"upload"`
	RxRate         float64 `json:"rxRate"`
	TxRate         float64 `json:"txRate"`
	Temp           float64 `json:"temp"`
}

type DeviceInterface interface {
	GetType() string
	GetMac() string
	GetName() string
	GetSubtype() string
	GetModel() string
	GetShowModel() string
	GetVersion() string
	GetVersionWithUpgrade() string
	GetHwVersion() string
	GetFirmwareVersion() string
	GetIp() string
	GetStatus() string
	GetUptime() float64
	GetMemUtilization() float64
	GetCpuUtilization() float64
	GetNeedUpgrade() bool
	GetDownload() float64
	GetUpload() float64
	GetTemp() float64
}

// GetType returns the Omada device type used to distinguish concrete models.
func (s *Device) GetType() string { return s.Type }

// GetMac returns the device MAC address.
func (s *Device) GetMac() string { return s.Mac }

// GetName returns the device display name.
func (s *Device) GetName() string { return s.Name }

// GetSubtype returns the controller-specific device subtype.
func (s *Device) GetSubtype() string { return s.Subtype }

// GetModel returns the hardware model identifier.
func (s *Device) GetModel() string { return s.Model }

// GetShowModel returns the user-facing model name reported by Omada.
func (s *Device) GetShowModel() string { return s.ShowModel }

// GetVersion returns the primary device software version string.
func (s *Device) GetVersion() string { return s.Version }

// GetHwVersion returns the hardware revision string.
func (s *Device) GetHwVersion() string { return s.HwVersion }

// GetFirmwareVersion returns the firmware version string.
func (s *Device) GetFirmwareVersion() string { return s.FirmwareVersion }

// GetIp returns the management IP address.
func (s *Device) GetIp() string { return s.IP }

// GetStatus maps the numeric device status code to a human-readable state label.
func (s *Device) GetStatus() string {
	switch s.Status {
	case 0:
		return "Disconnected"
	case 1:
		return "Disconnected(Migrating)"
	case 10:
		return "Provisioning"
	case 11:
		return "Configuring"
	case 12:
		return "Upgrading"
	case 13:
		return "Rebooting"
	case 14:
		return "Connected"
	case 15:
		return "Connected(Wireless)"
	case 16:
		return "Connected(Migrating)"
	case 17:
		return "Connected(Wireless,Migrating)"
	case 20:
		return "Pending"
	case 21:
		return "Pending(Wireless)"
	case 22:
		return "Adopting"
	case 23:
		return "Adopting(Wireless)"
	case 24:
		return "Adopt Failed"
	case 25:
		return "Adopt Failed(Wireless)"
	case 26:
		return "Managed By Others"
	case 27:
		return "Managed By Others(Wireless)"
	case 30:
		return "Heartbeat Missed"
	case 31:
		return "Heartbeat Missed(Wireless)"
	case 32:
		return "Heartbeat Missed(Migrating)"
	case 33:
		return "Heartbeat Missed(Wireless,Migrating)"
	case 40:
		return "Isolated"
	case 41:
		return "Isolated(Migrating)"
	case 50:
		return "Slice Configuring"
	}
	return ""
}

// GetUptime returns the device uptime value reported by Omada.
func (s *Device) GetUptime() float64 { return s.Uptime }

// GetMemUtilization returns the current memory utilization percentage.
func (s *Device) GetMemUtilization() float64 { return s.MemUtilization }

// GetCpuUtilization returns the current CPU utilization percentage.
func (s *Device) GetCpuUtilization() float64 { return s.CpuUtilization }

// GetNeedUpgrade reports whether Omada marks the device as having an available upgrade.
func (s *Device) GetNeedUpgrade() bool { return s.NeedUpgrade }

// GetDownload returns the current downstream throughput value.
func (s *Device) GetDownload() float64 { return s.Download }

// GetUpload returns the current upstream throughput value.
func (s *Device) GetUpload() float64 { return s.Upload }

// GetTemp returns the current device temperature reading.
func (s *Device) GetTemp() float64 { return s.Temp }

// GetVersionWithUpgrade appends an upgrade marker when Omada reports a pending upgrade.
func (s *Device) GetVersionWithUpgrade() string {
	if s.NeedUpgrade {
		return s.Version + " ↑"
	}
	return s.Version
}
