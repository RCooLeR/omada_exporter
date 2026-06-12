package webapi

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/RCooLeR/omada_exporter/internal/api"
	"github.com/RCooLeR/omada_exporter/internal/model"
	"github.com/rs/zerolog/log"
)

// GetDPIInsights returns cached DPI traffic insights for the configured window.
func (c *Client) GetDPIInsights(windowSeconds int) (*model.DPIInsights, error) {
	if windowSeconds <= 0 {
		windowSeconds = 86400
	}
	cacheKey := fmt.Sprintf("webapi:dpi:insights:%d", windowSeconds)
	return api.FetchCached(c.Client, cacheKey, func() (*model.DPIInsights, error) {
		return c.getDPIInsightsFresh(windowSeconds)
	})
}

func (c *Client) getDPIInsightsFresh(windowSeconds int) (*model.DPIInsights, error) {
	end := time.Now().Unix()
	start := end - int64(windowSeconds)

	overviewURL := fmt.Sprintf("%s/%s/api/v2/sites/%s/stat/dpi/overview?start=%d&end=%d", c.Config.Host, c.OmadaCID, c.SiteId, start, end)
	overview := dpiOverviewResponse{}
	if err := c.getWebAPIJSON(overviewURL, "DPI overview", &overview); err != nil {
		return nil, err
	}

	cardsURL := fmt.Sprintf("%s/%s/api/v2/sites/%s/stat/dpi/category/categoryCards?start=%d&end=%d&selectAll=true", c.Config.Host, c.OmadaCID, c.SiteId, start, end)
	cards := dpiCategoryCardsResponse{}
	if err := c.getWebAPIJSON(cardsURL, "DPI category cards", &cards); err != nil {
		return nil, err
	}

	insights := &model.DPIInsights{
		WindowSeconds: int64(windowSeconds),
		TotalTraffic:  overview.Result.TotalTraffic,
		Categories:    overview.Result.CategoryTraffics,
	}
	if len(insights.Categories) == 0 {
		insights.Categories = categoriesFromCards(cards.Result.CategoryCards)
	}

	for _, card := range cards.Result.CategoryCards {
		for _, app := range card.Applications {
			app.FamilyID = card.FamilyID
			app.FamilyName = card.FamilyName
			insights.Applications = append(insights.Applications, app)
		}
	}

	return insights, nil
}

func (c *Client) getWebAPIJSON(url, endpointName string, target any) error {
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return err
	}

	resp, err := c.MakeLoggedInRequest(req)
	if err != nil {
		return err
	}

	body, err := io.ReadAll(resp.Body)
	_ = resp.Body.Close()
	if err != nil {
		return err
	}

	log.Info().Msgf("Received data from %s endpoint", endpointName)
	log.Debug().Bytes("data", body).Msgf("Received data from %s endpoint", endpointName)

	if err := api.ValidateAPIResponse(body, endpointName); err != nil {
		return err
	}

	return json.Unmarshal(body, target)
}

func categoriesFromCards(cards []model.DPICategoryCard) []model.DPICategoryTraffic {
	categories := make([]model.DPICategoryTraffic, 0, len(cards))
	for _, card := range cards {
		categories = append(categories, model.DPICategoryTraffic{
			FamilyID:   card.FamilyID,
			FamilyName: card.FamilyName,
			Traffic:    card.TotalTraffic,
		})
	}
	return categories
}

type dpiOverviewResponse struct {
	Result struct {
		CategoryTraffics []model.DPICategoryTraffic `json:"categoryTraffics"`
		TotalTraffic     float64                    `json:"totalTraffic"`
	} `json:"result"`
}

type dpiCategoryCardsResponse struct {
	Result struct {
		CategoryCards []model.DPICategoryCard `json:"categoryCards"`
	} `json:"result"`
}
