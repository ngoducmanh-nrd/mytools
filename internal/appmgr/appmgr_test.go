package appmgr

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"mytools/internal/core"
)

func TestBuiltinSpec(t *testing.T) {
	spec, ok := BuiltinSpec("yt-dlp")
	if !ok {
		t.Fatal("không có builtin yt-dlp")
	}
	if spec.SchemaVersion != 1 || spec.ID != "yt-dlp" || spec.Runtime != "binary" {
		t.Fatalf("spec builtin không đúng: %+v", spec)
	}
	if spec.Download == "" || len(spec.Entry) == 0 {
		t.Fatalf("spec builtin thiếu download/entry: %+v", spec)
	}
	if _, ok := BuiltinSpec("không-tồn-tại"); ok {
		t.Fatal("BuiltinSpec nhận id lạ")
	}
}

func TestAddLoadListAndState(t *testing.T) {
	dataDir := filepath.Join(t.TempDir(), "dữ liệu mt")
	mgr := New(dataDir)
	spec := testSpec("demo")

	app, err := mgr.Add(spec)
	if err != nil {
		t.Fatalf("Add: %v", err)
	}
	wantDir := filepath.Join(dataDir, "apps", "demo")
	if app.Dir != wantDir {
		t.Fatalf("Dir = %q, muốn %q", app.Dir, wantDir)
	}
	if _, err := os.Stat(filepath.Join(app.Dir, "app.toml")); err != nil {
		t.Fatalf("app.toml: %v", err)
	}
	if _, err := os.Stat(filepath.Join(app.Dir, "state.json")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("state.json không được tồn tại trước khi cài: %v", err)
	}

	loaded, err := mgr.LoadApp("demo")
	if err != nil {
		t.Fatalf("LoadApp: %v", err)
	}
	if loaded.ID != spec.ID || loaded.Runtime != spec.Runtime || loaded.Download != spec.Download {
		t.Fatalf("LoadApp fields = %+v", loaded.AppSpec)
	}
	if len(loaded.Entry) != 1 || loaded.Entry[0] != "tool.exe" {
		t.Fatalf("Entry = %v", loaded.Entry)
	}
	if !loaded.DownloadedAt.IsZero() || loaded.Sha256 != "" {
		t.Fatalf("state ban đầu phải rỗng: %+v", loaded.AppState)
	}

	apps, err := mgr.List()
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(apps) != 1 || apps[0].ID != "demo" || apps[0].Dir != wantDir {
		t.Fatalf("List = %+v", apps)
	}

	when := time.Now().UTC().Truncate(time.Millisecond)
	state := core.AppState{URL: spec.Download, Sha256: "abc123", DownloadedAt: when}
	if err := mgr.SaveState("demo", state); err != nil {
		t.Fatalf("SaveState: %v", err)
	}
	loaded, err = mgr.LoadApp("demo")
	if err != nil {
		t.Fatalf("LoadApp sau SaveState: %v", err)
	}
	if loaded.URL != state.URL || loaded.Sha256 != state.Sha256 {
		t.Fatalf("state = %+v", loaded.AppState)
	}
	if !loaded.DownloadedAt.Equal(when) {
		t.Fatalf("DownloadedAt = %v, muốn %v", loaded.DownloadedAt, when)
	}

	raw, err := os.ReadFile(filepath.Join(app.Dir, "state.json"))
	if err != nil {
		t.Fatalf("đọc state.json: %v", err)
	}
	var keys map[string]any
	if err := json.Unmarshal(raw, &keys); err != nil {
		t.Fatalf("state.json không phải JSON: %v", err)
	}
	for _, key := range []string{"url", "sha256", "downloaded_at"} {
		if _, ok := keys[key]; !ok {
			t.Fatalf("state.json thiếu %q: %s", key, raw)
		}
	}
	if _, err := os.Stat(filepath.Join(app.Dir, "state.json.tmp")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("còn file .tmp: %v", err)
	}
}

func TestListMissingAppsDir(t *testing.T) {
	mgr := New(t.TempDir())
	apps, err := mgr.List()
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(apps) != 0 {
		t.Fatalf("apps = %+v", apps)
	}
}

func TestListMissingAppTOML(t *testing.T) {
	dataDir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dataDir, "apps", "hong"), 0o700); err != nil {
		t.Fatal(err)
	}

	_, err := New(dataDir).List()
	if err == nil {
		t.Fatal("List phải lỗi khi app thiếu app.toml")
	}
	if !errors.Is(err, core.ErrAppConfigMissing) {
		t.Fatalf("lỗi phải bọc ErrAppConfigMissing: %v", err)
	}
}

func TestLoadAppIDMismatch(t *testing.T) {
	dataDir := t.TempDir()
	appDir := filepath.Join(dataDir, "apps", "demo")
	if err := os.MkdirAll(appDir, 0o700); err != nil {
		t.Fatal(err)
	}
	content := `schema_version = 1
id = "khac"
runtime = "binary"
download = "https://example.com/tool.exe"
entry = ["tool.exe"]
`
	if err := os.WriteFile(filepath.Join(appDir, "app.toml"), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	_, err := New(dataDir).LoadApp("demo")
	if err == nil || !strings.Contains(err.Error(), `khai báo id "khac"`) {
		t.Fatalf("lỗi không đúng: %v", err)
	}
}

func TestRemoveOnlyAppDir(t *testing.T) {
	dataDir := filepath.Join(t.TempDir(), "dữ liệu mt")
	mgr := New(dataDir)
	if _, err := mgr.Add(testSpec("demo")); err != nil {
		t.Fatalf("Add: %v", err)
	}

	keepDir := filepath.Join(dataDir, "apps", "keep")
	if err := os.MkdirAll(keepDir, 0o700); err != nil {
		t.Fatal(err)
	}
	keepFile := filepath.Join(keepDir, "keep.txt")
	if err := os.WriteFile(keepFile, []byte("giữ lại"), 0o644); err != nil {
		t.Fatal(err)
	}
	rootFile := filepath.Join(dataDir, "root.txt")
	if err := os.WriteFile(rootFile, []byte("giữ lại"), 0o644); err != nil {
		t.Fatal(err)
	}

	target := filepath.Join(dataDir, "apps", "demo")
	if err := mgr.Remove("demo"); err != nil {
		t.Fatalf("Remove: %v", err)
	}
	if _, err := os.Stat(target); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("thư mục app còn tồn tại: %v", err)
	}
	for _, path := range []string{keepFile, rootFile, filepath.Join(dataDir, "apps")} {
		if _, err := os.Stat(path); err != nil {
			t.Fatalf("Remove đụng nhầm %s: %v", path, err)
		}
	}
}

func TestRemoveRejectsTraversalAndInvalidIDs(t *testing.T) {
	dataDir := t.TempDir()
	mgr := New(dataDir)
	outside := filepath.Join(dataDir, "core")
	if err := os.MkdirAll(outside, 0o700); err != nil {
		t.Fatal(err)
	}
	keep := filepath.Join(outside, "keep.txt")
	if err := os.WriteFile(keep, []byte("không được xóa"), 0o644); err != nil {
		t.Fatal(err)
	}

	ids := []string{"", "..", ".", "../core", `..\core`, "a/b", `a\b`, "/abs", `C:\Windows`, "Demo", "-demo", "demo app"}
	for _, id := range ids {
		if err := mgr.Remove(id); err == nil {
			t.Fatalf("Remove(%q) phải lỗi", id)
		}
	}
	if _, err := os.Stat(keep); err != nil {
		t.Fatalf("Remove đã xóa ngoài phạm vi: %v", err)
	}
	if err := mgr.SaveState("../core", core.AppState{Sha256: "x"}); err == nil {
		t.Fatal("SaveState phải từ chối id traversal")
	}
	if _, err := os.Stat(filepath.Join(outside, "state.json")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("SaveState ghi ra ngoài phạm vi: %v", err)
	}
	if _, err := mgr.Add(testSpec("../core")); err == nil {
		t.Fatal("Add phải từ chối id traversal")
	}
	if _, err := os.Stat(filepath.Join(outside, "app.toml")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("Add ghi ra ngoài phạm vi: %v", err)
	}
}

func TestRemoveRejectsNonDirectory(t *testing.T) {
	dataDir := t.TempDir()
	appsDir := filepath.Join(dataDir, "apps")
	if err := os.MkdirAll(appsDir, 0o700); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(appsDir, "demo")
	if err := os.WriteFile(path, []byte("không phải thư mục"), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := New(dataDir).Remove("demo"); err == nil {
		t.Fatal("Remove phải từ chối file")
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("file bị đụng: %v", err)
	}
}

func TestAddRejectsInvalidSpecs(t *testing.T) {
	mgr := New(t.TempDir())

	cases := []struct {
		name string
		spec core.AppSpec
	}{
		{"id traversal", func() core.AppSpec { s := testSpec("../core"); return s }()},
		{"missing download", func() core.AppSpec { s := testSpec("demo"); s.Download = ""; return s }()},
		{"bad download", func() core.AppSpec { s := testSpec("demo"); s.Download = "ftp://example.com/tool.exe"; return s }()},
		{"entry traversal", func() core.AppSpec { s := testSpec("demo"); s.Entry = []string{"../tool.exe"}; return s }()},
		{"python runtime", func() core.AppSpec {
			s := testSpec("demo")
			s.Runtime = "python"
			s.Download = ""
			s.Source = "https://github.com/example/repo"
			s.Entry = []string{"python", "app.py"}
			return s
		}()},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := mgr.Add(tc.spec); err == nil {
				t.Fatal("Add phải lỗi")
			}
		})
	}
}

func testSpec(id string) core.AppSpec {
	return core.AppSpec{
		SchemaVersion: 1,
		ID:            id,
		Runtime:       "binary",
		Download:      "https://example.com/tool.exe",
		Entry:         []string{"tool.exe"},
	}
}
