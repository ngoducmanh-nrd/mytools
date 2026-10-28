package service

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"mytools/internal/appmgr"
	"mytools/internal/core"
	"mytools/internal/runner"
)

const testToolsTOML = `schema_version = 1

[[tool]]
id = "movies"
name = "Movie Tracker"
kind = "url"
target = "https://movie-tracker-v2-jade.vercel.app/"

[[tool]]
id = "dl"
name = "Tải video"
kind = "app"
app = "yt-dlp"
args = ["{url}", "--newline", "-o", "{out}/%(title)s.%(ext)s"]

  [[tool.param]]
  name = "url"
  type = "string"
  required = true

  [[tool.param]]
  name = "out"
  type = "path"
  default = "~/Downloads"

  [[tool.param]]
  name = "audio_only"
  type = "bool"

  [[tool.param]]
  name = "quality"
  type = "enum"
  choices = ["best", "720", "480"]
`

func newTestService(t *testing.T) *Service {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "tools.toml"), []byte(testToolsTOML), 0o644); err != nil {
		t.Fatal(err)
	}
	s := New(dir)
	t.Cleanup(s.Close)
	return s
}

func TestListTools(t *testing.T) {
	s := newTestService(t)
	tools, err := s.ListTools()
	if err != nil {
		t.Fatalf("ListTools: %v", err)
	}
	if len(tools) != 2 || tools[0].ID != "movies" || tools[1].ID != "dl" {
		t.Fatalf("tools = %+v", tools)
	}
}

func TestRunToolUnknownAndNonApp(t *testing.T) {
	s := newTestService(t)

	if _, err := s.RunTool(context.Background(), "khong-co", nil, ""); err == nil || !strings.Contains(err.Error(), `không có tool "khong-co"`) {
		t.Fatalf("tool lạ: %v", err)
	}
	if _, err := s.RunTool(context.Background(), "movies", nil, ""); err == nil || !strings.Contains(err.Error(), "web chỉ chạy được kind") {
		t.Fatalf("tool url: %v", err)
	}
	if _, err := s.RunTool(context.Background(), "", nil, ""); err == nil || !strings.Contains(err.Error(), "thiếu id tool") {
		t.Fatalf("id rỗng: %v", err)
	}
	_, err := s.RunTool(context.Background(), "dl", nil, "")
	if err == nil || !strings.Contains(err.Error(), "thiếu tham số bắt buộc: url") {
		t.Fatalf("thiếu param url: %v", err)
	}
	if _, err := s.RunTool(context.Background(), "dl", map[string]string{"url": "https://example.com/v"}, ""); err == nil || !strings.Contains(err.Error(), `app "yt-dlp" chưa cài`) {
		t.Fatalf("app chưa cài: %v", err)
	}
}

func TestRunToolRawArgs(t *testing.T) {
	s := newTestService(t)

	// Raw args hợp lệ vẫn đi tới bước chạy app (lỗi app chưa cài chứng tỏ đã qua bước nối tham số).
	if _, err := s.RunTool(context.Background(), "dl", map[string]string{"url": "https://example.com/v"}, `--no-mtime -f best`); err == nil || !strings.Contains(err.Error(), `app "yt-dlp" chưa cài`) {
		t.Fatalf("raw args hợp lệ: %v", err)
	}
	// Nhóm bằng nháy kép vẫn hợp lệ.
	if _, err := s.RunTool(context.Background(), "dl", map[string]string{"url": "https://example.com/v"}, `--match-title "chương trình tối"`); err == nil || !strings.Contains(err.Error(), `app "yt-dlp" chưa cài`) {
		t.Fatalf("raw args có nháy kép: %v", err)
	}
	// Nháy kép thiếu phải báo lỗi trước khi chạy app.
	if _, err := s.RunTool(context.Background(), "dl", map[string]string{"url": "https://example.com/v"}, `--match-title "thiếu đóng`); err == nil || !strings.Contains(err.Error(), `thiếu dấu "`) {
		t.Fatalf("raw args hỏng: %v", err)
	}
}

func TestToolHelpRejects(t *testing.T) {
	s := newTestService(t)
	ctx := context.Background()

	if _, err := s.ToolHelp(ctx, ""); err == nil {
		t.Fatal("thiếu id phải lỗi")
	}
	if _, err := s.ToolHelp(ctx, "khong-co"); err == nil {
		t.Fatal("tool lạ phải lỗi")
	}
	// Tool URL không có --help.
	if _, err := s.ToolHelp(ctx, "movies"); err == nil || !strings.Contains(err.Error(), "không có --help") {
		t.Fatalf("movies: %v", err)
	}
	// Tool app chưa cài: lỗi từ runner khi thiếu file chạy.
	if _, err := s.ToolHelp(ctx, "dl"); err == nil || !strings.Contains(err.Error(), `app "yt-dlp" chưa cài`) {
		t.Fatalf("dl chưa cài: %v", err)
	}
}

func TestResolveParams(t *testing.T) {
	tool := &core.Tool{
		ID: "dl",
		Params: []core.Param{
			{Name: "url", Type: "string", Required: true},
			{Name: "out", Type: "path", Default: "~/Downloads"},
			{Name: "audio_only", Type: "bool", Default: true},
			{Name: "quality", Type: "enum", Choices: []string{"best", "720", "480"}},
			{Name: "count", Type: "int", Default: int64(3)},
		},
	}

	values, err := resolveParams(tool, map[string]string{"url": "https://example.com/v"})
	if err != nil {
		t.Fatalf("resolveParams: %v", err)
	}
	home, err := os.UserHomeDir()
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]string{
		"url":        "https://example.com/v",
		"out":        filepath.Join(home, "Downloads"),
		"audio_only": "true",
		"quality":    "",
		"count":      "3",
	}
	for name, expected := range want {
		if values[name] != expected {
			t.Errorf("values[%q] = %q, want %q", name, values[name], expected)
		}
	}

	if _, err := resolveParams(tool, nil); err == nil || !strings.Contains(err.Error(), "thiếu tham số bắt buộc: url") {
		t.Errorf("thiếu url: %v", err)
	}
	if _, err := resolveParams(tool, map[string]string{"url": "x", "quality": "1080"}); err == nil || !strings.Contains(err.Error(), `"1080" không hợp lệ`) {
		t.Errorf("enum sai: %v", err)
	}
	if _, err := resolveParams(tool, map[string]string{"url": "x", "count": "abc"}); err == nil || !strings.Contains(err.Error(), "phải là số nguyên") {
		t.Errorf("int sai: %v", err)
	}
	if _, err := resolveParams(tool, map[string]string{"url": "x", "audio_only": "có"}); err == nil || !strings.Contains(err.Error(), "phải là true hoặc false") {
		t.Errorf("bool sai: %v", err)
	}
}

func TestParamDefaultBadType(t *testing.T) {
	tool := &core.Tool{
		ID:     "x",
		Params: []core.Param{{Name: "p", Type: "string", Default: 42}},
	}
	if _, err := resolveParams(tool, nil); err == nil || !strings.Contains(err.Error(), "default phải là chuỗi") {
		t.Fatalf("default sai kiểu: %v", err)
	}
}

func TestNewRunID(t *testing.T) {
	first, err := newRunID()
	if err != nil {
		t.Fatal(err)
	}
	second, err := newRunID()
	if err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{first, second} {
		if len(id) != 16 || strings.Trim(id, "0123456789abcdef") != "" {
			t.Errorf("runID %q không phải 16 ký tự hex", id)
		}
	}
	if first == second {
		t.Error("hai runID trùng nhau")
	}
}

func TestSubscribeReplayAndPublish(t *testing.T) {
	s := newTestService(t)
	runID := "run-1"
	s.mu.Lock()
	s.runs[runID] = &run{id: runID, subs: make(map[chan core.Event]struct{})}
	s.mu.Unlock()

	s.publish(runID, core.Event{RunID: runID, Kind: "log", Text: "dòng 1"})
	s.publish(runID, core.Event{RunID: runID, Kind: "log", Text: "dòng 2"})

	ch, unsubscribe, err := s.Subscribe(runID)
	if err != nil {
		t.Fatalf("Subscribe: %v", err)
	}
	if event := <-ch; event.Text != "dòng 1" {
		t.Errorf("event đầu = %+v", event)
	}
	if event := <-ch; event.Text != "dòng 2" {
		t.Errorf("event hai = %+v", event)
	}

	s.publish(runID, core.Event{RunID: runID, Kind: "log", Text: "dòng 3"})
	if event := <-ch; event.Text != "dòng 3" {
		t.Errorf("event ba = %+v", event)
	}

	unsubscribe()
	if _, ok := <-ch; ok {
		t.Error("kênh phải đóng sau unsubscribe")
	}
	s.publish(runID, core.Event{RunID: runID, Kind: "log", Text: "dòng 4"})

	if _, _, err := s.Subscribe("khong-co"); err == nil || !strings.Contains(err.Error(), "không có lần chạy") {
		t.Errorf("subscribe run lạ: %v", err)
	}
}

func TestFinishRunClosesSubscribers(t *testing.T) {
	s := newTestService(t)
	runID := "run-finish"
	s.mu.Lock()
	s.runs[runID] = &run{id: runID, subs: make(map[chan core.Event]struct{})}
	s.mu.Unlock()

	ch, unsubscribe, err := s.Subscribe(runID)
	if err != nil {
		t.Fatalf("Subscribe: %v", err)
	}
	s.publish(runID, core.Event{RunID: runID, Kind: "result", Text: "xong"})
	s.finishRun(runID)

	if event := <-ch; event.Text != "xong" {
		t.Errorf("event cuối = %+v", event)
	}
	if _, ok := <-ch; ok {
		t.Error("kênh phải đóng sau finishRun")
	}
	unsubscribe()

	late, lateUnsubscribe, err := s.Subscribe(runID)
	if err != nil {
		t.Fatalf("Subscribe sau khi xong: %v", err)
	}
	if event := <-late; event.Text != "xong" {
		t.Errorf("replay sau khi xong = %+v", event)
	}
	if _, ok := <-late; ok {
		t.Error("subscribe muộn phải nhận kênh đã đóng")
	}
	lateUnsubscribe()
}

func TestAnswer(t *testing.T) {
	s := newTestService(t)
	pr, pw := newPipe(t)

	runID := "run-wait"
	s.mu.Lock()
	s.runs[runID] = &run{id: runID, subs: make(map[chan core.Event]struct{}), stdin: pw}
	s.mu.Unlock()

	if err := s.Answer(runID, "Minh\r\n"); err != nil {
		t.Fatalf("Answer: %v", err)
	}
	buf := make([]byte, 16)
	n, err := pr.Read(buf)
	if err != nil {
		t.Fatalf("đọc pipe: %v", err)
	}
	if string(buf[:n]) != "Minh\n" {
		t.Errorf("stdin nhận %q, want \"Minh\\n\"", buf[:n])
	}

	s.finishRun(runID)
	if err := s.Answer(runID, "nữa"); err == nil || !strings.Contains(err.Error(), "không chờ câu trả lời") {
		t.Errorf("answer sau khi xong: %v", err)
	}
	if err := s.Answer("khong-co", "x"); err == nil || !strings.Contains(err.Error(), "không có lần chạy") {
		t.Errorf("answer run lạ: %v", err)
	}
	_ = pr.Close()
}

func TestCancel(t *testing.T) {
	s := newTestService(t)
	cancelled := false
	runID := "run-cancel"
	s.mu.Lock()
	s.runs[runID] = &run{
		id:     runID,
		subs:   make(map[chan core.Event]struct{}),
		cancel: func() { cancelled = true },
	}
	s.mu.Unlock()

	if err := s.Cancel(runID); err != nil {
		t.Fatalf("Cancel: %v", err)
	}
	if !cancelled {
		t.Error("cancel không được gọi")
	}

	s.finishRun(runID)
	if err := s.Cancel(runID); err != nil {
		t.Errorf("cancel khi đã xong: %v", err)
	}
	if err := s.Cancel("khong-co"); err == nil || !strings.Contains(err.Error(), "không có lần chạy") {
		t.Errorf("cancel run lạ: %v", err)
	}
}

func TestSweepExpiredRuns(t *testing.T) {
	s := newTestService(t)
	now := time.Now()

	s.mu.Lock()
	s.runs["het-han"] = &run{
		id:       "het-han",
		subs:     make(map[chan core.Event]struct{}),
		done:     true,
		finished: now.Add(-31 * time.Minute),
	}
	s.runs["con-moi"] = &run{
		id:       "con-moi",
		subs:     make(map[chan core.Event]struct{}),
		done:     true,
		finished: now.Add(-time.Minute),
	}
	s.runs["dang-chay"] = &run{id: "dang-chay", subs: make(map[chan core.Event]struct{})}
	s.mu.Unlock()

	subCh, _, err := s.Subscribe("het-han")
	if err != nil {
		t.Fatalf("Subscribe: %v", err)
	}

	s.sweep(now)

	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.runs["het-han"]; ok {
		t.Error("run quá hạn phải bị xóa")
	}
	if _, ok := s.runs["con-moi"]; !ok {
		t.Error("run chưa quá hạn phải được giữ")
	}
	if _, ok := s.runs["dang-chay"]; !ok {
		t.Error("run đang chạy phải được giữ")
	}
	if _, ok := <-subCh; ok {
		t.Error("kênh của run bị xóa phải đóng")
	}
}

func TestCheckTrusted(t *testing.T) {
	s := newTestService(t)
	if err := s.checkTrusted(context.Background(), &core.App{AppSpec: core.AppSpec{ID: "a"}}); err != nil {
		t.Errorf("app không có source phải qua: %v", err)
	}
	err := s.checkTrusted(context.Background(), &core.App{AppSpec: core.AppSpec{ID: "a", Source: "https://example.com/repo.git"}})
	if err == nil || !strings.Contains(err.Error(), `app "a" chưa cài`) {
		t.Errorf("app git chưa cài: %v", err)
	}
}

func TestRunForegroundAskFlow(t *testing.T) {
	s := newTestService(t)
	runID := "run-ask"
	s.mu.Lock()
	s.runs[runID] = &run{id: runID, subs: make(map[chan core.Event]struct{})}
	s.mu.Unlock()

	ch, _, err := s.Subscribe(runID)
	if err != nil {
		t.Fatalf("Subscribe: %v", err)
	}

	process := serviceHelperCommand()
	app := &core.App{AppSpec: core.AppSpec{ID: "cli-tool", Runtime: "python"}}
	go s.runForeground(runID, "cli-tool", app, process, []string{"python", "cli_tool.py"})

	deadline := time.Now().Add(10 * time.Second)
	answered := false
	var kinds []string
	for {
		select {
		case event, ok := <-ch:
			if !ok {
				t.Fatal("kênh đóng sớm")
			}
			kinds = append(kinds, event.Kind)
			switch event.Kind {
			case "ask":
				if event.Text != "Nhập tên: " {
					t.Errorf("ask text = %q", event.Text)
				}
				if !answered {
					answered = true
					if err := s.Answer(runID, "Minh"); err != nil {
						t.Fatalf("Answer: %v", err)
					}
				}
			case "result":
				if event.Data["ok"] != true {
					t.Errorf("result = %+v", event)
				}
				return
			}
		case <-time.After(time.Until(deadline)):
			t.Fatalf("hết giờ chờ result: kinds=%v", kinds)
		}
	}
}

func TestRunForegroundExitError(t *testing.T) {
	s := newTestService(t)
	runID := "run-fail"
	s.mu.Lock()
	s.runs[runID] = &run{id: runID, subs: make(map[chan core.Event]struct{})}
	s.mu.Unlock()

	ch, _, err := s.Subscribe(runID)
	if err != nil {
		t.Fatalf("Subscribe: %v", err)
	}

	process := exec.Command(os.Args[0], "-test.run=^TestServiceHelper$")
	process.Env = append(os.Environ(), "GO_WANT_SERVICE_HELPER=7")
	app := &core.App{AppSpec: core.AppSpec{ID: "cli-tool", Runtime: "python"}}
	go s.runForeground(runID, "cli-tool", app, process, nil)

	deadline := time.Now().Add(10 * time.Second)
	for {
		select {
		case event, ok := <-ch:
			if !ok {
				t.Fatal("kênh đóng sớm")
			}
			if event.Kind == "result" {
				if event.Data["ok"] != false || !strings.Contains(event.Text, "mã 7") {
					t.Errorf("result = %+v", event)
				}
				return
			}
		case <-time.After(time.Until(deadline)):
			t.Fatal("hết giờ chờ result")
		}
	}
}

func TestRunDetachedSavesState(t *testing.T) {
	s := newTestService(t)
	runID := "run-gui"
	s.mu.Lock()
	s.runs[runID] = &run{id: runID, subs: make(map[chan core.Event]struct{})}
	s.mu.Unlock()

	ch, _, err := s.Subscribe(runID)
	if err != nil {
		t.Fatalf("Subscribe: %v", err)
	}

	process := exec.Command(os.Args[0], "-test.run=^TestServiceHelper$")
	process.Env = append(os.Environ(), "GO_WANT_SERVICE_HELPER=gui")
	app := &core.App{AppSpec: core.AppSpec{ID: "gui-tool", Runtime: "python", GUI: true}}
	go s.runDetached(runID, "gui-tool", app, process, nil)

	deadline := time.Now().Add(10 * time.Second)
	for {
		select {
		case event, ok := <-ch:
			if !ok {
				t.Fatal("kênh đóng sớm")
			}
			if event.Kind == "result" {
				if event.Data["ok"] != true || !strings.Contains(event.Text, "PID") {
					t.Errorf("result = %+v", event)
				}
				break
			}
		case <-time.After(time.Until(deadline)):
			t.Fatal("hết giờ chờ result")
		}
		break
	}

	state, err := s.mgr.LoadRunState("gui-tool")
	if err != nil {
		t.Fatalf("LoadRunState: %v", err)
	}
	if state.PID <= 0 || state.Cmd == "" {
		t.Errorf("state = %+v", state)
	}
	if appmgr.IsProcessAlive(state.PID, state.StartTime) {
		if err := appmgr.StopProcess(*state); err != nil {
			t.Errorf("StopProcess: %v", err)
		}
	}
	_ = s.mgr.RemoveRunState("gui-tool")
}

func serviceHelperCommand() *exec.Cmd {
	cmd := exec.Command(os.Args[0], "-test.run=^TestServiceHelper$")
	cmd.Env = append(os.Environ(), "GO_WANT_SERVICE_HELPER=ask")
	return cmd
}

func TestNpxToolAsksBeforeRun(t *testing.T) {
	// npx giả trên PATH: chỉ ghi marker khi chạy thật (không phải --help sau cài),
	// không đụng tới mạng.
	fakeDir := t.TempDir()
	name := "npx"
	script := "#!/bin/sh\nprintf 'fake npx %s\\n' \"$*\"\ncase \"$*\" in *--help*) exit 0;; esac\necho ran > npx-ran.txt\n"
	if runtime.GOOS == "windows" {
		name = "npx.cmd"
		script = "@echo off\r\necho fake npx %*\r\necho.%* | findstr /c:\"--help\" >nul && exit /b 0\r\necho ran> npx-ran.txt\r\nexit /b 0\r\n"
	}
	if err := os.WriteFile(filepath.Join(fakeDir, name), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", fakeDir+string(os.PathListSeparator)+os.Getenv("PATH"))

	s := newTestService(t)

	// Mock tool node chạy bằng npx; cài từ thư mục local.
	mockDir := t.TempDir()
	appTOML := "schema_version = 1\nid = 'npx-tool'\nruntime = 'node'\nentry = ['npx', 'some-package']\n"
	if err := os.WriteFile(filepath.Join(mockDir, "app.toml"), []byte(appTOML), 0o644); err != nil {
		t.Fatal(err)
	}

	installRun, err := s.InstallTool(context.Background(), mockDir)
	if err != nil {
		t.Fatalf("InstallTool: %v", err)
	}
	installCh, _, err := s.Subscribe(installRun)
	if err != nil {
		t.Fatalf("Subscribe install: %v", err)
	}
	waitEvent := func(t *testing.T, ch <-chan core.Event, kind string) core.Event {
		t.Helper()
		deadline := time.Now().Add(15 * time.Second)
		for {
			select {
			case event, ok := <-ch:
				if !ok {
					t.Fatalf("kênh đóng trước khi nhận %q", kind)
				}
				if event.Kind == kind {
					return event
				}
			case <-time.After(time.Until(deadline)):
				t.Fatalf("hết giờ chờ event %q", kind)
			}
		}
	}
	installResult := waitEvent(t, installCh, "result")
	if installResult.Data["ok"] != true {
		t.Fatalf("cài npx-tool thất bại: %+v", installResult)
	}

	app, err := s.mgr.LoadApp("npx-tool")
	if err != nil {
		t.Fatalf("LoadApp: %v", err)
	}
	if app.TrustedNpx {
		t.Fatal("state.json sau khi cài phải có trusted_npx = false")
	}
	if app.CacheDir != filepath.Join(s.dir, "cache") {
		t.Fatalf("CacheDir = %q, muốn %q", app.CacheDir, filepath.Join(s.dir, "cache"))
	}

	// Lần chạy 1: có cảnh báo ask; trả lời "n" thì hủy, không chạy npx.
	run1, err := s.RunTool(context.Background(), "npx-tool", nil, "")
	if err != nil {
		t.Fatalf("RunTool lần 1: %v", err)
	}
	ch1, _, err := s.Subscribe(run1)
	if err != nil {
		t.Fatalf("Subscribe run1: %v", err)
	}
	ask1 := waitEvent(t, ch1, "ask")
	if ask1.Text != runner.NpxWarning {
		t.Fatalf("ask text = %q", ask1.Text)
	}
	if err := s.Answer(run1, "n"); err != nil {
		t.Fatalf("Answer n: %v", err)
	}
	if result := waitEvent(t, ch1, "result"); result.Data["ok"] != false || result.Text != "Đã hủy." {
		t.Fatalf("result sau n = %+v", result)
	}
	if _, err := os.Stat(filepath.Join(app.Dir, "npx-ran.txt")); !os.IsNotExist(err) {
		t.Fatalf("npx không được chạy khi từ chối: %v", err)
	}

	// Lần chạy 2: vẫn hỏi; trả lời "y" thì chạy và ghi nhớ.
	run2, err := s.RunTool(context.Background(), "npx-tool", nil, "")
	if err != nil {
		t.Fatalf("RunTool lần 2: %v", err)
	}
	ch2, _, err := s.Subscribe(run2)
	if err != nil {
		t.Fatalf("Subscribe run2: %v", err)
	}
	if ask := waitEvent(t, ch2, "ask"); ask.Text != runner.NpxWarning {
		t.Fatalf("ask lần 2 = %q", ask.Text)
	}
	if err := s.Answer(run2, "y"); err != nil {
		t.Fatalf("Answer y: %v", err)
	}
	if result := waitEvent(t, ch2, "result"); result.Data["ok"] != true {
		t.Fatalf("result sau y = %+v", result)
	}
	if _, err := os.Stat(filepath.Join(app.Dir, "npx-ran.txt")); err != nil {
		t.Fatalf("npx phải chạy sau khi đồng ý: %v", err)
	}
	app, err = s.mgr.LoadApp("npx-tool")
	if err != nil {
		t.Fatalf("LoadApp sau y: %v", err)
	}
	if !app.TrustedNpx {
		t.Fatal("state.json sau khi đồng ý phải có trusted_npx = true")
	}

	// Lần chạy 3: không hỏi lại, chạy thẳng.
	run3, err := s.RunTool(context.Background(), "npx-tool", nil, "")
	if err != nil {
		t.Fatalf("RunTool lần 3: %v", err)
	}
	ch3, _, err := s.Subscribe(run3)
	if err != nil {
		t.Fatalf("Subscribe run3: %v", err)
	}
	deadline := time.Now().Add(15 * time.Second)
	for {
		select {
		case event, ok := <-ch3:
			if !ok {
				t.Fatal("kênh đóng trước khi có result")
			}
			if event.Kind == "ask" {
				t.Fatal("lần chạy sau khi tin cậy không được hỏi lại")
			}
			if event.Kind == "result" {
				if event.Data["ok"] != true {
					t.Fatalf("result lần 3 = %+v", event)
				}
				return
			}
		case <-time.After(time.Until(deadline)):
			t.Fatal("hết giờ chờ result lần 3")
		}
	}
}

func TestServiceHelper(t *testing.T) {
	switch os.Getenv("GO_WANT_SERVICE_HELPER") {
	case "":
		return
	case "7":
		fmt.Fprintln(os.Stderr, "lỗi giả lập")
		os.Exit(7)
	case "gui":
		time.Sleep(1500 * time.Millisecond)
		os.Exit(0)
	}
	fmt.Print("Nhập tên: ")
	time.Sleep(300 * time.Millisecond)
	var name string
	fmt.Fscanln(os.Stdin, &name)
	fmt.Println("Xin chào, " + name + "!")
	os.Exit(0)
}

func newPipe(t *testing.T) (read, write *os.File) {
	t.Helper()
	read, write, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	return read, write
}
