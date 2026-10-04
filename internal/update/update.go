// Package update checks a configurable feed for newer zen-gate builds.
// The feed is any URL returning {"version":"x.y.z","url":"https://…"} —
// e.g. GitHub's releases/latest API reshaped, or a static JSON file.
package update

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// Current is the running version, overridable via -ldflags.
var Current = "1.1.0"

// FeedJSON is the expected remote shape.
type FeedJSON struct {
	Version string `json:"version"`
	URL     string `json:"url"`
	Notes   string `json:"notes"`
}

// Check fetches the feed and compares against Current.
// Returns (hasUpdate, version, downloadURL, notes, error).
func Check(feedURL string, client getJSONer) (bool, string, string, string, error) {
	if strings.TrimSpace(feedURL) == "" {
		return false, "", "", "", nil
	}
	data, err := client.Get(feedURL)
	if err != nil {
		return false, "", "", "", err
	}
	var feed FeedJSON
	if err := json.Unmarshal(data, &feed); err != nil {
		return false, "", "", "", err
	}
	return Newer(feed.Version, Current), strings.TrimSpace(feed.Version), strings.TrimSpace(feed.URL), feed.Notes, nil
}

type getJSONer interface{ Get(url string) ([]byte, error) }

// Newer compares dotted versions; "1.2.10" > "1.2.9".
func Newer(remote, current string) bool {
	a := parseVersion(remote)
	b := parseVersion(current)
	if len(a) == 0 {
		return false
	}
	for i := 0; i < 3; i++ {
		if a[i] != b[i] {
			return a[i] > b[i]
		}
	}
	return false
}

func parseVersion(v string) [3]int {
	var out [3]int
	v = strings.TrimPrefix(strings.TrimSpace(v), "v")
	parts := strings.SplitN(v, ".", 3)
	for i := 0; i < len(parts) && i < 3; i++ {
		n, err := strconv.Atoi(strings.TrimSpace(parts[i]))
		if err != nil {
			return [3]int{}
		}
		out[i] = n
	}
	return out
}

// HTTP fetcher with a sane timeout. An optional Client (e.g. the lane's
// proxy-aware one) takes precedence over the default transport.
type HTTP struct {
	Timeout time.Duration
	Client  *http.Client
}

func (h HTTP) Get(url string) ([]byte, error) {
	timeout := h.Timeout
	if timeout <= 0 {
		timeout = 15 * time.Second
	}
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	client := h.Client
	if client == nil {
		client = &http.Client{Timeout: timeout}
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("update feed returned HTTP %d", resp.StatusCode)
	}
	return io.ReadAll(io.LimitReader(resp.Body, 1<<20))
}
