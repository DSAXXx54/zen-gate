// zen-gate: a local tray gateway that exposes the OpenCode Zen free lane as
// OpenAI/Anthropic-compatible APIs and auto-configures installed agents.
//
// Protocol behaviour is ported from the MIT-licensed dsh-our-free-model
// plugin (github.com/zouyuxuan122/dsh-our-free-model); usage of the free lane
// remains subject to the upstream provider's terms.
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"time"

	"zen-gate/internal/agents"
	"zen-gate/internal/gateway"
	"zen-gate/internal/lane"
	"zen-gate/internal/logx"
	"zen-gate/internal/notify"
	"zen-gate/internal/store"
	"zen-gate/internal/tray"
	"zen-gate/internal/update"
	"zen-gate/internal/window"
)

// mainHwnd is the main window handle, captured at OnReady.
var mainHwnd uintptr

// windowHidden tracks tray-hide state for focus-aware notifications.
var windowHidden bool

func main() {
	// DPI awareness before any window (main window or tray) exists.
	window.SetProcessDPIAwareness()

	noTray := flag.Bool("no-tray", false, "console mode: no tray icon/window, log to stdout")
	port := flag.Int("port", 0, "override listen port")
	showVersion := flag.Bool("version", false, "print version and exit")
	flag.Parse()
	if *showVersion {
		fmt.Println("zen-gate", gateway.Version)
		return
	}

	st, err := store.Open()
	if err != nil {
		fmt.Println("open store:", err)
		os.Exit(1)
	}
	logger := logx.New(st.Home)
	defer logger.Close()

	window.SetAppUserModelID("zen-gate.gateway")

	if !window.AcquireSingleInstance(`Local\zen-gate-instance`, "Zen Gate · 本地免费模型网关") {
		logger.Infof("second launch: focused the running instance instead")
		return
	}

	cfg := st.Config()
	if *port > 0 {
		cfg.Port = *port
	}
	logger.Infof("zen-gate %s 启动 (port %d, proxy %s)", gateway.Version, cfg.Port, cfg.ProxyMode)

	ln := lane.NewLane()
	ln.SetDefaultMaxTokens(cfg.DefaultMaxTokens)
	ln.SetExposeRegion(cfg.ExposeRegion)
	ln.SetFailover(cfg.FailoverEnabled, cfg.FailoverMax)
	ln.LoadThrottleNotes(quotaNotesFromStore(st.SnapshotQuota()))
	ln.SetThrottleUpdate(func(model string, note lane.ThrottleNote) {
		qn := store.QuotaNote{ThrottledAt: note.ThrottledAt, CooldownUntil: note.CooldownUntil, LastOK: note.LastOK}
		for _, e := range note.Episodes {
			qn.Episodes = append(qn.Episodes, store.QuotaEpisode{Start: e.Start, End: e.End})
		}
		st.SetQuotaNote(model, qn)
	})
	lane.SetProxy(cfg.ProxyMode, cfg.ProxyURL)
	notify.SetEnabled(cfg.Notifications)
	ln.OnCall = func(rec lane.CallRecord) {
		st.Record(rec)
		_ = st.FlushStats()
	}
	// Probe first-token samples feed the persisted per-model average.
	ln.OnProbeResult = func(r lane.ProbeResult) {
		st.AddTTFTSample(r.Model, r.TTFTMs)
	}
	reg := agents.NewRegistry(st)
	gw := gateway.New(ln, st)
	gw.SetAgents(reg)
	gw.SetLogger(logger)
	window.SetDiag(func(msg string) { logger.Infof("%s", msg) })
	gw.SetAutostartState(agents.AutostartEnabled)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	stateText := func(s string) string {
		return map[string]string{lane.StateAvailable: "可用", lane.StateUnknown: "未知",
			lane.StateThrottled: "已限额", lane.StateRegionBlock: "地区受限",
			lane.StateUnavailable: "不可用"}[s]
	}
	ln.OnProbeEdge = func(model, from, to string) {
		if model == "" {
			logger.Warnf("免费车道整轮 429 限额，探测进入指数退避")
			if windowHidden {
				notify.Toast("免费车道已达限额", "本轮探测全部 429，稍后自动恢复")
			}
			return
		}
		logger.Infof("模型状态变化: %s %s → %s", model, stateText(from), stateText(to))
		if windowHidden {
			notify.Toast("模型状态变化", fmt.Sprintf("%s: %s → %s", model, stateText(from), stateText(to)))
		}
	}

	syncEndpoints := func() {
		reg.SetEndpoints(gw.BaseURL(), ln.ServableModels())
		tray.SetStatus(trayStatus(st, ln))
	}
	ln.OnChange = syncEndpoints
	ln.StartLoops(ctx, time.Duration(cfg.ProbeIntervalMinutes)*time.Minute)
	syncEndpoints()

	if err := gw.Start(); err != nil {
		logger.Errorf("listen on 127.0.0.1:%d: %v", cfg.Port, err)
		fmt.Println("listen error:", err)
		os.Exit(1)
	}
	dashURL := strings.TrimSuffix(gw.BaseURL(), "/v1")
	logger.Infof("dashboard ready at %s", dashURL)

	// periodic stats flush
	go func() {
		t := time.NewTicker(10 * time.Second)
		defer t.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-t.C:
				_ = st.FlushStats()
				st.FlushPerf()
			}
		}
	}()

	// update check loop (disabled until a feed URL is configured)
	go func() {
		check := func() {
			cfg := st.Config()
			if strings.TrimSpace(cfg.UpdateFeed) == "" {
				return
			}
			has, ver, url, _, err := update.Check(cfg.UpdateFeed, update.HTTP{Timeout: 15 * time.Second, Client: lane.Client()})
			if err != nil {
				logger.Warnf("update check failed: %v", err)
				return
			}
			gw.SetUpdateState(has, ver, url)
			if has && cfg.LastVersion != ver {
				logger.Infof("发现新版本 %s (当前 %s)", ver, gateway.Version)
				if windowHidden {
					notify.Toast("Zen Gate 有新版本 "+ver, "当前 "+gateway.Version+" · 打开管理页查看下载链接")
				}
				st.Config().LastVersion = ver
				_ = st.Save()
				syncEndpoints()
			}
		}
		time.Sleep(45 * time.Second)
		check()
		t := time.NewTicker(6 * time.Hour)
		defer t.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-t.C:
				check()
			}
		}
	}()

	if *noTray {
		logger.Infof("serving %s (dashboard %s)", gw.BaseURL(), dashURL)
		c := make(chan os.Signal, 1)
		signal.Notify(c, os.Interrupt)
		<-c
		cancel()
		gw.Stop()
		_ = st.FlushStats()
		return
	}

	// tray lives on its own locked thread; main thread owns the window
	go tray.Run(tray.Options{
		DashboardURL: dashURL,
		OnQuit: func() {
			saveWindowState(st)
			cancel()
			gw.Stop()
			_ = st.FlushStats()
			os.Exit(0)
		},
		OnReprobe: func() { go ln.ProbeRound(context.Background(), true) },
		OnShow: func() {
			windowHidden = false
			if mainHwnd != 0 {
				window.ShowWindowWin(mainHwnd)
			}
		},
	})

	window.Run(window.Options{
		Title:    "Zen Gate · 本地免费模型网关",
		URL:      dashURL,
		DataPath: st.Home + string(os.PathSeparator) + "webview",
		Width:    1160,
		Height:   820,
		Bounds: func() *window.Bounds {
			w := cfg.Window
			if w.W > 0 {
				return &window.Bounds{X: w.X, Y: w.Y, W: w.W, H: w.H, Maximized: w.Maximized}
			}
			return nil
		}(),
		OnReady: func(hwnd uintptr) {
			mainHwnd = hwnd
			saveWindowState(st) // persist migrated defaults + restored bounds
		},
		OnCloseButton: func() bool {
			saveWindowState(st)
			if st.Config().CloseToTray {
				windowHidden = true
				if mainHwnd != 0 {
					window.HideWindow(mainHwnd)
				}
				logger.Infof("窗口已最小化到托盘")
				return false
			}
			return true
		},
	})

	// window closed → quit
	cancel()
	gw.Stop()
	_ = st.FlushStats()
	logger.Infof("zen-gate 已退出")
}

// quotaNotesFromStore converts persisted quota notes back into the lane's
// shape for boot-time seeding.
func quotaNotesFromStore(in map[string]store.QuotaNote) map[string]lane.ThrottleNote {
	out := map[string]lane.ThrottleNote{}
	for m, n := range in {
		note := lane.ThrottleNote{ThrottledAt: n.ThrottledAt, CooldownUntil: n.CooldownUntil, LastOK: n.LastOK}
		for _, e := range n.Episodes {
			note.Episodes = append(note.Episodes, lane.ThrottleEpisode{Start: e.Start, End: e.End})
		}
		out[m] = note
	}
	return out
}

func saveWindowState(st *store.Store) {
	if mainHwnd == 0 {
		return
	}
	// A minimized window reports the sentinel off-screen rect; persisting it
	// would make the next launch open invisibly. Keep the last good bounds.
	if window.IsMinimized(mainHwnd) {
		return
	}
	x, y, r, b := window.GetBounds(mainHwnd)
	if r-x < 400 || b-y < 300 {
		return // collapsed or garbage rect — keep the last good state
	}
	cfg := st.Config()
	cfg.Window = store.WindowState{X: x, Y: y, W: r - x, H: b - y, Maximized: window.IsMaximized(mainHwnd)}
	_ = st.Save()
}

func trayStatus(st *store.Store, ln *lane.Lane) string {
	days, _ := st.SnapshotStats()
	today := time.Now().Format("2006-01-02")
	tok := 0
	if d, ok := days[today]; ok {
		tok = d.Output
	}
	return fmt.Sprintf("运行中 · 今日输出 %d tok · %d 模型可用", tok, len(ln.ServableModels()))
}
