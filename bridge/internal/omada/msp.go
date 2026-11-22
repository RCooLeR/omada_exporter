package omada

import (
	"fmt"

	"github.com/RCooLeR/omada_exporter/internal/model"
)

// MSPMode reports whether the controller runs in MSP mode.
func (c *Client) MSPMode() bool {
	return c != nil && c.session != nil && c.session.MSPMode()
}

// MSPCustomers lists every customer of the MSP. It must be called on the
// root (MSP-level) client; the request path carries the MSP ID.
func (c *Client) MSPCustomers() ([]model.MSPCustomer, error) {
	if !c.MSPMode() {
		return nil, fmt.Errorf("customer discovery requires MSP mode")
	}
	if err := c.requireOpenAPICredentials(); err != nil {
		return nil, err
	}
	mspID := c.session.MSPID()
	urlTemplate := fmt.Sprintf("%s/openapi/v1/msp/%s/customers?page=%%d&pageSize=%%d", c.session.Config.Host, mspID)
	return c.fetchOpenAPIGrid[model.MSPCustomer]("MSP customers", urlTemplate)
}

// CustomerSites lists the sites of the customer this client is scoped to. It
// must be called on a customer view: /openapi/v1/{mspId}/sites is rejected
// with -1005, while /openapi/v1/{customerId}/sites works with the MSP token.
func (c *Client) CustomerSites() ([]model.MSPSite, error) {
	if !c.MSPMode() {
		return nil, fmt.Errorf("site discovery requires MSP mode")
	}
	if err := c.requireOpenAPICredentials(); err != nil {
		return nil, err
	}
	customerID, _ := c.ContextIDs()
	if customerID == "" || customerID == c.session.MSPID() {
		return nil, fmt.Errorf("site discovery requires a customer-scoped client")
	}
	urlTemplate := fmt.Sprintf("%s/openapi/v1/%s/sites?page=%%d&pageSize=%%d", c.session.Config.Host, customerID)
	return c.fetchOpenAPIGrid[model.MSPSite]("customer sites", urlTemplate)
}
