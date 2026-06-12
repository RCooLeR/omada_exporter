package model

// DPIInsights represents DPI traffic totals for a query window.
type DPIInsights struct {
	WindowSeconds int64
	TotalTraffic  float64
	Categories    []DPICategoryTraffic
	Applications  []DPIApplicationTraffic
}

// DPICategoryTraffic represents traffic attributed to one DPI category.
type DPICategoryTraffic struct {
	FamilyID   int     `json:"familyId"`
	FamilyName string  `json:"familyName"`
	Traffic    float64 `json:"traffic"`
}

// DPIApplicationTraffic represents traffic attributed to one DPI application.
type DPIApplicationTraffic struct {
	FamilyID        int
	FamilyName      string
	ApplicationID   int     `json:"applicationId"`
	ApplicationName string  `json:"applicationName"`
	Traffic         float64 `json:"traffic"`
}

// DPICategoryCard represents category traffic with application breakdowns.
type DPICategoryCard struct {
	FamilyID     int                     `json:"familyId"`
	FamilyName   string                  `json:"familyName"`
	TotalTraffic float64                 `json:"totalTraffic"`
	Applications []DPIApplicationTraffic `json:"applications"`
}
