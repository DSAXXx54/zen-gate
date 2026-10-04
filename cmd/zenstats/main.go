// Command zenstats is the server-side companion of Zen Gate: it receives the
// anonymous heartbeats the desktop gateway sends ({installId, version,
// country}) and serves a dashboard showing how many people are using it.
//
// Single binary, zero dependencies, one JSON file for storage:
//
//	zenstats.exe              # serves :8321 (dashboard + API)
//	ZENSTATS_ADDR=:8321       # override listen address
//	ZENSTATS_TOKEN=s3cret     # require ?token=s3cret on the dashboard (optional)
package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// Install is one deployed Zen Gate instance.
type Install struct {
	InstallID string `json:"installId"`
	Version   string `json:"version"`
	Country   string `json:"country,omitempty"`
	FirstSeen int64  `json:"firstSeen"`
	LastSeen  int64  `json:"lastSeen"`
	Pings     int64  `json:"pings"`
}

type store struct {
	mu       sync.Mutex
	installs map[string]*Install
	dirty    bool
}

var db = &store{installs: map[string]*Install{}}

var dataFile string

func main() {
	addr := os.Getenv("ZENSTATS_ADDR")
	if addr == "" {
		addr = ":8321"
	}
	exe, _ := os.Executable()
	dataFile = filepath.Join(filepath.Dir(exe), "stats-server.json")
	load()

	mux := http.NewServeMux()
	mux.HandleFunc("/", handleDashboard)
	mux.HandleFunc("/api/ping", handlePing)
	mux.HandleFunc("/api/stats", handleStats)
	mux.HandleFunc("/api/installs", handleInstalls)

	fmt.Printf("zenstats serving on %s (data: %s)\n", addr, dataFile)
	if err := http.ListenAndServe(addr, mux); err != nil {
		fmt.Println("listen error:", err)
		os.Exit(1)
	}
}

func load() {
	b, err := os.ReadFile(dataFile)
	if err != nil {
		return
	}
	_ = json.Unmarshal(b, &db.installs)
}

func saveLocked() {
	snapshot := make(map[string]*Install, len(db.installs))
	for k, v := range db.installs {
		cp := *v
		snapshot[k] = &cp
	}
	b, _ := json.MarshalIndent(snapshot, "", " ")
	tmp := dataFile + ".tmp"
	if err := os.WriteFile(tmp, b, 0o600); err == nil {
		_ = os.Rename(tmp, dataFile)
	}
	db.dirty = false
}

func handlePing(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var in Install
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		http.Error(w, "bad json", http.StatusBadRequest)
		return
	}
	in.InstallID = strings.TrimSpace(in.InstallID)
	if in.InstallID == "" || len(in.InstallID) > 64 {
		http.Error(w, "missing installId", http.StatusBadRequest)
		return
	}
	now := time.Now().UnixMilli()
	in.Version = strings.TrimSpace(in.Version)
	in.Country = strings.ToUpper(strings.TrimSpace(in.Country))
	in.LastSeen = now

	db.mu.Lock()
	defer db.mu.Unlock()
	cur, ok := db.installs[in.InstallID]
	if !ok {
		in.FirstSeen = now
		in.Pings = 1
		db.installs[in.InstallID] = &in
		db.dirty = true
	} else {
		cur.Version, cur.Country, cur.LastSeen = in.Version, in.Country, in.LastSeen
		cur.Pings++
		db.dirty = true
	}
	if db.dirty {
		saveLocked()
	}
	w.Header().Set("content-type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{"ok": true})
}

func handleStats(w http.ResponseWriter, r *http.Request) {
	db.mu.Lock()
	defer db.mu.Unlock()
	if db.dirty {
		saveLocked()
	}
	now := time.Now().UnixMilli()
	active7, active30 := int64(0), int64(0)
	versions := map[string]int64{}
	countries := map[string]int64{}
	for _, in := range db.installs {
		if now-in.LastSeen <= 7*24*3600*1000 {
			active7++
		}
		if now-in.LastSeen <= 30*24*3600*1000 {
			active30++
		}
		if in.Version != "" {
			versions[in.Version]++
		}
		if in.Country != "" {
			countries[in.Country]++
		}
	}
	writeJSON(w, map[string]any{
		"totalInstalls": len(db.installs),
		"active7d":      active7,
		"active30d":     active30,
		"versions":      versions,
		"countries":     countries,
		"generatedAt":   now,
	})
}

// handleInstalls lists every install (the dashboard table + /api consumers).
func handleInstalls(w http.ResponseWriter, r *http.Request) {
	db.mu.Lock()
	defer db.mu.Unlock()
	if db.dirty {
		saveLocked()
	}
	out := make([]Install, 0, len(db.installs))
	for _, in := range db.installs {
		out = append(out, *in)
	}
	writeJSON(w, out)
}

// --- dashboard ----------------------------------------------------------------

var dashboardToken = os.Getenv("ZENSTATS_TOKEN")

func handleDashboard(w http.ResponseWriter, r *http.Request) {
	if dashboardToken != "" && r.URL.Query().Get("token") != dashboardToken {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	w.Header().Set("content-type", "text/html; charset=utf-8")
	_, _ = w.Write([]byte(dashboardHTML()))
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("content-type", "application/json")
	_ = json.NewEncoder(w).Encode(v)
}

func dashboardHTML() string { return dashHTML }

const dashHTML = `<!doctype html>
<html lang="zh"><head><meta charset="utf-8"><title>Zen Gate · 用户总览</title>
<meta name="viewport" content="width=device-width,initial-scale=1">
<style>
:root { --bg:#141414; --panel:#1d1d1d; --panel-2:#262626; --ink:#ececec; --ink-dim:#a3a3a3; --ink-mute:#737373;
  --hairline:rgba(255,255,255,.08); --accent:#38bdf8; --ok:#34d399; --warn:#fbbf24; }
* { box-sizing:border-box; }
body { margin:0; background:var(--bg); color:var(--ink); font:14px/1.6 "Segoe UI",system-ui,sans-serif; }
.wrap { max-width:1080px; margin:0 auto; padding:28px 20px 60px; }
h1 { font-size:20px; letter-spacing:.14em; margin:0 0 4px; }
h1 b { color:var(--accent); font-weight:600; }
.sub { color:var(--ink-mute); font-size:12px; margin-bottom:24px; }
.cards { display:grid; grid-template-columns:repeat(auto-fit,minmax(190px,1fr)); gap:12px; margin-bottom:20px; }
.card { background:var(--panel); border:1px solid var(--hairline); border-radius:10px; padding:16px 18px; }
.card .v { font-size:28px; font-weight:700; color:var(--accent); }
.card .l { font-size:12px; color:var(--ink-mute); margin-top:2px; }
.panel { background:var(--panel); border:1px solid var(--hairline); border-radius:10px; padding:16px 18px; margin-bottom:16px; }
.panel h3 { margin:0 0 12px; font-size:13px; color:var(--ink-dim); letter-spacing:.06em; }
.bar-row { display:flex; align-items:center; gap:10px; margin:6px 0; }
.bar-row .nm { width:120px; font-size:12.5px; color:var(--ink-dim); text-align:right; flex:none; }
.bar-row .tr { flex:1; height:14px; background:var(--panel-2); border-radius:4px; overflow:hidden; }
.bar-row .tr i { display:block; height:100%; background:var(--accent); border-radius:4px; }
.bar-row .ct { width:56px; font-size:12px; color:var(--ink-mute); font-variant-numeric:tabular-nums; }
table { width:100%; border-collapse:collapse; font-size:12.5px; }
th { text-align:left; color:var(--ink-mute); font-weight:500; padding:4px 8px; border-bottom:1px solid var(--hairline); }
td { padding:5px 8px; border-bottom:1px solid rgba(255,255,255,.04); color:var(--ink-dim); }
.dot { width:7px; height:7px; border-radius:50%; background:var(--ok); display:inline-block; margin-right:6px; }
.muted { color:var(--ink-mute); font-size:11.5px; margin-top:14px; }
</style></head><body><div class="wrap">
<h1>ZEN—GATE <b>用户总览</b></h1>
<div class="sub">数据来自各部署实例的匿名心跳（installId / 版本 / 国家），仅存于本服务器</div>
<div class="cards">
  <div class="card"><div class="v" id="c-total">—</div><div class="l">累计安装</div></div>
  <div class="card"><div class="v" id="c-7d">—</div><div class="l">7 日活跃</div></div>
  <div class="card"><div class="v" id="c-30d">—</div><div class="l">30 日活跃</div></div>
  <div class="card"><div class="v" id="c-ver">—</div><div class="l">版本数</div></div>
</div>
<div class="panel"><h3>版本分布</h3><div id="p-versions"></div></div>
<div class="panel"><h3>国家 / 地区分布</h3><div id="p-countries"></div></div>
<div class="panel"><h3>最近心跳</h3><table><thead><tr><th>实例</th><th>版本</th><th>国家</th><th>首次</th><th>最近</th><th>心跳</th></tr></thead><tbody id="tb"></tbody></table></div>
<div class="muted" id="gen"></div>
</div>
<script>
const f = n => new Date(n).toLocaleString("zh-CN", {hour12:false});
const shortId = s => s.slice(0,8);
fetch("/api/stats").then(r=>r.json()).then(d=>{
  document.getElementById("c-total").textContent = d.totalInstalls;
  document.getElementById("c-7d").textContent = d.active7d;
  document.getElementById("c-30d").textContent = d.active30d;
  const vers = Object.keys(d.versions||{});
  document.getElementById("c-ver").textContent = vers.length;
  const bars = (obj, el) => {
    const items = Object.entries(obj||{}).sort((a,b)=>b[1]-a[1]);
    const max = Math.max(1, ...items.map(x=>x[1]));
    document.getElementById(el).innerHTML = items.length
      ? items.map(([k,v])=>'<div class="bar-row"><span class="nm">'+k+'</span><span class="tr"><i style="width:'+Math.round(v/max*100)+'%"></i></span><span class="ct">'+v+'</span></div>').join("")
      : '<div class="muted">暂无数据</div>';
  };
  bars(d.versions, "p-versions");
  bars(d.countries, "p-countries");
  document.getElementById("gen").textContent = "生成于 " + f(d.generatedAt);
});
fetch("/api/installs").then(r=>r.json()).then(list=>{
  const rows = [...list].sort((a,b)=>b.lastSeen-a.lastSeen).slice(0,50);
  document.getElementById("tb").innerHTML = rows.map(x=>
    '<tr><td><span class="dot"></span>'+shortId(x.installId)+'…</td><td>'+ (x.version||"—") +'</td><td>'+ (x.country||"—") +'</td><td>'+f(x.firstSeen)+'</td><td>'+f(x.lastSeen)+'</td><td>'+x.pings+'</td></tr>').join("");
});
</script></body></html>`
