package appmgr

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
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

func TestCloneGitPythonApp(t *testing.T) {
	requireGit(t)
	repo := newGitRepo(t, map[string]string{
		"app.toml": `schema_version = 1
id = "hello-py"
runtime = "python"
requirements = "requirements.txt"
entry = ["python", "main.py"]
`,
		"requirements.txt": "requests\n",
		"main.py":          "print('hello')\n",
	})
	head := strings.TrimSpace(gitIn(t, repo, "rev-parse", "HEAD"))

	dataDir := filepath.Join(t.TempDir(), "dữ liệu mt")
	mgr := New(dataDir)
	app, err := mgr.CloneGit(context.Background(), repo, "")
	if err != nil {
		t.Fatalf("CloneGit: %v", err)
	}
	if app.ID != "hello-py" || app.Runtime != "python" {
		t.Fatalf("app = %+v", app.AppSpec)
	}
	if app.Source != repo {
		t.Fatalf("Source = %q, muốn %q", app.Source, repo)
	}
	if app.Ref != head || len(head) < 7 {
		t.Fatalf("Ref = %q, HEAD = %q", app.Ref, head)
	}
	for _, name := range []string{"app.toml", "requirements.txt", "main.py"} {
		if _, err := os.Stat(filepath.Join(app.Dir, name)); err != nil {
			t.Fatalf("%s: %v", name, err)
		}
	}

	gotHead, err := mgr.HeadCommit(context.Background(), "hello-py")
	if err != nil || gotHead != head {
		t.Fatalf("HeadCommit = %q, %v; muốn %q", gotHead, err, head)
	}

	loaded, err := mgr.LoadApp("hello-py")
	if err != nil {
		t.Fatalf("LoadApp: %v", err)
	}
	if loaded.Source != repo || loaded.Ref != head {
		t.Fatalf("app.toml chưa ghi source/ref: %+v", loaded.AppSpec)
	}

	entries, err := os.ReadDir(filepath.Join(dataDir, "apps"))
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 || entries[0].Name() != "hello-py" {
		t.Fatalf("còn thư mục tạm: %v", entries)
	}
}

func TestCloneGitMissingAppTOML(t *testing.T) {
	requireGit(t)
	repo := newGitRepo(t, map[string]string{"main.py": "print('hello')\n"})

	dataDir := t.TempDir()
	_, err := New(dataDir).CloneGit(context.Background(), repo, "")
	if !errors.Is(err, core.ErrAppConfigMissing) {
		t.Fatalf("lỗi phải bọc ErrAppConfigMissing: %v", err)
	}
	entries, statErr := os.ReadDir(filepath.Join(dataDir, "apps"))
	if statErr != nil {
		t.Fatalf("đọc apps: %v", statErr)
	}
	if len(entries) != 0 {
		t.Fatalf("bản clone chưa được xóa: %v", entries)
	}
}

func TestCloneGitExistingApp(t *testing.T) {
	requireGit(t)
	repo := newGitRepo(t, map[string]string{
		"app.toml": `schema_version = 1
id = "demo"
runtime = "python"
entry = ["python", "main.py"]
`,
		"main.py": "print('hello')\n",
	})

	dataDir := t.TempDir()
	mgr := New(dataDir)
	if _, err := mgr.Add(testSpec("demo")); err != nil {
		t.Fatalf("Add: %v", err)
	}

	_, err := mgr.CloneGit(context.Background(), repo, "")
	if err == nil || !strings.Contains(err.Error(), "đã tồn tại") {
		t.Fatalf("lỗi không đúng: %v", err)
	}
	entries, statErr := os.ReadDir(filepath.Join(dataDir, "apps"))
	if statErr != nil {
		t.Fatal(statErr)
	}
	if len(entries) != 1 || entries[0].Name() != "demo" {
		t.Fatalf("apps = %v", entries)
	}
	loaded, err := mgr.LoadApp("demo")
	if err != nil {
		t.Fatalf("LoadApp: %v", err)
	}
	if loaded.Runtime != "binary" {
		t.Fatalf("app cũ bị ghi đè: %+v", loaded.AppSpec)
	}
}

func TestCloneGitRef(t *testing.T) {
	requireGit(t)
	repo := newGitRepo(t, map[string]string{
		"app.toml": `schema_version = 1
id = "hello-py"
runtime = "python"
entry = ["python", "main.py"]
`,
		"main.py": "print('v1')\n",
	})
	first := strings.TrimSpace(gitIn(t, repo, "rev-parse", "HEAD"))

	if err := os.WriteFile(filepath.Join(repo, "main.py"), []byte("print('v2')\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	gitIn(t, repo, "add", ".")
	gitIn(t, repo, "-c", "user.email=test@example.com", "-c", "user.name=Test", "commit", "--quiet", "-m", "v2")
	second := strings.TrimSpace(gitIn(t, repo, "rev-parse", "HEAD"))
	if first == second {
		t.Fatal("hai commit phải khác nhau")
	}

	mgr := New(t.TempDir())
	app, err := mgr.CloneGit(context.Background(), repo, first)
	if err != nil {
		t.Fatalf("CloneGit: %v", err)
	}
	if app.Ref != first {
		t.Fatalf("Ref = %q, muốn %q", app.Ref, first)
	}
	data, err := os.ReadFile(filepath.Join(app.Dir, "main.py"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "v1") {
		t.Fatalf("main.py = %q, phải là bản v1", data)
	}
	head, err := mgr.HeadCommit(context.Background(), app.ID)
	if err != nil || head != first {
		t.Fatalf("HeadCommit = %q, %v", head, err)
	}

	if _, err := mgr.CloneGit(context.Background(), repo, "-nguy-hiem"); err == nil {
		t.Fatal("CloneGit phải từ chối ref bắt đầu bằng -")
	}
}

func TestListSkipsCloneStaging(t *testing.T) {
	dataDir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dataDir, "apps", ".clone-abc"), 0o700); err != nil {
		t.Fatal(err)
	}
	apps, err := New(dataDir).List()
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(apps) != 0 {
		t.Fatalf("apps = %+v", apps)
	}
}

func TestRebuildRejectsBinary(t *testing.T) {
	mgr := New(t.TempDir())
	if _, err := mgr.Add(testSpec("demo")); err != nil {
		t.Fatalf("Add: %v", err)
	}
	err := mgr.Rebuild(context.Background(), "demo", nil)
	if err == nil || !strings.Contains(err.Error(), "chỉ hỗ trợ runtime python") {
		t.Fatalf("lỗi không đúng: %v", err)
	}
}

func requireGit(t *testing.T) {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("không có git trong PATH")
	}
}

func newGitRepo(t *testing.T, files map[string]string) string {
	t.Helper()
	dir := t.TempDir()
	for name, content := range files {
		path := filepath.Join(dir, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	gitIn(t, dir, "init", "--quiet")
	gitIn(t, dir, "add", ".")
	gitIn(t, dir, "-c", "user.email=test@example.com", "-c", "user.name=Test", "commit", "--quiet", "-m", "init")
	return dir
}

func gitIn(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
	cmd.Env = append(os.Environ(), "GIT_TERMINAL_PROMPT=0")
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, output)
	}
	return string(output)
}
