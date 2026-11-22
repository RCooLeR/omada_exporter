package omada

import (
	"strings"
	"testing"

	"github.com/RCooLeR/omada_exporter/internal/config"
	"github.com/RCooLeR/omada_exporter/internal/omada/session"
	"github.com/RCooLeR/omada_exporter/internal/testsupport/mspfake"
)

func newMSPTestClient(t *testing.T, fake *mspfake.Controller) *session.Client {
	t.Helper()
	root, err := session.Configure(&config.Config{
		Host: fake.URL(), Username: fake.Username, Password: fake.Password,
		ClientId: fake.ClientID, SecretId: fake.ClientSecret, SystemType: config.SystemTypeAuto, Timeout: 5,
	})
	if err != nil {
		t.Fatal(err)
	}
	return root
}

func TestMSPCustomersPaginatesWithMSPID(t *testing.T) {
	fake := mspfake.New(t)
	fake.MaxPageSize = 2
	root := newMSPTestClient(t, fake)
	fake.ResetRequests()

	customers, err := NewClient(root).MSPCustomers()
	if err != nil {
		t.Fatal(err)
	}
	if len(customers) != 3 || customers[2].CustomerID != "cust-c" || customers[0].CustomerName != "Alpha Corp" {
		t.Fatalf("customers = %+v", customers)
	}
	pages := 0
	for _, r := range fake.Requests() {
		if r.Path == "/openapi/v1/msp/msp-1/customers" {
			pages++
		}
	}
	if pages != 2 {
		t.Fatalf("customer pages = %d, want 2", pages)
	}
}

func TestCustomerSitesUsesCustomerID(t *testing.T) {
	fake := mspfake.New(t)
	fake.MaxPageSize = 1
	root := newMSPTestClient(t, fake)

	if _, err := NewClient(root).CustomerSites(); err == nil {
		t.Fatal("site discovery on the MSP-level client must fail")
	}
	view, err := root.ForTarget("cust-a", "", "")
	if err != nil {
		t.Fatal(err)
	}
	fake.ResetRequests()
	sites, err := NewClient(view).CustomerSites()
	if err != nil {
		t.Fatal(err)
	}
	if len(sites) != 2 || sites[1].SiteID != "site-a2" || sites[1].Name != "Branch" {
		t.Fatalf("sites = %+v", sites)
	}
	for _, r := range fake.Requests() {
		if strings.HasPrefix(r.Path, "/openapi/v1/") && r.Path != "/openapi/v1/cust-a/sites" {
			t.Fatalf("unexpected request %s", r.Path)
		}
	}
	if len(fake.Requests()) != 2 {
		t.Fatalf("site pages = %d, want 2", len(fake.Requests()))
	}
}

func TestMSPDiscoveryRequiresMSPMode(t *testing.T) {
	if _, err := (&Client{session: &session.Client{Config: &config.Config{}}}).MSPCustomers(); err == nil {
		t.Fatal("MSPCustomers must fail outside MSP mode")
	}
}
