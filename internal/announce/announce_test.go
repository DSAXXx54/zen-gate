package announce

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestFetchParsesAndFilters(t *testing.T) {
	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("content-type", "application/json")
		_, _ = w.Write([]byte(`{"announcements":[
			{"id":"a1","title":"维护","level":"warn","start":"2026-01-01"},
			{"id":"a2","title":"过期","level":"info","start":"2026-01-01","end":"2026-01-31"},
			{"id":"a3","title":"未来","level":"info","start":"2099-01-01"},
			{"id":"","title":"无id"},
			{"id":"a4","title":"紧急","level":"critical"}
		]}`))
	}))
	defer up.Close()
	items, err := Fetch(context.Background(), up.URL, up.Client())
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 2 {
		t.Fatalf("items = %+v, want a4 (critical) then a1 (warn)", items)
	}
	if items[0].ID != "a4" || items[1].ID != "a1" {
		t.Fatalf("order = %v, %v", items[0].ID, items[1].ID)
	}
}

func TestFetch404IsEmpty(t *testing.T) {
	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer up.Close()
	items, err := Fetch(context.Background(), up.URL, up.Client())
	if err != nil {
		t.Fatalf("404 must not be an error, got %v", err)
	}
	if items != nil {
		t.Fatalf("items = %v, want nil", items)
	}
}

func TestFetchBadJSON(t *testing.T) {
	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("<html>not json</html>"))
	}))
	defer up.Close()
	if _, err := Fetch(context.Background(), up.URL, up.Client()); err == nil {
		t.Fatal("want error for non-JSON body")
	}
}

func TestActiveDefaultsLevel(t *testing.T) {
	now := time.Date(2026, 10, 5, 12, 0, 0, 0, time.Local)
	out := Active([]Item{{ID: "x", Title: "t", Level: "weird"}}, now)
	if len(out) != 1 || out[0].Level != LevelInfo {
		t.Fatalf("out = %+v", out)
	}
}
