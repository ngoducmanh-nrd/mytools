package service

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"mytools/internal/core"
)

const testBinaryPayload = "MT-BINARY-PAYLOAD-1\n"

func startBinaryServer(t *testing.T) *httptest.Server {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/tool.exe" && r.URL.Path != "/standalone.exe" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/octet-stream")
		_, _ = w.Write([]byte(testBinaryPayload))
	}))
	t.Cleanup(server.Close)
	return server
}

func localToolDir(t *testing.T, server *httptest.Server, id, exeName string) string {
	t.Helper()
	dir := t.TempDir()
	appTOML := "schema_version = 1\nid = \"" + id + "\"\nruntime = \"binary\"\ndownload = \"" + server.URL + "/tool.exe\"\nentry = [\"" + exeName + "\"]\n"
	if err := os.WriteFile(filepath.Join(dir, "app.toml"), []byte(appTOML), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "ghi chú.txt"), []byte("nội dung phụ\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	return dir
}

func waitInstallResult(t *testing.T, s *Service, runID string) core.Event {
	t.Helper()
	ch, _, err := s.Subscribe(runID)
	if err != nil {
		t.Fatalf("Subscribe: %v", err)
	}
	deadline := time.Now().Add(30 * time.Second)
	var kinds []string
	for {
		select {
		case event, ok := <-ch:
			if !ok {
				t.Fatal("kênh đóng sớm")
			}
			if event.Kind == "result" {
				return event
			}
			kinds = append(kinds, event.Kind+":"+event.Text)
		case <-time.After(time.Until(deadline)):
			t.Fatalf("hết giờ chờ result; events=%v", kinds)
		}
	}
}

func TestInstallToolLocalDir(t *testing.T) {
	s := newTestService(t)
	server := startBinaryServer(t)
	srcDir := localToolDir(t, server, "web-tool", "web-tool.exe")

	runID, err := s.InstallTool(context.Background(), srcDir)
	if err != nil {
		t.Fatalf("InstallTool: %v", err)
	}
	result := waitInstallResult(t, s, runID)
	if result.Data["ok"] != true || result.Data["id"] != "web-tool" {
		t.Fatalf("result = %+v", result)
	}

	tools, err := s.ListTools()
	if err != nil {
		t.Fatalf("ListTools: %v", err)
	}
	got := findTool(tools, "web-tool")
	if got == nil || got.Kind != "app" || got.App != "web-tool" || got.Name != "web-tool" {
		t.Fatalf("tool web-tool = %+v", got)
	}

	app, err := s.mgr.LoadApp("web-tool")
	if err != nil {
		t.Fatalf("LoadApp: %v", err)
	}
	if app.Sha256 == "" {
		t.Error("state.json thiếu sha256")
	}
	data, err := os.ReadFile(filepath.Join(s.dir, "apps", "web-tool", "web-tool.exe"))
	if err != nil {
		t.Fatalf("đọc binary đã cài: %v", err)
	}
	if string(data) != testBinaryPayload {
		t.Errorf("binary = %q", data)
	}
	if _, err := os.Stat(filepath.Join(s.dir, "apps", "web-tool", "ghi chú.txt")); err != nil {
		t.Errorf("thiếu file phụ được copy: %v", err)
	}
}

func TestInstallToolDirectBinaryURL(t *testing.T) {
	s := newTestService(t)
	server := startBinaryServer(t)

	runID, err := s.InstallTool(context.Background(), server.URL+"/standalone.exe")
	if err != nil {
		t.Fatalf("InstallTool: %v", err)
	}
	result := waitInstallResult(t, s, runID)
	if result.Data["ok"] != true || result.Data["id"] != "standalone" {
		t.Fatalf("result = %+v", result)
	}

	tools, err := s.ListTools()
	if err != nil {
		t.Fatalf("ListTools: %v", err)
	}
	if findTool(tools, "standalone") == nil {
		t.Fatalf("thiếu tool standalone trong %+v", tools)
	}
	if _, err := os.Stat(filepath.Join(s.dir, "apps", "standalone", "standalone.exe")); err != nil {
		t.Errorf("thiếu binary standalone: %v", err)
	}
}

func TestInstallToolGitRepo(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("không có git trong PATH")
	}
	s := newTestService(t)
	server := startBinaryServer(t)

	repo := t.TempDir()
	if err := os.WriteFile(filepath.Join(repo, "app.toml"), []byte(
		"schema_version = 1\nid = \"git-tool\"\nruntime = \"binary\"\ndownload = \""+server.URL+"/tool.exe\"\nentry = [\"git-tool.exe\"]\n",
	), 0o644); err != nil {
		t.Fatal(err)
	}
	gitIn(t, repo, "init", "--quiet")
	gitIn(t, repo, "add", ".")
	gitIn(t, repo, "-c", "user.email=test@example.com", "-c", "user.name=Test", "commit", "--quiet", "-m", "init")

	// Dùng URL file:// để tránh bị phân loại thành thư mục tool cục bộ
	// (repo chứa app.toml nên IsLocalToolDir sẽ bắt trước nếu đưa đường dẫn thô).
	source := "file://" + filepath.ToSlash(repo)
	if runtime.GOOS == "windows" {
		source = "file:///" + filepath.ToSlash(repo)
	}

	runID, err := s.InstallTool(context.Background(), source)
	if err != nil {
		t.Fatalf("InstallTool: %v", err)
	}
	result := waitInstallResult(t, s, runID)
	if result.Data["ok"] != true || result.Data["id"] != "git-tool" {
		t.Fatalf("result = %+v", result)
	}

	tools, err := s.ListTools()
	if err != nil {
		t.Fatalf("ListTools: %v", err)
	}
	if findTool(tools, "git-tool") == nil {
		t.Fatalf("thiếu tool git-tool trong %+v", tools)
	}
	app, err := s.mgr.LoadApp("git-tool")
	if err != nil {
		t.Fatalf("LoadApp: %v", err)
	}
	if app.Source != source || app.Ref == "" {
		t.Errorf("app = %+v", app)
	}
}

func TestInstallToolDuplicateApp(t *testing.T) {
	s := newTestService(t)
	server := startBinaryServer(t)
	srcDir := localToolDir(t, server, "web-tool", "web-tool.exe")

	first, err := s.InstallTool(context.Background(), srcDir)
	if err != nil {
		t.Fatalf("InstallTool lần 1: %v", err)
	}
	if result := waitInstallResult(t, s, first); result.Data["ok"] != true {
		t.Fatalf("lần 1 = %+v", result)
	}

	second, err := s.InstallTool(context.Background(), srcDir)
	if err != nil {
		t.Fatalf("InstallTool lần 2: %v", err)
	}
	result := waitInstallResult(t, s, second)
	if result.Data["ok"] != false {
		t.Fatalf("lần 2 phải thất bại: %+v", result)
	}

	tools, err := s.ListTools()
	if err != nil {
		t.Fatalf("ListTools: %v", err)
	}
	count := 0
	for _, tool := range tools {
		if tool.ID == "web-tool" {
			count++
		}
	}
	if count != 1 {
		t.Fatalf("registry có %d tool web-tool, want 1", count)
	}
}

func TestInstallToolExistingRegistryEntry(t *testing.T) {
	dir := t.TempDir()
	content := testToolsTOML + `
[[tool]]
id = "web-tool"
name = "Web Tool"
kind = "app"
app = "web-tool"
`
	if err := os.WriteFile(filepath.Join(dir, "tools.toml"), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	s := New(dir)
	t.Cleanup(s.Close)

	server := startBinaryServer(t)
	srcDir := localToolDir(t, server, "web-tool", "web-tool.exe")

	runID, err := s.InstallTool(context.Background(), srcDir)
	if err != nil {
		t.Fatalf("InstallTool: %v", err)
	}
	result := waitInstallResult(t, s, runID)
	if result.Data["ok"] != true || result.Data["id"] != "web-tool" {
		t.Fatalf("result = %+v", result)
	}

	data, err := os.ReadFile(filepath.Join(dir, "tools.toml"))
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.Count(string(data), "id = \"web-tool\""); got != 1 {
		t.Errorf("registry có %d entry web-tool, want 1:\n%s", got, data)
	}
}

func TestInstallToolEmptySource(t *testing.T) {
	s := newTestService(t)
	if _, err := s.InstallTool(context.Background(), "   "); err == nil || !strings.Contains(err.Error(), "thiếu nguồn cài đặt") {
		t.Fatalf("nguồn rỗng: %v", err)
	}
}

func waitInstallEvents(t *testing.T, s *Service, runID string) []core.Event {
	t.Helper()
	ch, _, err := s.Subscribe(runID)
	if err != nil {
		t.Fatalf("Subscribe: %v", err)
	}
	deadline := time.Now().Add(30 * time.Second)
	var events []core.Event
	for {
		select {
		case event, ok := <-ch:
			if !ok {
				t.Fatal("kênh đóng sớm")
			}
			events = append(events, event)
			if event.Kind == "result" {
				return events
			}
		case <-time.After(time.Until(deadline)):
			t.Fatalf("hết giờ chờ result; events=%v", events)
		}
	}
}

func TestInstallToolGitRepoMissingAppTOML(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("không có git trong PATH")
	}
	s := newTestService(t)

	repo := t.TempDir()
	if err := os.WriteFile(filepath.Join(repo, "main.py"), []byte("print('hi')\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	gitIn(t, repo, "init", "--quiet")
	gitIn(t, repo, "add", ".")
	gitIn(t, repo, "-c", "user.email=test@example.com", "-c", "user.name=Test", "commit", "--quiet", "-m", "init")

	source := "file://" + filepath.ToSlash(repo)
	if runtime.GOOS == "windows" {
		source = "file:///" + filepath.ToSlash(repo)
	}

	runID, err := s.InstallTool(context.Background(), source)
	if err != nil {
		t.Fatalf("InstallTool: %v", err)
	}
	events := waitInstallEvents(t, s, runID)

	var result *core.Event
	sawManual := false
	for i := range events {
		switch events[i].Kind {
		case "error":
			t.Errorf("không được có event error: %+v", events[i])
		case "manual":
			sawManual = true
			if events[i].Data["source"] != source {
				t.Errorf("manual.source = %v, muốn %q", events[i].Data["source"], source)
			}
		case "result":
			result = &events[i]
		}
	}
	if !sawManual {
		t.Error("thiếu event kind=manual")
	}
	if result == nil || result.Data["manual"] != true || result.Data["ok"] != false {
		t.Fatalf("result = %+v", result)
	}

	entries, statErr := os.ReadDir(filepath.Join(s.dir, "apps"))
	if statErr != nil && !os.IsNotExist(statErr) {
		t.Fatalf("đọc apps: %v", statErr)
	}
	for _, entry := range entries {
		if !strings.HasPrefix(entry.Name(), ".") {
			t.Errorf("thư mục app còn sót sau khi báo thiếu app.toml: %s", entry.Name())
		}
	}
}

func TestInstallManualToolGitRepo(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("không có git trong PATH")
	}
	s := newTestService(t)

	repo := t.TempDir()
	if err := os.WriteFile(filepath.Join(repo, "main.py"), []byte("print('hi')\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	gitIn(t, repo, "init", "--quiet")
	gitIn(t, repo, "add", ".")
	gitIn(t, repo, "-c", "user.email=test@example.com", "-c", "user.name=Test", "commit", "--quiet", "-m", "init")

	source := "file://" + filepath.ToSlash(repo)
	if runtime.GOOS == "windows" {
		source = "file:///" + filepath.ToSlash(repo)
	}

	runID, err := s.InstallManualTool(context.Background(), source, "Rea", "python main.py", true)
	if err != nil {
		t.Fatalf("InstallManualTool: %v", err)
	}
	result := waitInstallResult(t, s, runID)
	if result.Data["ok"] != true || result.Data["id"] != "rea" {
		t.Fatalf("result = %+v", result)
	}

	tools, err := s.ListTools()
	if err != nil {
		t.Fatalf("ListTools: %v", err)
	}
	tool := findTool(tools, "rea")
	if tool == nil || tool.Kind != "app" || tool.App != "rea" || tool.Name != "Rea" {
		t.Fatalf("tool rea = %+v", tool)
	}

	app, err := s.mgr.LoadApp("rea")
	if err != nil {
		t.Fatalf("LoadApp: %v", err)
	}
	if app.Source != source || app.Ref == "" {
		t.Errorf("source/ref = %q/%q", app.Source, app.Ref)
	}
	if app.Runtime != "python" || len(app.Entry) != 2 || app.Entry[0] != "python" || app.Entry[1] != "main.py" {
		t.Errorf("runtime/entry = %q/%v", app.Runtime, app.Entry)
	}
	if !app.GUI {
		t.Error("app.GUI phải được ghi vào app.toml")
	}
	if _, err := os.Stat(filepath.Join(s.dir, "apps", "rea", "main.py")); err != nil {
		t.Errorf("thiếu main.py trong app: %v", err)
	}
	if _, err := os.Stat(filepath.Join(s.dir, "apps", "rea", "env")); err != nil {
		t.Errorf("thiếu venv: %v", err)
	}
}

func TestInstallManualToolBadInput(t *testing.T) {
	s := newTestService(t)

	cases := []struct {
		source, name, entry string
		gui                 bool
		want                string
	}{
		{source: "   ", name: "Rea", entry: "python main.py", want: "thiếu nguồn cài đặt"},
		{source: "https://github.com/x/y.git", name: "Rea", entry: "", want: "lệnh chạy phải có dạng"},
		{source: "https://github.com/x/y.git", name: "Rea", entry: "py main.py", want: `phải bắt đầu bằng "python"`},
		{source: "https://github.com/x/y.git", name: "Rea", entry: "python", want: "lệnh chạy phải có dạng"},
		{source: "https://github.com/x/y.git", name: "###", entry: "python main.py", want: "không tạo được id app"},
	}
	for _, tc := range cases {
		if _, err := s.InstallManualTool(context.Background(), tc.source, tc.name, tc.entry, tc.gui); err == nil || !strings.Contains(err.Error(), tc.want) {
			t.Errorf("InstallManualTool(%q, %q, %q) = %v, muốn chứa %q", tc.source, tc.name, tc.entry, err, tc.want)
		}
	}
}

func TestRemoveTool(t *testing.T) {
	s := newTestService(t)

	if err := s.RemoveTool("movies"); err != nil {
		t.Fatalf("RemoveTool movies: %v", err)
	}
	tools, err := s.ListTools()
	if err != nil {
		t.Fatalf("ListTools: %v", err)
	}
	if len(tools) != 1 || tools[0].ID != "dl" {
		t.Fatalf("tools = %+v", tools)
	}

	if err := s.RemoveTool("dl"); err != nil {
		t.Fatalf("RemoveTool dl: %v", err)
	}
	tools, err = s.ListTools()
	if err != nil {
		t.Fatalf("ListTools: %v", err)
	}
	if len(tools) != 0 {
		t.Fatalf("tools = %+v", tools)
	}

	if err := s.RemoveTool("khong-co"); err == nil || !strings.Contains(err.Error(), `không có tool "khong-co"`) {
		t.Errorf("tool lạ: %v", err)
	}
	if err := s.RemoveTool(""); err == nil || !strings.Contains(err.Error(), "thiếu id tool") {
		t.Errorf("id rỗng: %v", err)
	}
}

func TestRemoveToolRemovesInstalledApp(t *testing.T) {
	s := newTestService(t)
	server := startBinaryServer(t)
	srcDir := localToolDir(t, server, "web-tool", "web-tool.exe")

	runID, err := s.InstallTool(context.Background(), srcDir)
	if err != nil {
		t.Fatalf("InstallTool: %v", err)
	}
	if result := waitInstallResult(t, s, runID); result.Data["ok"] != true {
		t.Fatalf("install = %+v", result)
	}

	if err := s.RemoveTool("web-tool"); err != nil {
		t.Fatalf("RemoveTool: %v", err)
	}
	if _, err := os.Stat(filepath.Join(s.dir, "apps", "web-tool")); !os.IsNotExist(err) {
		t.Errorf("thư mục app vẫn còn: %v", err)
	}
	tools, err := s.ListTools()
	if err != nil {
		t.Fatalf("ListTools: %v", err)
	}
	if findTool(tools, "web-tool") != nil {
		t.Fatal("tool web-tool vẫn còn trong registry")
	}
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

func fileURL(t *testing.T, path string) string {
	t.Helper()
	raw := "file://" + filepath.ToSlash(path)
	if runtime.GOOS == "windows" {
		raw = "file:///" + filepath.ToSlash(path)
	}
	return raw
}

func newPythonRepo(t *testing.T, files map[string]string) string {
	t.Helper()
	repo := t.TempDir()
	for name, content := range files {
		path := filepath.Join(repo, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	gitIn(t, repo, "init", "--quiet")
	gitIn(t, repo, "add", ".")
	gitIn(t, repo, "-c", "user.email=test@example.com", "-c", "user.name=Test", "commit", "--quiet", "-m", "init")
	return repo
}

func TestInstallCustomToolGit(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("không có git trong PATH")
	}
	s := newTestService(t)
	repo := newPythonRepo(t, map[string]string{"main.py": "print('hi')\n"})
	source := fileURL(t, repo)

	runID, err := s.InstallCustomTool(context.Background(), "Rea Tool", "Rea Tool", "git", source, "main.py", []string{"ai", "python"}, true)
	if err != nil {
		t.Fatalf("InstallCustomTool: %v", err)
	}
	events := waitInstallEvents(t, s, runID)
	var result *core.Event
	for i := range events {
		if events[i].Kind == "result" {
			result = &events[i]
		}
	}
	if result == nil || result.Data["ok"] != true || result.Data["id"] != "rea-tool" {
		t.Fatalf("result = %+v", result)
	}

	app, err := s.mgr.LoadApp("rea-tool")
	if err != nil {
		t.Fatalf("LoadApp: %v", err)
	}
	if app.Runtime != "python" || len(app.Entry) != 2 || app.Entry[0] != "python" || app.Entry[1] != "main.py" || app.GUI {
		t.Errorf("app spec = %+v", app.AppSpec)
	}
	if _, err := os.Stat(filepath.Join(s.dir, "apps", "rea-tool", "env")); err != nil {
		t.Errorf("thiếu venv: %v", err)
	}

	tools, err := s.ListTools()
	if err != nil {
		t.Fatalf("ListTools: %v", err)
	}
	got := findTool(tools, "rea-tool")
	if got == nil || got.Name != "Rea Tool" || got.Kind != "app" || got.App != "rea-tool" {
		t.Fatalf("tool = %+v", got)
	}
	if len(got.Tags) != 2 || got.Tags[0] != "ai" || got.Tags[1] != "python" {
		t.Errorf("tags = %v", got.Tags)
	}
}

func TestInstallCustomToolGitGUIEntryWithPythonPrefix(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("không có git trong PATH")
	}
	s := newTestService(t)
	repo := newPythonRepo(t, map[string]string{"app.py": "print('gui')\n"})
	source := fileURL(t, repo)

	runID, err := s.InstallCustomTool(context.Background(), "gui-tool", "GUI Tool", "gui", source, "python app.py", nil, true)
	if err != nil {
		t.Fatalf("InstallCustomTool: %v", err)
	}
	events := waitInstallEvents(t, s, runID)
	last := events[len(events)-1]
	if last.Kind != "result" || last.Data["ok"] != true {
		t.Fatalf("events = %+v", events)
	}
	app, err := s.mgr.LoadApp("gui-tool")
	if err != nil {
		t.Fatalf("LoadApp: %v", err)
	}
	if !app.GUI || app.Entry[1] != "app.py" {
		t.Errorf("app spec = %+v", app.AppSpec)
	}
}

func TestInstallCustomToolGitNoVenv(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("không có git trong PATH")
	}
	s := newTestService(t)
	repo := newPythonRepo(t, map[string]string{"main.py": "print('hi')\n"})

	runID, err := s.InstallCustomTool(context.Background(), "no-venv", "No Venv", "cli", fileURL(t, repo), "main.py", nil, false)
	if err != nil {
		t.Fatalf("InstallCustomTool: %v", err)
	}
	events := waitInstallEvents(t, s, runID)
	last := events[len(events)-1]
	if last.Kind != "result" || last.Data["ok"] != true {
		t.Fatalf("events = %+v", events)
	}
	if _, err := os.Stat(filepath.Join(s.dir, "apps", "no-venv")); err != nil {
		t.Fatalf("thiếu thư mục app: %v", err)
	}
	if _, err := os.Stat(filepath.Join(s.dir, "apps", "no-venv", "env")); !os.IsNotExist(err) {
		t.Errorf("venv vẫn được tạo dù người dùng bỏ chọn: %v", err)
	}
}

func TestInstallCustomToolBinary(t *testing.T) {
	s := newTestService(t)
	server := startBinaryServer(t)

	runID, err := s.InstallCustomTool(context.Background(), "my-tool", "My Tool", "binary", server.URL+"/standalone.exe", "", []string{"bin"}, true)
	if err != nil {
		t.Fatalf("InstallCustomTool: %v", err)
	}
	events := waitInstallEvents(t, s, runID)
	last := events[len(events)-1]
	if last.Kind != "result" || last.Data["ok"] != true || last.Data["id"] != "my-tool" {
		t.Fatalf("events = %+v", events)
	}

	app, err := s.mgr.LoadApp("my-tool")
	if err != nil {
		t.Fatalf("LoadApp: %v", err)
	}
	if app.Runtime != "binary" || app.Download != server.URL+"/standalone.exe" || app.Entry[0] != "standalone.exe" {
		t.Errorf("app spec = %+v", app.AppSpec)
	}
	if _, err := os.Stat(filepath.Join(s.dir, "apps", "my-tool", "standalone.exe")); err != nil {
		t.Errorf("thiếu binary đã tải: %v", err)
	}

	tools, err := s.ListTools()
	if err != nil {
		t.Fatalf("ListTools: %v", err)
	}
	got := findTool(tools, "my-tool")
	if got == nil || got.Name != "My Tool" || got.Kind != "app" {
		t.Fatalf("tool = %+v", got)
	}
	if len(got.Tags) != 1 || got.Tags[0] != "bin" {
		t.Errorf("tags = %v", got.Tags)
	}
}

func TestInstallCustomToolURL(t *testing.T) {
	s := newTestService(t)

	runID, err := s.InstallCustomTool(context.Background(), "docs", "Tài liệu", "url", "https://example.com/docs", "", []string{"web"}, true)
	if err != nil {
		t.Fatalf("InstallCustomTool: %v", err)
	}
	if runID != "" {
		t.Errorf("url tool không cần run, runID = %q", runID)
	}

	tools, err := s.ListTools()
	if err != nil {
		t.Fatalf("ListTools: %v", err)
	}
	got := findTool(tools, "docs")
	if got == nil || got.Kind != "url" || got.Target != "https://example.com/docs" || got.Name != "Tài liệu" {
		t.Fatalf("tool = %+v", got)
	}
	if len(got.Tags) != 1 || got.Tags[0] != "web" {
		t.Errorf("tags = %v", got.Tags)
	}
}

func TestInstallCustomToolBadInput(t *testing.T) {
	s := newTestService(t)
	cases := []struct {
		name, id, kind, target, entry, want string
	}{
		{"thiếu target", "x-tool", "git", "  ", "main.py", "thiếu nguồn cài đặt"},
		{"kind lạ", "x-tool", "docker", "https://github.com/x/y.git", "main.py", "không hợp lệ"},
		{"thiếu entry", "x-tool", "git", "https://github.com/x/y.git", "", "thiếu lệnh chạy"},
		{"id rỗng", "", "git", "https://github.com/x/y.git", "main.py", "id app"},
		{"id bắt đầu bằng số", "9tool", "git", "https://github.com/x/y.git", "main.py", "id app"},
		{"url sai", "x-tool", "url", "không-phải-url", "", "http/https"},
	}
	for _, tc := range cases {
		if _, err := s.InstallCustomTool(context.Background(), tc.id, "X", tc.kind, tc.target, tc.entry, nil, true); err == nil || !strings.Contains(err.Error(), tc.want) {
			t.Errorf("%s: err = %v, muốn chứa %q", tc.name, err, tc.want)
		}
	}
}
