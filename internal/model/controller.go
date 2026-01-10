package model

type Controller struct {
	Name              string  `json:"name"`
	MacAddress        string  `json:"macAddress"`
	FirmwareVersion   string  `json:"firmwareVersion"`
	ControllerVersion string  `json:"controllerVersion"`
	Model             string  `json:"model"`
	Ip                string  `json:"ip"`
	Uptime            float64 `json:"upTime"`
	Storage           []struct {
		Name  string  `json:"name"`
		Total float64 `json:"totalStorage"`
		Used  float64 `json:"usedStorage"`
	} `json:"hwcStorage"`
	DeviceCapacity struct {
		AdoptedApNum             int  `json:"adoptedApNum"`
		ApCapacity               int  `json:"apCapacity"`
		AdoptedOswNum            int  `json:"adoptedOswNum"`
		OswCapacity              int  `json:"oswCapacity"`
		AdoptedOsgNum            int  `json:"adoptedOsgNum"`
		OsgCapacity              int  `json:"osgCapacity"`
		AdoptedOltNum            int  `json:"adoptedOltNum"`
		OltCapacity              int  `json:"oltCapacity"`
		ShareApAndSwitchCapacity bool `json:"shareApAndSwitchCapacity"`
		AdoptedApAndSwitchNum    int  `json:"adoptedApAndSwitchNum"`
		ApAndSwitchCapacity      int  `json:"apAndSwitchCapacity"`
	} `json:"deviceCapacity"`

	UpgradeList []ControllerUpdate `json:"upgradeList"`
}

type ControllerUpdate struct {
	Channel         int    `json:"channel"`
	UpdateAvailable bool   `json:"update"`
	LatestVersion   string `json:"latestVersion,omitempty"`
}

func (c *Controller) GetVersionWithUpgrade() string {
	version := c.ControllerVersion
	for _, upgrade := range c.UpgradeList {
		if upgrade.UpdateAvailable {
			version += " ↑ " + upgrade.GetChannel()
			return version
		}
	}
	return version
}
func (cu *ControllerUpdate) GetChannel() string {
	switch cu.Channel {
	case 0:
		return "Stable"
	case 1:
		return "Release Candidate"
	case 2:
		return "Beta"
	}
	return ""
}
