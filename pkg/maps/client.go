package maps

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"time"
)

type GeocodedLocation struct {
	FormattedAddress string
	Latitude         float64
	Longitude        float64
	PlaceID          string
}

type MapsClient interface {
	Geocode(ctx context.Context, address string) (*GeocodedLocation, error)
	ReverseGeocode(ctx context.Context, latitude, longitude float64) (*GeocodedLocation, error)
}

type Client struct {
	apiKey  string
	enabled bool
	client  *http.Client
}

func NewClient(apiKey string, enabled bool, timeout time.Duration) *Client {
	if timeout <= 0 {
		timeout = 5 * time.Second
	}
	return &Client{apiKey: apiKey, enabled: enabled && apiKey != "", client: &http.Client{Timeout: timeout}}
}

func (c *Client) Geocode(ctx context.Context, address string) (*GeocodedLocation, error) {
	return c.lookup(ctx, url.Values{"address": {address}})
}

func (c *Client) ReverseGeocode(ctx context.Context, latitude, longitude float64) (*GeocodedLocation, error) {
	return c.lookup(ctx, url.Values{"latlng": {fmt.Sprintf("%f,%f", latitude, longitude)}})
}

func (c *Client) lookup(ctx context.Context, query url.Values) (*GeocodedLocation, error) {
	if !c.enabled {
		return nil, fmt.Errorf("Google Maps geocoding is disabled")
	}
	query.Set("key", c.apiKey)
	endpoint := "https://maps.googleapis.com/maps/api/geocode/json?" + query.Encode()
	var lastErr error
	for attempt := 0; attempt < 2; attempt++ {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
		if err != nil {
			return nil, err
		}
		resp, err := c.client.Do(req)
		if err != nil {
			lastErr = err
			continue
		}
		var payload struct {
			Status  string `json:"status"`
			Results []struct {
				FormattedAddress string `json:"formatted_address"`
				PlaceID          string `json:"place_id"`
				Geometry         struct {
					Location struct {
						Lat float64 `json:"lat"`
						Lng float64 `json:"lng"`
					} `json:"location"`
				} `json:"geometry"`
			} `json:"results"`
		}
		err = json.NewDecoder(resp.Body).Decode(&payload)
		resp.Body.Close()
		if err != nil {
			lastErr = err
			continue
		}
		if payload.Status != "OK" || len(payload.Results) == 0 {
			return nil, fmt.Errorf("Google Maps geocoding failed: %s", payload.Status)
		}
		result := payload.Results[0]
		return &GeocodedLocation{FormattedAddress: result.FormattedAddress, Latitude: result.Geometry.Location.Lat, Longitude: result.Geometry.Location.Lng, PlaceID: result.PlaceID}, nil
	}
	return nil, lastErr
}
