package agents

import (
	"archive/zip"
	"bytes"
	_ "embed"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// dsh manages the dsh-our-free-model plugin for DeepSeek Harness: it installs
// the plugin's runtime files into each profile's node_modules and registers it
// in the profile's package.json (dependencies + dsh.profile.bundles), the
// exact wiring the plugin's own documentation prescribes. The bundled plugin
// is the MIT-licensed upstream v1.3.2.

//go:embed assets/dsh-plugin.zip
var dshPluginZip []byte

const dshPluginID = "dsh-our-free-model"

type dsh struct{}

func newDSH() *dsh { return &dsh{} }

func (d *dsh) Meta() (string, string, string) {
	return "dsh", "DeepSeek Harness",
		"安装/摘除 dsh-our-free-model 插件（~/.dsh/profiles/*/node_modules + bundles）"
}

func (d *dsh) profilesDir() string { return homePath(".dsh", "profiles") }

// profileDirs lists profile folders that carry a package.json.
func (d *dsh) profileDirs() []string {
	entries, err := os.ReadDir(d.profilesDir())
	if err != nil {
		return nil
	}
	out := []string{}
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		if _, err := os.Stat(filepath.Join(d.profilesDir(), e.Name(), "package.json")); err == nil {
			out = append(out, filepath.Join(d.profilesDir(), e.Name()))
		}
	}
	return out
}

func (d *dsh) Detect() (bool, string, string) {
	profiles := d.profileDirs()
	if len(profiles) == 0 {
		return false, "", "未检测到 DeepSeek Harness profile"
	}
	version := d.bundledVersion()
	// report the installed plugin version if any profile has one
	for _, p := range profiles {
		data, err := os.ReadFile(filepath.Join(p, "node_modules", dshPluginID, "package.json"))
		if err == nil {
			var pkg struct {
				Version string `json:"version"`
			}
			if json.Unmarshal(data, &pkg) == nil && pkg.Version != "" {
				return true, pkg.Version, fmt.Sprintf("%d 个 profile", len(profiles))
			}
		}
	}
	return true, version, fmt.Sprintf("%d 个 profile（未安装插件）", len(profiles))
}

func (d *dsh) IsEnabled() (bool, string, error) {
	for _, p := range d.profileDirs() {
		if d.profileEnabled(p) {
			return true, "", nil
		}
	}
	return false, "", nil
}

func (d *dsh) profileEnabled(profile string) bool {
	data, err := os.ReadFile(filepath.Join(profile, "package.json"))
	if err != nil {
		return false
	}
	return strings.Contains(string(data), `"`+dshPluginID+`"`) &&
		containsBundle(data, dshPluginID)
}

func containsBundle(data []byte, id string) bool {
	var pkg struct {
		Bundles []string `json:"dsh.profile.bundles"`
	}
	if json.Unmarshal(data, &pkg) != nil {
		return strings.Contains(string(data), `"`+id+`"`)
	}
	for _, b := range pkg.Bundles {
		if b == id {
			return true
		}
	}
	return false
}

func (d *dsh) Enable(o Options) error {
	profiles := d.profileDirs()
	if len(profiles) == 0 {
		return fmt.Errorf("未找到 ~/.dsh/profiles 下的 profile")
	}
	version := d.bundledVersion()
	for _, profile := range profiles {
		pkgPath := filepath.Join(profile, "package.json")
		data, err := os.ReadFile(pkgPath)
		if err != nil {
			continue
		}
		_, _ = backupFile("dsh", pkgPath, data)
		if err := extractPlugin(filepath.Join(profile, "node_modules")); err != nil {
			return fmt.Errorf("profile %s 安装失败: %w", filepath.Base(profile), err)
		}
		var pkg map[string]any
		if json.Unmarshal(data, &pkg) != nil {
			continue
		}
		deps, _ := pkg["dependencies"].(map[string]any)
		if deps == nil {
			deps = map[string]any{}
		}
		deps[dshPluginID] = version
		pkg["dependencies"] = deps
		bundles, _ := pkg["dsh.profile.bundles"].([]any)
		has := false
		for _, b := range bundles {
			if s, ok := b.(string); ok && s == dshPluginID {
				has = true
			}
		}
		if !has {
			bundles = append(bundles, dshPluginID)
		}
		pkg["dsh.profile.bundles"] = bundles
		out, err := json.MarshalIndent(pkg, "", "  ")
		if err != nil {
			return err
		}
		if err := atomicWrite(pkgPath, out); err != nil {
			return err
		}
	}
	return nil
}

func (d *dsh) Disable() error {
	for _, profile := range d.profileDirs() {
		pkgPath := filepath.Join(profile, "package.json")
		data, err := os.ReadFile(pkgPath)
		if err != nil {
			continue
		}
		var pkg map[string]any
		if json.Unmarshal(data, &pkg) != nil {
			continue
		}
		if deps, ok := pkg["dependencies"].(map[string]any); ok {
			delete(deps, dshPluginID)
		}
		if bundles, ok := pkg["dsh.profile.bundles"].([]any); ok {
			kept := []any{}
			for _, b := range bundles {
				if s, ok := b.(string); ok && s == dshPluginID {
					continue
				}
				kept = append(kept, b)
			}
			pkg["dsh.profile.bundles"] = kept
		}
		out, err := json.MarshalIndent(pkg, "", "  ")
		if err != nil {
			return err
		}
		if err := atomicWrite(pkgPath, out); err != nil {
			return err
		}
	}
	return nil
}

// extractPlugin unpacks the bundled zip into <nodeModules>/dsh-our-free-model.
func extractPlugin(nodeModules string) error {
	dest := filepath.Join(nodeModules, dshPluginID)
	if err := os.RemoveAll(dest); err != nil {
		return err
	}
	if err := os.MkdirAll(dest, 0o755); err != nil {
		return err
	}
	zr, err := zip.NewReader(bytes.NewReader(dshPluginZip), int64(len(dshPluginZip)))
	if err != nil {
		return err
	}
	for _, f := range zr.File {
		rel := filepath.FromSlash(f.Name)
		// zip-slip guard
		if strings.Contains(rel, "..") {
			continue
		}
		target := filepath.Join(dest, strings.TrimPrefix(rel, dshPluginID+string(os.PathSeparator)))
		if f.FileInfo().IsDir() {
			_ = os.MkdirAll(target, 0o755)
			continue
		}
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return err
		}
		rc, err := f.Open()
		if err != nil {
			return err
		}
		data, err := io.ReadAll(rc)
		rc.Close()
		if err != nil {
			return err
		}
		if err := os.WriteFile(target, data, 0o644); err != nil {
			return err
		}
	}
	return nil
}

func (d *dsh) bundledVersion() string {
	zr, err := zip.NewReader(bytes.NewReader(dshPluginZip), int64(len(dshPluginZip)))
	if err != nil {
		return ""
	}
	for _, f := range zr.File {
		if f.Name == dshPluginID+"/package.json" {
			rc, err := f.Open()
			if err != nil {
				return ""
			}
			data, _ := io.ReadAll(rc)
			rc.Close()
			var pkg struct {
				Version string `json:"version"`
			}
			if json.Unmarshal(data, &pkg) == nil {
				return pkg.Version
			}
		}
	}
	return ""
}
