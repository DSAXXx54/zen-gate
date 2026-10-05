// Package announce pulls the author's announcement feed — a small JSON file
// (default: announcements.json in the project's GitHub repo) — and filters it
// to the entries active today. 404 means "no announcements published", which
// is a normal state for private or fresh installs, not an error.
package announce

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sort"
	"time"
)

// Level spellings for an announcement.
const (
	LevelInfo     = "info"
	LevelWarn     = "warn"
	LevelCritical = "critical"
)

// Item is one announcement in the feed.
type Item struct {
	ID    string `json:"id"` // stable id; the dashboard uses it for 已读 tracking
	Title string `json:"title"`
	Body  string `json:"body,omitempty"`
	Level string `json:"level,omitempty"` // info | warn | critical (default info)
	Start string `json:"start,omitempty"` // YYYY-MM-DD, inclusive
	End   string `json:"end,omitempty"`   // YYYY-MM-DD, inclusive; empty = no end
	URL   string `json:"url,omitempty"`   // optional "了解更多" link
}

type feedFile struct {
	Announcements []Item `json:"announcements"`
}

const dateLayout = "2006-01-02"

// Fetch pulls and parses the feed, keeping only entries active today. A 404
// yields (nil, nil). Entries outside their [start,end] window are dropped and
// the rest are ordered critical → warn → info, newest first within a level.
func Fetch(ctx context.Context, url string, client *http.Client) ([]Item, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("accept", "application/json")
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNotFound {
		return nil, nil
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return nil, err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	var f feedFile
	if err := json.Unmarshal(data, &f); err != nil {
		return nil, fmt.Errorf("公告格式不是合法 JSON: %v", err)
	}
	return Active(f.Announcements, time.Now()), nil
}

// Active filters items to those whose window contains now and normalizes
// levels. Items with an unparsable or future start are skipped.
func Active(items []Item, now time.Time) []Item {
	today := now.Format(dateLayout)
	out := []Item{}
	for _, it := range items {
		if it.ID == "" || it.Title == "" {
			continue
		}
		switch it.Level {
		case LevelInfo, LevelWarn, LevelCritical:
		default:
			it.Level = LevelInfo
		}
		if it.Start != "" {
			if _, err := time.ParseInLocation(dateLayout, it.Start, time.Local); err != nil {
				continue
			}
			// Zero-padded YYYY-MM-DD compares correctly as a string.
			if today < it.Start {
				continue
			}
		}
		if it.End != "" {
			if _, err := time.ParseInLocation(dateLayout, it.End, time.Local); err != nil {
				it.End = ""
			} else if today > it.End {
				continue
			}
		}
		out = append(out, it)
	}
	sort.SliceStable(out, func(i, j int) bool {
		ri, rj := levelRank(out[i].Level), levelRank(out[j].Level)
		if ri != rj {
			return ri < rj
		}
		return out[i].Start > out[j].Start
	})
	return out
}

func levelRank(l string) int {
	switch l {
	case LevelCritical:
		return 0
	case LevelWarn:
		return 1
	default:
		return 2
	}
}
