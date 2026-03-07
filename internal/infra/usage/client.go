package usage

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/prakersh/codexmultiauth/internal/domain"
	"github.com/prakersh/codexmultiauth/internal/infra/store"
)

type Client struct {
	BaseURL    string
	HTTPClient *http.Client
}

func NewClient(baseURL string) *Client {
	if strings.TrimSpace(baseURL) == "" {
		baseURL = "https://chatgpt.com/backend-api/wham/usage"
	}
	return &Client{
		BaseURL: baseURL,
		HTTPClient: &http.Client{
			Timeout: 10 * time.Second,
		},
	}
}

func (c *Client) Fetch(ctx context.Context, auth store.CodexAuth) (domain.UsageSummary, error) {
	if auth.Tokens == nil || auth.Tokens.AccessToken == "" {
		return domain.UsageSummary{}, fmt.Errorf("usage API requires access token")
	}
	endpoints := []string{c.BaseURL}
	if strings.Contains(c.BaseURL, "/backend-api/wham/usage") {
		endpoints = append(endpoints, strings.Replace(c.BaseURL, "/backend-api/wham/usage", "/api/codex/usage", 1))
	} else if strings.Contains(c.BaseURL, "/api/codex/usage") {
		endpoints = append(endpoints, strings.Replace(c.BaseURL, "/api/codex/usage", "/backend-api/wham/usage", 1))
	}

	var lastErr error
	for _, endpoint := range endpoints {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
		if err != nil {
			return domain.UsageSummary{}, err
		}
		req.Header.Set("Authorization", "Bearer "+auth.Tokens.AccessToken)
		req.Header.Set("Accept", "application/json")
		if auth.Tokens.AccountID != "" {
			req.Header.Set("X-Account-Id", auth.Tokens.AccountID)
			req.Header.Set("ChatClaude-Account-Id", auth.Tokens.AccountID)
		}
		resp, err := c.HTTPClient.Do(req)
		if err != nil {
			lastErr = err
			continue
		}
		body, readErr := io.ReadAll(io.LimitReader(resp.Body, 1<<16))
		resp.Body.Close()
		if readErr != nil {
			lastErr = readErr
			continue
		}
		if resp.StatusCode == http.StatusNotFound {
			lastErr = fmt.Errorf("usage endpoint %s returned 404", endpoint)
			continue
		}
		if resp.StatusCode != http.StatusOK {
			lastErr = fmt.Errorf("usage endpoint %s returned %d", endpoint, resp.StatusCode)
			continue
		}
		return ParseResponse(body)
	}
	if lastErr != nil {
		return domain.UsageSummary{}, lastErr
	}
	return domain.UsageSummary{}, fmt.Errorf("usage endpoint unavailable")
}
