package config

import (
	"slices"
	"testing"
)

func TestMSPFilters(t *testing.T) {
	cases := []struct {
		value string
		want  []string
	}{
		{"", nil},
		{"all", nil},
		{" ALL ", nil},
		{"Alpha, cust-b ,,", []string{"Alpha", "cust-b"}},
		{"Alpha,all", nil},
	}
	for _, tc := range cases {
		if got := (&Config{MSPCustomers: tc.value}).MSPCustomerFilter(); !slices.Equal(got, tc.want) {
			t.Errorf("customers %q = %v, want %v", tc.value, got, tc.want)
		}
	}
	if got := (&Config{Site: "Default"}).MSPSiteFilter(); got != nil {
		t.Errorf("implicit default site must not filter, got %v", got)
	}
	if got := (&Config{Site: "*", SiteSet: true}).MSPSiteFilter(); got != nil {
		t.Errorf("* must not filter, got %v", got)
	}
	if got := (&Config{Site: "Default,HQ", SiteSet: true}).MSPSiteFilter(); !slices.Equal(got, []string{"Default", "HQ"}) {
		t.Errorf("site filter = %v", got)
	}
}

func TestValidateMSP(t *testing.T) {
	valid := Config{ClientId: "id", SecretId: "secret", OpenAPIAuth: OpenAPIAuthAuto}
	if err := ValidateMSP(&valid); err != nil {
		t.Fatalf("valid config: %v", err)
	}
	for name, mutate := range map[string]func(*Config){
		"missing secret":  func(c *Config) { c.SecretId = "" },
		"disabled":        func(c *Config) { c.OpenAPIAuth = OpenAPIAuthDisabled },
		"web session":     func(c *Config) { c.OpenAPIAuth = OpenAPIAuthWebSession },
		"mqtt":            func(c *Config) { c.MQTTEnabled = true },
		"device control":  func(c *Config) { c.AllowDeviceControl = true },
		"negative slots":  func(c *Config) { c.MSPScrapeConcurrency = -1 },
		"negative period": func(c *Config) { c.MSPDiscoveryInterval = -1 },
	} {
		conf := valid
		mutate(&conf)
		if ValidateMSP(&conf) == nil {
			t.Errorf("%s: expected an error", name)
		}
	}
}
