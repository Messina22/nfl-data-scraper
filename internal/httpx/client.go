package httpx

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"time"
)

// Client is a shared HTTP client with a browser-like User-Agent.
type Client struct {
	HTTP    *http.Client
	Referer string
	Cookie  string
}

func New(timeout time.Duration) *Client {
	return &Client{
		HTTP: &http.Client{Timeout: timeout},
	}
}

func (c *Client) Get(ctx context.Context, url string) ([]byte, string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, "", err
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/122.0.0.0 Safari/537.36")
	req.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,application/json;q=0.9,*/*;q=0.8")
	req.Header.Set("Accept-Language", "en-US,en;q=0.9")
	if c.Referer != "" {
		req.Header.Set("Referer", c.Referer)
	}
	if c.Cookie != "" {
		req.Header.Set("Cookie", c.Cookie)
	}

	res, err := c.HTTP.Do(req)
	if err != nil {
		return nil, "", err
	}
	defer res.Body.Close()

	body, err := io.ReadAll(io.LimitReader(res.Body, 12<<20))
	if err != nil {
		return nil, "", err
	}
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		return body, res.Request.URL.String(), fmt.Errorf("HTTP %d for %s", res.StatusCode, url)
	}
	final := url
	if res.Request != nil && res.Request.URL != nil {
		final = res.Request.URL.String()
	}
	return body, final, nil
}
