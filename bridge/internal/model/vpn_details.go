package model

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
)

// StringList accepts the string, array, or object-array shapes Omada uses for
// IP/network detail fields and exposes them as a stable comma-separated label.
type StringList []string

// UnmarshalJSON decodes a flexible list of strings from Omada API responses.
func (l *StringList) UnmarshalJSON(data []byte) error {
	if string(data) == "null" {
		return nil
	}

	var single string
	if err := json.Unmarshal(data, &single); err == nil {
		*l = appendStringValue((*l)[:0], single)
		return nil
	}

	var values []any
	if err := json.Unmarshal(data, &values); err == nil {
		*l = appendAnyValues((*l)[:0], values...)
		return nil
	}

	var object map[string]any
	if err := json.Unmarshal(data, &object); err == nil {
		*l = appendAnyValues((*l)[:0], object)
		return nil
	}

	return nil
}

// String returns the list in a compact, stable format for Prometheus labels.
func (l StringList) String() string {
	return strings.Join(uniqueNonEmptyStrings(l), ",")
}

// VPNDetailLabels contains optional VPN IP/network fields surfaced to HA as attributes.
type VPNDetailLabels struct {
	LocalIP        string
	LocalNetworks  string
	RemoteNetworks string
	AllowedIPs     string
	Endpoint       string
	EndpointIP     string
}

// Values returns the labels in the collector descriptor order.
func (d VPNDetailLabels) Values() []string {
	return []string{d.LocalIP, d.LocalNetworks, d.RemoteNetworks, d.AllowedIPs, d.Endpoint, d.EndpointIP}
}

// VPNDetailLabelNames returns optional VPN detail label names shared by collectors.
func VPNDetailLabelNames() []string {
	return []string{"local_ip", "local_networks", "remote_networks", "allowed_ips", "endpoint", "endpoint_ip"}
}

func vpnDetailLabels(localIP string, localNetworks, remoteNetworks, allowedIPs []StringList, endpoint, endpointIP string) VPNDetailLabels {
	return VPNDetailLabels{
		LocalIP:        strings.TrimSpace(localIP),
		LocalNetworks:  joinStringLists(localNetworks...),
		RemoteNetworks: joinStringLists(remoteNetworks...),
		AllowedIPs:     joinStringLists(allowedIPs...),
		Endpoint:       strings.TrimSpace(endpoint),
		EndpointIP:     strings.TrimSpace(endpointIP),
	}
}

func joinStringLists(lists ...StringList) string {
	values := []string{}
	for _, list := range lists {
		values = append(values, list...)
	}
	return strings.Join(uniqueNonEmptyStrings(values), ",")
}

func uniqueNonEmptyStrings(values []string) []string {
	seen := map[string]struct{}{}
	result := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if _, exists := seen[value]; exists {
			continue
		}
		seen[value] = struct{}{}
		result = append(result, value)
	}
	sort.Strings(result)
	return result
}

func appendAnyValues(values []string, items ...any) []string {
	for _, item := range items {
		switch typed := item.(type) {
		case string:
			values = appendStringValue(values, typed)
		case float64:
			values = appendStringValue(values, fmt.Sprintf("%g", typed))
		case map[string]any:
			values = appendObjectValue(values, typed)
		case []any:
			values = appendAnyValues(values, typed...)
		}
	}
	return values
}

func appendObjectValue(values []string, object map[string]any) []string {
	keys := []string{"cidr", "network", "subnet", "ip", "address", "value", "name"}
	for _, key := range keys {
		if value, ok := object[key]; ok {
			values = appendAnyValues(values, value)
			return values
		}
	}

	return values
}

func appendStringValue(values []string, value string) []string {
	for _, part := range strings.FieldsFunc(value, func(r rune) bool {
		return r == ',' || r == ';' || r == '\n' || r == '\r' || r == '\t'
	}) {
		part = strings.TrimSpace(part)
		if part != "" {
			values = append(values, part)
		}
	}
	return values
}
