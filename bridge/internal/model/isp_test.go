package model

import "testing"

func TestIspGetGatewayStatusUsesGatewayStatus(t *testing.T) {
	isp := Isp{Status: 0, GatewayStatus: 1}
	if got := isp.GetGatewayStatus(); got != "Online" {
		t.Fatalf("GetGatewayStatus() = %q, want Online", got)
	}

	isp.Status = 1
	isp.GatewayStatus = 0
	if got := isp.GetGatewayStatus(); got != "Offline" {
		t.Fatalf("GetGatewayStatus() = %q, want Offline", got)
	}
}
