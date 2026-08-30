package client

import (
	"io"
	"net/http"
	"time"

	"github.com/Tushardevx01/runstack/internal/config"
)

type Client struct {
	BaseURL    string
	Token      string
	HTTPClient *http.Client
}

func NewFromConfig() (*Client, error) {
	cfg, err := config.LoadConfig()
	if err != nil {
		return nil, err
	}
	ctx, err := cfg.GetActiveContext()
	if err != nil {
		return nil, err
	}
	return New(ctx.Endpoint, ctx.Token), nil
}

func New(baseURL, token string) *Client {
	return &Client{
		BaseURL: baseURL,
		Token:   token,
		HTTPClient: &http.Client{
			Timeout: 10 * time.Second,
		},
	}
}

func (c *Client) DoReq(req *http.Request) (*http.Response, error) {
	if c.Token != "" {
		req.Header.Set("Authorization", "Bearer "+c.Token)
	}
	return c.HTTPClient.Do(req)
}

func (c *Client) Get(path string) (*http.Response, error) {
	req, err := http.NewRequest("GET", c.BaseURL+path, nil)
	if err != nil {
		return nil, err
	}
	return c.DoReq(req)
}

func (c *Client) Post(path string, contentType string, body io.Reader) (*http.Response, error) {
	req, err := http.NewRequest("POST", c.BaseURL+path, body)
	if err != nil {
		return nil, err
	}
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
	return c.DoReq(req)
}

func (c *Client) Put(path string, contentType string, body io.Reader) (*http.Response, error) {
	req, err := http.NewRequest("PUT", c.BaseURL+path, body)
	if err != nil {
		return nil, err
	}
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
	return c.DoReq(req)
}

func (c *Client) Delete(path string) (*http.Response, error) {
	req, err := http.NewRequest("DELETE", c.BaseURL+path, nil)
	if err != nil {
		return nil, err
	}
	return c.DoReq(req)
}

func (c *Client) Patch(path string, contentType string, body io.Reader) (*http.Response, error) {
	req, err := http.NewRequest("PATCH", c.BaseURL+path, body)
	if err != nil {
		return nil, err
	}
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
	return c.DoReq(req)
}
