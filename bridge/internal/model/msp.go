package model

// MSPCustomer is one entry of /openapi/v1/msp/{mspId}/customers.
type MSPCustomer struct {
	CustomerID   string `json:"customerId"`
	CustomerName string `json:"customerName"`
}

// MSPSite is one entry of /openapi/v1/{customerId}/sites.
type MSPSite struct {
	SiteID string `json:"siteId"`
	Name   string `json:"name"`
}
