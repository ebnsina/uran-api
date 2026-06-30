package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
)

// client talks to the Uran API using a saved bearer token.
type client struct {
	apiURL string
	token  string
	http   *http.Client
}

func newClient(c credentials) *client {
	return &client{apiURL: strings.TrimRight(c.APIURL, "/"), token: c.Token, http: http.DefaultClient}
}

// request builds an authenticated request with an optional JSON body.
func (c *client) request(ctx context.Context, method, path string, body any) (*http.Request, error) {
	var r io.Reader
	if body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			return nil, err
		}
		r = bytes.NewReader(data)
	}
	req, err := http.NewRequestWithContext(ctx, method, c.apiURL+path, r)
	if err != nil {
		return nil, err
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if c.token != "" {
		req.Header.Set("Authorization", "Bearer "+c.token)
	}
	return req, nil
}

// do executes a request and decodes a JSON response into out (if non-nil).
// It returns an error for any non-2xx status, surfacing the server's message.
func (c *client) do(ctx context.Context, method, path string, body, out any) error {
	req, err := c.request(ctx, method, path, body)
	if err != nil {
		return err
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("%s %s: %s", method, path, serverError(resp))
	}
	if out != nil && resp.StatusCode != http.StatusNoContent {
		return json.NewDecoder(resp.Body).Decode(out)
	}
	return nil
}

// serverError extracts a human-readable message from an error response.
func serverError(resp *http.Response) string {
	data, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<16))
	var e struct {
		Error string `json:"error"`
	}
	if json.Unmarshal(data, &e) == nil && e.Error != "" {
		return fmt.Sprintf("%s (%d)", e.Error, resp.StatusCode)
	}
	return fmt.Sprintf("%s (%d)", strings.TrimSpace(string(data)), resp.StatusCode)
}
