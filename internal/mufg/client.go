// Package mufg fetches 基準価額 (NAV) history from Mitsubishi UFJ Asset
// Management's public "ファンド情報 API" (developer.am.mufg.jp), verified
// against the vendor's published API spec (仕様書 v1.4.0, 2026-06-09). This
// is the documented, externally-published endpoint SPEC.md 4.1 refers to —
// not screen-scraping.
package mufg

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"time"
)

const defaultBaseURL = "https://developer.am.mufg.jp"

// ErrNoData means the API had nothing for that fund/date — normal for
// weekends, market holidays, and dates before the fund's inception.
var ErrNoData = errors.New("mufg: no NAV data for this fund/date")

// Quote is one day's 基準価額 (price per 10,000 units, in yen).
type Quote struct {
	Date   time.Time
	NAVYen int
}

// Client calls the MUFG fund information API.
type Client struct {
	baseURL    string
	httpClient *http.Client
}

// Option configures a Client.
type Option func(*Client)

// WithBaseURL overrides the API base URL (for tests).
func WithBaseURL(baseURL string) Option {
	return func(c *Client) { c.baseURL = baseURL }
}

// WithHTTPClient overrides the *http.Client used for requests (for tests).
func WithHTTPClient(hc *http.Client) Option {
	return func(c *Client) { c.httpClient = hc }
}

// NewClient builds a Client against the real MUFG API by default.
func NewClient(opts ...Option) *Client {
	c := &Client{baseURL: defaultBaseURL, httpClient: http.DefaultClient}
	for _, opt := range opts {
		opt(c)
	}
	return c
}

type apiResponse struct {
	Result struct {
		Status int    `json:"status"`
		ErrMsg string `json:"errmsg"`
	} `json:"result"`
	Datasets []struct {
		NAV int `json:"nav"`
	} `json:"datasets"`
}

// FetchByDate fetches fundCode's 基準価額 for the given calendar date, via
// GET /fund_information_date/fund_cd/{fundCode}/base_date/{YYYYMMDD}.
func (c *Client) FetchByDate(ctx context.Context, fundCode string, d time.Time) (Quote, error) {
	url := fmt.Sprintf("%s/fund_information_date/fund_cd/%s/base_date/%s", c.baseURL, fundCode, d.Format("20060102"))

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return Quote{}, fmt.Errorf("building request: %w", err)
	}
	req.Header.Set("Accept", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return Quote{}, fmt.Errorf("calling mufg api: %w", err)
	}
	defer resp.Body.Close()

	var body apiResponse
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return Quote{}, fmt.Errorf("decoding mufg api response: %w", err)
	}

	if body.Result.Status == http.StatusNotFound || len(body.Datasets) == 0 {
		return Quote{}, ErrNoData
	}
	if body.Result.Status != http.StatusOK {
		return Quote{}, fmt.Errorf("mufg api error: status=%d %s", body.Result.Status, body.Result.ErrMsg)
	}

	return Quote{Date: d, NAVYen: body.Datasets[0].NAV}, nil
}
