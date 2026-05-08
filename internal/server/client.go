// Package server is the HTTP client the sync engine uses to talk to the
// organization server. Three endpoints, no retries — the engine owns retry
// semantics by virtue of running on a tick.
package server

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"

	"github.com/Armatorix/TelegramOrganizationSync/internal/api"
)

type Client struct {
	baseURL *url.URL
	apiKey  string
	http    *http.Client
}

func New(baseURL, apiKey string) (*Client, error) {
	u, err := url.Parse(baseURL)
	if err != nil {
		return nil, fmt.Errorf("parse base url: %w", err)
	}
	return &Client{
		baseURL: u,
		apiKey:  apiKey,
		http:    &http.Client{Timeout: 30 * time.Second},
	}, nil
}

func (c *Client) ListChannels(ctx context.Context) ([]api.Channel, error) {
	var out []api.Channel
	if err := c.do(ctx, http.MethodGet, "/api/v1/channels", nil, &out); err != nil {
		return nil, err
	}
	return out, nil
}

func (c *Client) CreateChannel(ctx context.Context, req api.CreateChannelRequest) (api.Channel, error) {
	var out api.Channel
	if err := c.do(ctx, http.MethodPost, "/api/v1/channels", req, &out); err != nil {
		return api.Channel{}, err
	}
	return out, nil
}

func (c *Client) Reconcile(ctx context.Context, channelID string, req api.ReconcileRequest) (api.ReconcileResponse, error) {
	var out api.ReconcileResponse
	path := "/api/v1/channels/" + url.PathEscape(channelID) + "/members:reconcile"
	if err := c.do(ctx, http.MethodPost, path, req, &out); err != nil {
		return api.ReconcileResponse{}, err
	}
	return out, nil
}

func (c *Client) do(ctx context.Context, method, path string, body, out any) error {
	var rdr io.Reader
	if body != nil {
		buf, err := json.Marshal(body)
		if err != nil {
			return fmt.Errorf("marshal body: %w", err)
		}
		rdr = bytes.NewReader(buf)
	}

	u := *c.baseURL
	u.Path = path
	req, err := http.NewRequestWithContext(ctx, method, u.String(), rdr)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+c.apiKey)
	req.Header.Set("Accept", "application/json")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		raw, _ := io.ReadAll(resp.Body)
		var prob api.Problem
		if json.Unmarshal(raw, &prob) == nil && prob.Title != "" {
			prob.Status = resp.StatusCode
			return &prob
		}
		return fmt.Errorf("%s %s: %s: %s", method, path, resp.Status, string(raw))
	}

	if out != nil {
		if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
			return fmt.Errorf("decode response: %w", err)
		}
	}
	return nil
}
