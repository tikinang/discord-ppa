package ppa

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
	"time"
)

var HTTPClient = &http.Client{
	Timeout: 5 * time.Minute,
}

func HTTPWithRetry(ctx context.Context, url, method string) (*http.Response, error) {
	for attempt := range 3 {
		req, err := http.NewRequestWithContext(ctx, method, url, nil)
		if err != nil {
			return nil, err
		}
		resp, err := HTTPClient.Do(req)
		if err != nil {
			return nil, err
		}
		if resp.StatusCode != http.StatusTooManyRequests {
			return resp, nil
		}
		resp.Body.Close()

		wait := 30 * time.Second * time.Duration(1<<attempt)
		if ra := resp.Header.Get("Retry-After"); ra != "" {
			if secs, err := strconv.Atoi(ra); err == nil && secs > 0 {
				wait = time.Duration(min(secs, 3600)) * time.Second
			}
		}
		slog.Warn("Rate limited", "retry_after", wait, "attempt", attempt+1)
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(wait):
		}
	}
	return nil, fmt.Errorf("rate limited after 3 retries")
}
