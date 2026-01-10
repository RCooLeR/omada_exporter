package model

type DevicesInterface interface {
	GetType() string
}
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
}

func (s *Device) GetType() string            { return s.Type }
func (s *Device) GetMac() string             { return s.Mac }
func (s *Device) GetName() string            { return s.Name }
func (s *Device) GetSubtype() string         { return s.Subtype }
func (s *Device) GetModel() string           { return s.Model }
func (s *Device) GetShowModel() string       { return s.ShowModel }
func (s *Device) GetVersion() string         { return s.Version }
func (s *Device) GetHwVersion() string       { return s.HwVersion }
func (s *Device) GetFirmwareVersion() string { return s.FirmwareVersion }
func (s *Device) GetIp() string              { return s.IP }
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
func (s *Device) GetUptime() float64         { return s.Uptime }
func (s *Device) GetMemUtilization() float64 { return s.MemUtilization }
func (s *Device) GetCpuUtilization() float64 { return s.CpuUtilization }
func (s *Device) GetNeedUpgrade() bool       { return s.NeedUpgrade }
func (s *Device) GetDownload() float64       { return s.Download }
func (s *Device) GetUpload() float64         { return s.Upload }
func (s *Device) GetVersionWithUpgrade() string {
	if s.NeedUpgrade {
		return s.Version + " ↑"
	}
	return s.Version
}
