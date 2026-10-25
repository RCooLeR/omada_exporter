package omada

import (
	"fmt"
	"net/url"
	"strconv"

	"github.com/RCooLeR/omada_exporter/internal/omada/session"
)

// These are IntelliRecover's read-only candidate lists, not its monitoring or
// reboot APIs. The contract was inspected in standard 6.3.0.45 and Fusion
// 6.3.14.1 frontends. Neither enabling monitoring nor editing PoE is necessary.
type poeEndpoint struct {
	MAC          string `json:"mac"`
	Name         string `json:"name"`
	SiteID       string `json:"siteId"`
	Eligible     bool   `json:"eligibleFlag"`
	ClientFlag   *bool  `json:"clientFlag"`
	DeviceType   int    `json:"deviceType"`
	UplinkDevice struct {
		MAC        string `json:"mac"`
		Type       string `json:"type"`
		UplinkType *int   `json:"uplinkType"`
		Port       int    `json:"uplinkPort"`
		IsStack    bool   `json:"isStack"`
		StackID    string `json:"stackId"`
		Unit       *int   `json:"unit"`
	} `json:"uplinkDevice"`
	fromClientList bool
}

type poePortKey struct {
	mac  string
	port int
}

func bindPoEEndpoints(s *session.WebControlSession, targets []ControlTarget) error {
	if len(targets) == 0 {
		return nil
	}
	devices, err := readPoEEndpoints(s, false)
	if err != nil {
		return err
	}
	clients, err := readPoEEndpoints(s, true)
	if err != nil {
		return err
	}
	endpoints := append(devices, clients...)
	portCounts := map[poePortKey]int{}
	macCounts := map[string]int{}
	bindings := map[poePortKey]poeEndpoint{}
	for _, endpoint := range endpoints {
		if endpoint.SiteID != "" && endpoint.SiteID != s.SiteID {
			return fmt.Errorf("PoE eligibility returned a different site")
		}
		uplink := endpoint.UplinkDevice
		ownerMAC, err := CanonicalControlMAC(uplink.MAC)
		if err != nil || uplink.Port < 1 || uplink.Port > 255 {
			continue
		}
		key := poePortKey{ownerMAC, uplink.Port}
		// Count even ineligible rows: a second endpoint attributed to this port
		// makes a one-device shortcut ambiguous, regardless of its eligibility.
		portCounts[key]++
		endpoint.MAC, err = CanonicalControlMAC(endpoint.MAC)
		if err != nil {
			continue
		}
		macCounts[endpoint.MAC]++
		if !endpoint.Eligible || endpoint.ClientFlag == nil || *endpoint.ClientFlag != endpoint.fromClientList || endpoint.MAC == ownerMAC ||
			uplink.UplinkType == nil || *uplink.UplinkType != 0 || uplink.IsStack || uplink.StackID != "" || uplink.Unit != nil {
			continue
		}
		if (!endpoint.fromClientList && endpoint.DeviceType != 3) || (endpoint.fromClientList && endpoint.DeviceType != 107 && endpoint.DeviceType != 4) {
			continue
		}
		bindings[key] = endpoint
	}
	// Do not modify any target until both paginated inventories are complete.
	for i := range targets {
		target := &targets[i]
		key := poePortKey{target.MAC, target.Port}
		endpoint, ok := bindings[key]
		if !ok || portCounts[key] != 1 || macCounts[endpoint.MAC] != 1 || endpoint.UplinkDevice.Type != target.DeviceType {
			continue
		}
		target.PoweredDeviceMAC, target.PoweredDeviceName = endpoint.MAC, endpoint.Name
	}
	return nil
}

func readPoEEndpoints(s *session.WebControlSession, clients bool) ([]poeEndpoint, error) {
	path := "/monitor-device/eligible-device-grids"
	query := url.Values{"filters.status": {"14,15,16,17"}, "filters.deviceType": {"3"}}
	if clients {
		path = "/monitor-client/eligible-client-grids"
		query = url.Values{"filters.active": {"true"}, "filters.wireless": {"false"}}
	}
	query.Set("currentPageSize", strconv.Itoa(controlClientPageSize))
	var endpoints []poeEndpoint
	total := -1
	for page := 1; page <= controlMaxClientPages; page++ {
		query.Set("currentPage", strconv.Itoa(page))
		var response struct {
			Result struct {
				TotalRows   *int          `json:"totalRows"`
				CurrentPage *int          `json:"currentPage"`
				Data        []poeEndpoint `json:"data"`
			} `json:"result"`
		}
		if err := s.ReadJSON(path+"?"+query.Encode(), &response); err != nil {
			return nil, err
		}
		result := response.Result
		if result.TotalRows == nil || *result.TotalRows < 0 || *result.TotalRows > controlMaxClientPages*controlClientPageSize ||
			result.CurrentPage == nil || *result.CurrentPage != page || (total >= 0 && total != *result.TotalRows) {
			return nil, fmt.Errorf("PoE eligibility pagination is missing or changed")
		}
		total = *result.TotalRows
		for _, endpoint := range result.Data {
			endpoint.fromClientList = clients
			endpoints = append(endpoints, endpoint)
		}
		if len(endpoints) > total || (len(result.Data) == 0 && len(endpoints) < total) {
			return nil, fmt.Errorf("PoE eligibility inventory is incomplete or inconsistent")
		}
		if len(endpoints) == total {
			return endpoints, nil
		}
	}
	return nil, fmt.Errorf("PoE eligibility exceeds pagination limit")
}
