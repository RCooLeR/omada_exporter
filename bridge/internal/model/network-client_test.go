package model

import (
	"encoding/json"
	"testing"
)

func TestNetworkClientUnmarshalAcceptsFusionCamelCaseFields(t *testing.T) {
	var client NetworkClient
	err := json.Unmarshal([]byte(`{
		"mac": "AA-BB-CC-DD-EE-FF",
		"name": "Laptop",
		"connectType": 2,
		"connectDevType": "gateway",
		"gatewayMac": "11-22-33-44-55-66",
		"gatewayName": "Fusion",
		"wifiMode": 9,
		"rxRate": 2400000000,
		"txRate": 2400000000
	}`), &client)
	if err != nil {
		t.Fatalf("json.Unmarshal() returned error: %v", err)
	}

	if client.ConnectType != 2 {
		t.Fatalf("ConnectType = %d, want 2", client.ConnectType)
	}
	if got := client.GetConnectType(); got != "wired user" {
		t.Fatalf("GetConnectType() = %q, want wired user", got)
	}
	if client.WifiMode != 9 {
		t.Fatalf("WifiMode = %d, want 9", client.WifiMode)
	}
	if got := client.GetWifiMode(); got != "802.11bea" {
		t.Fatalf("GetWifiMode() = %q, want 802.11bea", got)
	}
	if client.GatewayMac != "11-22-33-44-55-66" {
		t.Fatalf("GatewayMac = %q, want Fusion gateway MAC", client.GatewayMac)
	}
}
