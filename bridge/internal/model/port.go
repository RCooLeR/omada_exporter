package model

// Port represents static configuration for a device port.
type Port struct {
	Port       int8       `json:"port"`
	MaxSpeed   int8       `json:"maxSpeed"`
	Name       string     `json:"name"`
	Type       int8       `json:"type"`
	Operation  string     `json:"operation"`
	PortStatus PortStatus `json:"portStatus"`
}

// GetMaxSpeed converts the encoded maximum port speed to Mbps.
func (p *Port) GetMaxSpeed() int32 {
	switch p.MaxSpeed {
	case 0:
		return 0
	case 1:
		return 10
	case 2:
		return 100
	case 3:
		return 1000
	case 4:
		return 2500
	case 5:
		return 10000
	case 6:
		return 5000
	case 7:
		return 25000
	case 8:
		return 100000
	case 9:
		return 40000
	}
	return 0
}

// GetType maps the numeric port type to a readable media label.
func (p *Port) GetType() string {
	switch p.Type {
	case 1:
		return "Copper"
	case 2:
		return "Combo"
	case 3:
		return "SFP"
	default:
		return "Unknown"
	}
}
