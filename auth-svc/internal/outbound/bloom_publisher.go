package outbound

import (
	"auth-svc/internal/service"
	"bytes"
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/bits-and-blooms/bloom/v3"
)

var _ service.BloomPublisher = (*BloomPublisher)(nil)

type BloomPublisher struct {
	Store      service.UserStore
	CentralURL string
	Realm      string
	AuthToken  string
	HTTPClient *http.Client
}

func (p *BloomPublisher) Publish(ctx context.Context) error {
	usernames, err := p.Store.ListUsernames(ctx)
	if err != nil {
		return err
	}
	filter := bloom.NewWithEstimates(uint(len(usernames)+1), 0.001)
	for _, username := range usernames {
		filter.AddString(normalize(username))
	}
	var body bytes.Buffer
	if _, err := filter.WriteTo(&body); err != nil {
		return fmt.Errorf("serialize bloom filter: %w", err)
	}
	endpoint := fmt.Sprintf("%s/internal/realms/%s/bloom",
		p.CentralURL, url.PathEscape(p.Realm))
	req, err := http.NewRequestWithContext(ctx, http.MethodPut, endpoint, &body)
	if err != nil {
		return err
	}
	req.Header.Set("X-Central-Auth-Token", p.AuthToken)
	req.Header.Set("Content-Type", "application/octet-stream")
	client := p.HTTPClient
	if client == nil {
		client = &http.Client{Timeout: 10 * time.Second}
	}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNoContent {
		return fmt.Errorf("central auth returned %s", resp.Status)
	}
	return nil
}

func normalize(value string) string {
	return strings.ToLower(strings.TrimSpace(value))
}
