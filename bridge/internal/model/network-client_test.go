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
	if got := client.GetWifiMode(); got != "" {
		t.Fatalf("GetWifiMode() = %q for wired client, want empty", got)
	}
	if client.GatewayMac != "11-22-33-44-55-66" {
		t.Fatalf("GatewayMac = %q, want Fusion gateway MAC", client.GatewayMac)
	}
}

func TestNetworkClientAttachmentAndWirelessLabels(t *testing.T) {
	wired := NetworkClient{Port: 1, LagId: 0, WifiMode: 0}
	if got := wired.GetAttachmentPort(); got != "1" {
		t.Fatalf("GetAttachmentPort() = %q, want 1", got)
	}
	if got := wired.GetAttachmentLagID(); got != "" {
		t.Fatalf("GetAttachmentLagID() = %q, want empty", got)
	}
	if got := wired.GetWifiMode(); got != "" {
		t.Fatalf("GetWifiMode() = %q for wired client, want empty", got)
	}

	lagged := NetworkClient{Port: 8, LagId: 3}
	if got := lagged.GetAttachmentLagID(); got != "3" {
		t.Fatalf("GetAttachmentLagID() = %q, want 3", got)
	}

	wireless := NetworkClient{Wireless: true, Port: 1, LagId: 3, WifiMode: 9}
	if got := wireless.GetWifiMode(); got != "802.11bea" {
		t.Fatalf("GetWifiMode() = %q, want 802.11bea", got)
	}
	if got := wireless.GetAttachmentPort(); got != "" {
		t.Fatalf("GetAttachmentPort() = %q for wireless client, want empty", got)
	}
	if got := wireless.GetAttachmentLagID(); got != "" {
		t.Fatalf("GetAttachmentLagID() = %q for wireless client, want empty", got)
	}
}
