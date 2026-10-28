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

func TestInstallGitWritesProvidedSpec(t *testing.T) {
	requireGit(t)
	repo := newGitRepo(t, map[string]string{"main.py": "print('hello')\n"})
	head := strings.TrimSpace(gitIn(t, repo, "rev-parse", "HEAD"))

	dataDir := t.TempDir()
	app, err := New(dataDir).InstallGit(context.Background(), repo, core.AppSpec{
		SchemaVersion: 1,
		ID:            "rea",
		Runtime:       "python",
		Entry:         []string{"python", "main.py"},
		GUI:           true,
	})
	if err != nil {
		t.Fatalf("InstallGit: %v", err)
	}
	if app.Source != repo || app.Ref != head {
		t.Fatalf("source/ref = %q/%q, muốn %q/%q", app.Source, app.Ref, repo, head)
	}
	if app.Runtime != "python" || !app.GUI {
		t.Fatalf("spec = %+v", app.AppSpec)
	}

	for _, name := range []string{"app.toml", "main.py"} {
		if _, err := os.Stat(filepath.Join(app.Dir, name)); err != nil {
			t.Fatalf("%s: %v", name, err)
		}
	}
	loaded, err := New(dataDir).LoadApp("rea")
	if err != nil {
		t.Fatalf("LoadApp: %v", err)
	}
	if loaded.Entry == nil || len(loaded.Entry) != 2 || loaded.Entry[1] != "main.py" {
		t.Fatalf("entry = %v", loaded.Entry)
	}

	entries, statErr := os.ReadDir(filepath.Join(dataDir, "apps"))
	if statErr != nil {
		t.Fatal(statErr)
	}
	if len(entries) != 1 || entries[0].Name() != "rea" {
		t.Fatalf("còn thư mục tạm: %v", entries)
	}
}

func TestInstallGitRejectsBadSpec(t *testing.T) {
	requireGit(t)
	repo := newGitRepo(t, map[string]string{"main.py": "print('hello')\n"})

	dataDir := t.TempDir()
	_, err := New(dataDir).InstallGit(context.Background(), repo, core.AppSpec{
		SchemaVersion: 1,
		ID:            "rea",
		Runtime:       "python",
		Entry:         []string{"main.py"},
	})
	if err == nil || !strings.Contains(err.Error(), "entry") {
		t.Fatalf("entry sai phải bị từ chối: %v", err)
	}
	entries, statErr := os.ReadDir(filepath.Join(dataDir, "apps"))
	if statErr != nil {
		t.Fatal(statErr)
	}
	if len(entries) != 0 {
		t.Fatalf("bản clone chưa được xóa: %v", entries)
	}
}

func TestInstallGitExistingApp(t *testing.T) {
	requireGit(t)
	repo := newGitRepo(t, map[string]string{"main.py": "print('hello')\n"})

	dataDir := t.TempDir()
	mgr := New(dataDir)
	if _, err := mgr.Add(testSpec("demo")); err != nil {
		t.Fatalf("Add: %v", err)
	}

	_, err := mgr.InstallGit(context.Background(), repo, core.AppSpec{
		SchemaVersion: 1,
		ID:            "demo",
		Runtime:       "python",
		Entry:         []string{"python", "main.py"},
	})
	if err == nil || !strings.Contains(err.Error(), "đã tồn tại") {
		t.Fatalf("lỗi không đúng: %v", err)
	}
	loaded, loadErr := mgr.LoadApp("demo")
	if loadErr != nil {
		t.Fatal(loadErr)
	}
	if loaded.Runtime != "binary" {
		t.Fatalf("app cũ bị ghi đè: %+v", loaded.AppSpec)
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

func TestValidateSpecPythonWithoutSource(t *testing.T) {
	spec := core.AppSpec{
		SchemaVersion: 1,
		ID:            "demo",
		Runtime:       "python",
		Entry:         []string{"python", "app.py"},
	}
	if err := validateSpec(spec); err != nil {
		t.Fatalf("python không cần source khi cài local: %v", err)
	}
}

func TestValidateSpecNode(t *testing.T) {
	valid := core.AppSpec{
		SchemaVersion: 1,
		ID:            "demo",
		Runtime:       "node",
		Entry:         []string{"npm", "start"},
		Build:         []string{"npm", "run", "build"},
	}
	if err := validateSpec(valid); err != nil {
		t.Fatalf("node hợp lệ bị từ chối: %v", err)
	}

	bad := []core.AppSpec{
		{SchemaVersion: 1, ID: "demo", Runtime: "node"},
		{SchemaVersion: 1, ID: "demo", Runtime: "node", Entry: []string{"../npm", "start"}},
		{SchemaVersion: 1, ID: "demo", Runtime: "node", Entry: []string{"npm", "start"}, Build: []string{"../npm", "run", "build"}},
		{SchemaVersion: 1, ID: "demo", Runtime: "python", Entry: []string{"python", "main.py"}, Build: []string{"npm", "run", "build"}},
	}
	for _, spec := range bad {
		if err := validateSpec(spec); err == nil {
			t.Fatalf("validateSpec(%+v) phải lỗi", spec)
		}
	}
}

func TestInstallLocalSkipsJunkDirs(t *testing.T) {
	srcDir := t.TempDir()
	files := map[string]string{
		"app.toml": `schema_version = 1
id = "music-app"
runtime = "node"
entry = ["npm", "start"]
build = ["npm", "run", "build"]
`,
		"package.json":                  `{"name":"music-app"}`,
		"src/main.ts":                   "export {};\n",
		"src/node_modules/pkg/index.js": "nested junk\n",
		"node_modules/pkg/index.js":     "junk\n",
		"dist/index.html":               "<html></html>\n",
		"build/out.txt":                 "junk\n",
		"target/debug.txt":              "junk\n",
		"venv/bin/tool":                 "junk\n",
		"env/lib/tool":                  "junk\n",
		".git/HEAD":                     "ref\n",
	}
	for name, content := range files {
		path := filepath.Join(srcDir, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	app, err := New(t.TempDir()).InstallLocal(srcDir)
	if err != nil {
		t.Fatalf("InstallLocal: %v", err)
	}
	if app.Runtime != "node" {
		t.Fatalf("Runtime = %q, muốn node", app.Runtime)
	}
	if strings.Join(app.Build, " ") != "npm run build" {
		t.Fatalf("Build = %v, muốn [npm run build]", app.Build)
	}

	for _, name := range []string{"package.json", "src/main.ts"} {
		if _, err := os.Stat(filepath.Join(app.Dir, filepath.FromSlash(name))); err != nil {
			t.Fatalf("thiếu source %s: %v", name, err)
		}
	}
	for _, name := range []string{"node_modules", "dist", "build", "target", "venv", "env", ".git", "src/node_modules"} {
		if _, err := os.Stat(filepath.Join(app.Dir, filepath.FromSlash(name))); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("thư mục rác %s vẫn được copy: %v", name, err)
		}
	}
}

func TestInstallLocal(t *testing.T) {
	srcDir := t.TempDir()
	files := map[string]string{
		"app.toml": `schema_version = 1
id = "cli-tool"
runtime = "python"
entry = ["python", "cli_tool.py"]
`,
		"cli_tool.py":        "print('xin chào')\n",
		"data/danh sách.txt": "1\n",
	}
	for name, content := range files {
		path := filepath.Join(srcDir, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	dataDir := filepath.Join(t.TempDir(), "dữ liệu mt")
	mgr := New(dataDir)
	app, err := mgr.InstallLocal(srcDir)
	if err != nil {
		t.Fatalf("InstallLocal: %v", err)
	}
	if app.ID != "cli-tool" || app.Runtime != "python" || app.GUI {
		t.Fatalf("app không đúng: %+v", app.AppSpec)
	}

	for name := range files {
		if name == "app.toml" {
			continue // app.toml được viết chuẩn hóa lại khi cài
		}
		path := filepath.Join(app.Dir, filepath.FromSlash(name))
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("thiếu file %s trong app đã cài: %v", name, err)
		}
		if string(data) != files[name] {
			t.Errorf("nội dung %s sai: %q", name, data)
		}
	}

	loaded, err := mgr.LoadApp("cli-tool")
	if err != nil {
		t.Fatalf("LoadApp: %v", err)
	}
	if loaded.Source != "" {
		t.Errorf("Source = %q, muốn rỗng", loaded.Source)
	}

	if _, err := mgr.InstallLocal(srcDir); err == nil || !strings.Contains(err.Error(), "đã tồn tại") {
		t.Fatalf("cài lại phải báo đã tồn tại: %v", err)
	}
}

func TestInstallLocalRejectsMissingAppTOML(t *testing.T) {
	srcDir := t.TempDir()
	_, err := New(t.TempDir()).InstallLocal(srcDir)
	if err == nil || !strings.Contains(err.Error(), "app.toml") {
		t.Fatalf("thiếu app.toml phải báo lỗi rõ: %v", err)
	}
}

func TestInstallLocalRejectsBadID(t *testing.T) {
	srcDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(srcDir, "app.toml"), []byte(`schema_version = 1
id = "../ngoai"
runtime = "python"
entry = ["python", "app.py"]
`), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err := New(t.TempDir()).InstallLocal(srcDir)
	if err == nil || !strings.Contains(err.Error(), "id app") {
		t.Fatalf("id nguy hiểm phải bị từ chối: %v", err)
	}
}
