// Package keycloak implements the app ports with the Keycloak Admin REST API.
// Each Keycloak is reached with a service account (client credentials grant)
// that holds only the realm-management roles it needs.
package keycloak

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"control-plane-svc/internal/domain"
)

// Connection says how to reach one realm of one Keycloak.
type Connection struct {
	BaseURL      string // for example http://auth.dc-1.babylon.test
	Realm        string // for example babylon
	ClientID     string
	ClientSecret string
}

// Client calls the Admin REST API of one realm.
type Client struct {
	conn Connection
	http *http.Client

	mu      sync.Mutex
	token   string
	expires time.Time
}

func NewClient(conn Connection, httpClient *http.Client) *Client {
	conn.BaseURL = strings.TrimRight(conn.BaseURL, "/")
	return &Client{conn: conn, http: httpClient}
}

// accessToken returns a cached token, or fetches one when it's about to expire.
func (c *Client) accessToken(ctx context.Context) (string, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.token != "" && time.Now().Before(c.expires) {
		return c.token, nil
	}

	form := url.Values{
		"grant_type":    {"client_credentials"},
		"client_id":     {c.conn.ClientID},
		"client_secret": {c.conn.ClientSecret},
	}
	endpoint := fmt.Sprintf("%s/realms/%s/protocol/openid-connect/token", c.conn.BaseURL, url.PathEscape(c.conn.Realm))
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, strings.NewReader(form.Encode()))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := c.http.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("getting a token for client %q: %w", c.conn.ClientID, statusError(resp))
	}
	var body struct {
		AccessToken string `json:"access_token"`
		ExpiresIn   int    `json:"expires_in"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return "", fmt.Errorf("decoding the token response: %w", err)
	}
	c.token = body.AccessToken
	// Renew a little early so a token doesn't expire between check and use.
	c.expires = time.Now().Add(time.Duration(body.ExpiresIn)*time.Second - 30*time.Second)
	return c.token, nil
}

// do sends an admin request. path is relative to /admin/realms/{realm} and must
// already be escaped. When out isn't nil, a JSON response is decoded into it.
func (c *Client) do(ctx context.Context, method, path string, query url.Values, in, out any) (*http.Response, error) {
	token, err := c.accessToken(ctx)
	if err != nil {
		return nil, err
	}

	u := fmt.Sprintf("%s/admin/realms/%s%s", c.conn.BaseURL, url.PathEscape(c.conn.Realm), path)
	if len(query) > 0 {
		u += "?" + query.Encode()
	}
	var body io.Reader
	if in != nil {
		b, err := json.Marshal(in)
		if err != nil {
			return nil, err
		}
		body = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(ctx, method, u, body)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Accept", "application/json")
	if in != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	switch {
	case resp.StatusCode == http.StatusUnauthorized:
		// A Keycloak restart or key rotation can invalidate a cached token; fetch
		// a new one on the next call.
		c.mu.Lock()
		c.token = ""
		c.mu.Unlock()
		return resp, fmt.Errorf("%s %s: %w", method, path, statusError(resp))
	case resp.StatusCode == http.StatusNotFound:
		return resp, domain.ErrNotFound
	case resp.StatusCode == http.StatusConflict:
		return resp, domain.ErrConflict
	case resp.StatusCode >= 300:
		return resp, fmt.Errorf("%s %s: %w", method, path, statusError(resp))
	}
	if out != nil {
		if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
			return resp, fmt.Errorf("decoding %s %s: %w", method, path, err)
		}
	}
	return resp, nil
}

// statusError keeps Keycloak's error message, which says what was wrong, but
// caps it so a large HTML error page doesn't end up in the UI.
func statusError(resp *http.Response) error {
	b, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
	var kc struct {
		Error            string `json:"error"`
		ErrorDescription string `json:"error_description"`
		ErrorMessage     string `json:"errorMessage"`
	}
	if json.Unmarshal(b, &kc) == nil {
		for _, msg := range []string{kc.ErrorMessage, kc.ErrorDescription, kc.Error} {
			if msg != "" {
				return fmt.Errorf("HTTP %d: %s", resp.StatusCode, msg)
			}
		}
	}
	if msg := strings.TrimSpace(string(b)); msg != "" && !strings.HasPrefix(msg, "<") {
		return fmt.Errorf("HTTP %d: %s", resp.StatusCode, msg)
	}
	return errors.New(resp.Status)
}
