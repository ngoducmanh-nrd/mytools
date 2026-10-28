package web

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"mytools/internal/core"
	"mytools/internal/runner"
)

type fakeRun struct {
	history []core.Event
	subs    map[chan core.Event]struct{}
	mu      sync.Mutex
}

type fakeRunner struct {
	mu             sync.Mutex
	tools          []core.Tool
	runs           map[string]*fakeRun
	answers        map[string][]string
	toolErr        error
	runCalls       int
	lastRawArgs    string
	installCalls   int
	installSources []string
	manualCalls    []manualCall
	customCalls    []customCall
	installErr     error
	removedIDs     []string
	agentErr       error
	agentReply     string
	agentCfg       core.AgentConfig
	agentMsgs      []core.AgentMessage
	outDirID       string
	outDirErr      error
	outDirReply    string
}

type manualCall struct {
	source, name, entry string
	gui                 bool
}

type customCall struct {
	id, name, kind, target, entry string
	tags                          []string
	venv                          bool
}

func newFakeRunner() *fakeRunner {
	return &fakeRunner{
		tools: []core.Tool{
			{
				ID:   "dl",
				Name: "Tải video",
				Kind: "app",
				App:  "yt-dlp",
				Tags: []string{"media"},
				Params: []core.Param{
					{Name: "url", Type: "string", Required: true},
					{Name: "out", Type: "path", Default: "~/Downloads"},
					{Name: "audio_only", Type: "bool", Default: true},
					{Name: "quality", Type: "enum", Choices: []string{"best", "720"}},
				},
			},
			{ID: "movies", Name: "Movie Tracker", Kind: "url", Target: "https://movie-tracker-v2-jade.vercel.app/"},
		},
		runs:    make(map[string]*fakeRun),
		answers: make(map[string][]string),
	}
}

func (f *fakeRunner) ListTools() ([]core.Tool, error) {
	if f.toolErr != nil {
		return nil, f.toolErr
	}
	return f.tools, nil
}

func (f *fakeRunner) RunTool(ctx context.Context, id string, params map[string]string, rawArgs string) (string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.runCalls++
	f.lastRawArgs = rawArgs
	for _, tool := range f.tools {
		if tool.ID == id {
			runID := fmt.Sprintf("run-%d", f.runCalls)
			f.runs[runID] = &fakeRun{subs: make(map[chan core.Event]struct{})}
			return runID, nil
		}
	}
	return "", fmt.Errorf("không có tool %q", id)
}

func (f *fakeRunner) ToolHelp(ctx context.Context, id string) (runner.HelpInfo, error) {
	for _, tool := range f.tools {
		if tool.ID == id {
			if tool.Kind != "app" {
				return runner.HelpInfo{}, fmt.Errorf("tool %q có kind %q: không có --help", id, tool.Kind)
			}
			return runner.HelpInfo{
				Summary: "Cờ hỗ trợ (từ --help): --force --output",
				Output:  "SỬ DỤNG: yt-dlp [tùy chọn]\n  --force    Ép tải lại\n",
				Path:    `C:\apps\yt-dlp\help.txt`,
			}, nil
		}
	}
	return runner.HelpInfo{}, fmt.Errorf("không có tool %q", id)
}

func (f *fakeRunner) Subscribe(runID string) (<-chan core.Event, func(), error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	run := f.runs[runID]
	if run == nil {
		return nil, nil, fmt.Errorf("không có lần chạy %q", runID)
	}

	run.mu.Lock()
	defer run.mu.Unlock()
	ch := make(chan core.Event, len(run.history)+8)
	for _, event := range run.history {
		ch <- event
	}
	run.subs[ch] = struct{}{}

	unsubscribe := func() {
		run.mu.Lock()
		defer run.mu.Unlock()
		if _, ok := run.subs[ch]; !ok {
			return
		}
		delete(run.subs, ch)
		close(ch)
	}
	return ch, unsubscribe, nil
}

func (f *fakeRunner) Answer(runID, answer string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if _, ok := f.runs[runID]; !ok {
		return fmt.Errorf("không có lần chạy %q", runID)
	}
	f.answers[runID] = append(f.answers[runID], answer)
	return nil
}

func (f *fakeRunner) InstallTool(ctx context.Context, source string) (string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.installErr != nil {
		return "", f.installErr
	}
	f.installCalls++
	f.installSources = append(f.installSources, source)
	runID := fmt.Sprintf("install-%d", f.installCalls)
	f.runs[runID] = &fakeRun{subs: make(map[chan core.Event]struct{})}
	return runID, nil
}

func (f *fakeRunner) InstallManualTool(ctx context.Context, source, name, entry string, gui bool) (string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.installErr != nil {
		return "", f.installErr
	}
	f.installCalls++
	f.manualCalls = append(f.manualCalls, manualCall{source: source, name: name, entry: entry, gui: gui})
	runID := fmt.Sprintf("install-%d", f.installCalls)
	f.runs[runID] = &fakeRun{subs: make(map[chan core.Event]struct{})}
	return runID, nil
}

func (f *fakeRunner) InstallCustomTool(ctx context.Context, id, name, kind, target, entry string, tags []string, venv bool) (string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.installErr != nil {
		return "", f.installErr
	}
	f.installCalls++
	f.customCalls = append(f.customCalls, customCall{id: id, name: name, kind: kind, target: target, entry: entry, tags: tags, venv: venv})
	if kind == "url" {
		return "", nil
	}
	runID := fmt.Sprintf("install-%d", f.installCalls)
	f.runs[runID] = &fakeRun{subs: make(map[chan core.Event]struct{})}
	return runID, nil
}

func (f *fakeRunner) RemoveTool(id string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.removedIDs = append(f.removedIDs, id)
	return nil
}

func (f *fakeRunner) AgentChat(ctx context.Context, cfg core.AgentConfig, messages []core.AgentMessage) (string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.agentErr != nil {
		return "", f.agentErr
	}
	f.agentCfg = cfg
	f.agentMsgs = append([]core.AgentMessage(nil), messages...)
	return f.agentReply, nil
}

func (f *fakeRunner) ToolOutDir(id string) (string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.outDirID = id
	if f.outDirErr != nil {
		return "", f.outDirErr
	}
	return f.outDirReply, nil
}

func (f *fakeRunner) publish(runID string, event core.Event) {
	f.mu.Lock()
	run := f.runs[runID]
	f.mu.Unlock()
	if run == nil {
		return
	}
	run.mu.Lock()
	defer run.mu.Unlock()
	run.history = append(run.history, event)
	for ch := range run.subs {
		select {
		case ch <- event:
		default:
		}
	}
}

func (f *fakeRunner) subCount(runID string) int {
	f.mu.Lock()
	run := f.runs[runID]
	f.mu.Unlock()
	if run == nil {
		return 0
	}
	run.mu.Lock()
	defer run.mu.Unlock()
	return len(run.subs)
}

func newTestServer(t *testing.T, runner ToolRunner) (*Server, *httptest.Server) {
	t.Helper()
	s, err := New(0, runner)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	ts := httptest.NewUnstartedServer(s.Handler())
	ts.Listener = s.Listener()
	ts.Start()
	t.Cleanup(ts.Close)
	return s, ts
}

func TestIndexContainsToken(t *testing.T) {
	s, ts := newTestServer(t, newFakeRunner())

	resp, err := http.Get(ts.URL + "/")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d", resp.StatusCode)
	}
	if !strings.Contains(string(body), s.Token()) {
		t.Error("HTML thiếu token")
	}
	if strings.Contains(string(body), "__MT_TOKEN__") {
		t.Error("placeholder token chưa được thay")
	}
	if !strings.Contains(string(body), "DevTools Workspace") {
		t.Error("HTML thiếu nội dung giao diện")
	}
}

func TestSecurityChecks(t *testing.T) {
	s, ts := newTestServer(t, newFakeRunner())

	get := func(host, origin string) *http.Response {
		t.Helper()
		req, err := http.NewRequest(http.MethodGet, ts.URL+"/", nil)
		if err != nil {
			t.Fatal(err)
		}
		if host != "" {
			req.Host = host
		}
		if origin != "" {
			req.Header.Set("Origin", origin)
		}
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		return resp
	}

	post := func(host, origin, token string) *http.Response {
		t.Helper()
		body := strings.NewReader(`{"id":"dl","params":{"url":"https://example.com"}}`)
		req, err := http.NewRequest(http.MethodPost, ts.URL+"/api/run", body)
		if err != nil {
			t.Fatal(err)
		}
		if host != "" {
			req.Host = host
		}
		if origin != "" {
			req.Header.Set("Origin", origin)
		}
		if token != "" {
			req.Header.Set("X-MT-Token", token)
		}
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		return resp
	}

	if resp := get("", ""); resp.StatusCode != http.StatusOK {
		resp.Body.Close()
		t.Errorf("GET host chuẩn: %d", resp.StatusCode)
	} else {
		resp.Body.Close()
	}
	if resp := get("127.0.0.1:1", ""); resp.StatusCode != http.StatusForbidden {
		resp.Body.Close()
		t.Errorf("GET sai port: %d", resp.StatusCode)
	} else {
		resp.Body.Close()
	}
	if resp := get("example.com:7878", ""); resp.StatusCode != http.StatusForbidden {
		resp.Body.Close()
		t.Errorf("GET sai hostname: %d", resp.StatusCode)
	} else {
		resp.Body.Close()
	}
	if resp := get("", "http://evil.example"); resp.StatusCode != http.StatusForbidden {
		resp.Body.Close()
		t.Errorf("GET origin lạ: %d", resp.StatusCode)
	} else {
		resp.Body.Close()
	}
	if resp := get("", "http://"+ts.Listener.Addr().String()); resp.StatusCode != http.StatusOK {
		resp.Body.Close()
		t.Errorf("GET origin khớp: %d", resp.StatusCode)
	} else {
		resp.Body.Close()
	}

	if resp := post("", "", ""); resp.StatusCode != http.StatusForbidden {
		resp.Body.Close()
		t.Errorf("POST thiếu token: %d", resp.StatusCode)
	} else {
		resp.Body.Close()
	}
	if resp := post("", "", "sai-token"); resp.StatusCode != http.StatusForbidden {
		resp.Body.Close()
		t.Errorf("POST sai token: %d", resp.StatusCode)
	} else {
		resp.Body.Close()
	}
	if resp := post("", "http://evil.example", s.Token()); resp.StatusCode != http.StatusForbidden {
		resp.Body.Close()
		t.Errorf("POST origin lạ: %d", resp.StatusCode)
	} else {
		resp.Body.Close()
	}
	if resp := post("", "http://"+ts.Listener.Addr().String(), s.Token()); resp.StatusCode != http.StatusOK {
		resp.Body.Close()
		t.Errorf("POST hợp lệ: %d", resp.StatusCode)
	} else {
		resp.Body.Close()
	}
}

func TestAPITools(t *testing.T) {
	_, ts := newTestServer(t, newFakeRunner())

	resp, err := http.Get(ts.URL + "/api/tools")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	var tools []toolJSON
	if err := json.NewDecoder(resp.Body).Decode(&tools); err != nil {
		t.Fatal(err)
	}
	if len(tools) != 2 {
		t.Fatalf("tools = %d, want 2", len(tools))
	}
	if tools[0].ID != "dl" || tools[0].Name != "Tải video" || len(tools[0].Params) != 4 {
		t.Errorf("tool[0] = %+v", tools[0])
	}
	if tools[0].Params[1].Default != "~/Downloads" {
		t.Errorf("default out = %v", tools[0].Params[1].Default)
	}
	if tools[1].Kind != "url" || tools[1].Target == "" {
		t.Errorf("tool[1] = %+v", tools[1])
	}
}

func TestRunEndpoint(t *testing.T) {
	s, ts := newTestServer(t, newFakeRunner())

	post := func(body string) (*http.Response, map[string]any) {
		t.Helper()
		req, err := http.NewRequest(http.MethodPost, ts.URL+"/api/run", strings.NewReader(body))
		if err != nil {
			t.Fatal(err)
		}
		req.Header.Set("X-MT-Token", s.Token())
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		var parsed map[string]any
		_ = json.NewDecoder(resp.Body).Decode(&parsed)
		return resp, parsed
	}

	resp, parsed := post(`{"id":"dl","params":{"url":"https://example.com/v"}}`)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("run: %d %v", resp.StatusCode, parsed)
	}
	if parsed["run_id"] == nil || parsed["run_id"].(string) == "" {
		t.Errorf("thiếu run_id: %v", parsed)
	}

	resp, parsed = post(`{"id":"dl","params":{"url":"https://example.com/v"},"raw_args":"--no-mtime -f best"}`)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("run raw_args: %d %v", resp.StatusCode, parsed)
	}
	fake := s.runner.(*fakeRunner)
	fake.mu.Lock()
	rawArgs := fake.lastRawArgs
	fake.mu.Unlock()
	if rawArgs != "--no-mtime -f best" {
		t.Errorf("lastRawArgs = %q", rawArgs)
	}

	resp, parsed = post(`{"id":"khong-co"}`)
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("tool lạ: %d", resp.StatusCode)
	}
	if parsed["error"] == nil {
		t.Errorf("thiếu error: %v", parsed)
	}

	resp, parsed = post(`không phải JSON`)
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("JSON hỏng: %d", resp.StatusCode)
	}
}

func TestToolHelpEndpoint(t *testing.T) {
	_, ts := newTestServer(t, newFakeRunner())

	get := func(path string) (*http.Response, map[string]any) {
		t.Helper()
		resp, err := http.Get(ts.URL + path)
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		var parsed map[string]any
		_ = json.NewDecoder(resp.Body).Decode(&parsed)
		return resp, parsed
	}

	resp, parsed := get("/api/tools/dl/help")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("help: %d %v", resp.StatusCode, parsed)
	}
	if parsed["summary"] == nil || !strings.Contains(parsed["summary"].(string), "--force") {
		t.Errorf("summary = %v", parsed["summary"])
	}
	if parsed["output"] == nil || !strings.Contains(parsed["output"].(string), "SỬ DỤNG") {
		t.Errorf("output = %v", parsed["output"])
	}
	if parsed["path"] == nil || parsed["path"] == "" {
		t.Errorf("path = %v", parsed["path"])
	}

	if resp, parsed := get("/api/tools/movies/help"); resp.StatusCode != http.StatusBadRequest {
		t.Errorf("help url tool: %d %v", resp.StatusCode, parsed)
	}
	if resp, parsed := get("/api/tools/khong-co/help"); resp.StatusCode != http.StatusBadRequest {
		t.Errorf("help tool lạ: %d %v", resp.StatusCode, parsed)
	}
}

func TestAnswerEndpoint(t *testing.T) {
	s, ts := newTestServer(t, newFakeRunner())
	fake := s.runner.(*fakeRunner)
	fake.mu.Lock()
	fake.runs["run-1"] = &fakeRun{subs: make(map[chan core.Event]struct{})}
	fake.mu.Unlock()

	req, err := http.NewRequest(http.MethodPost, ts.URL+"/api/answer",
		strings.NewReader(`{"run_id":"run-1","answer":"Minh"}`))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("X-MT-Token", s.Token())
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("answer: %d", resp.StatusCode)
	}

	fake.mu.Lock()
	answers := append([]string(nil), fake.answers["run-1"]...)
	fake.mu.Unlock()
	if len(answers) != 1 || answers[0] != "Minh" {
		t.Errorf("answers = %v", answers)
	}

	req, err = http.NewRequest(http.MethodPost, ts.URL+"/api/answer",
		strings.NewReader(`{"run_id":"khong-co","answer":"x"}`))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("X-MT-Token", s.Token())
	resp, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("answer run lạ: %d", resp.StatusCode)
	}
}

func TestEventsSSEReplayAndLive(t *testing.T) {
	s, ts := newTestServer(t, newFakeRunner())
	fake := s.runner.(*fakeRunner)

	fake.mu.Lock()
	fake.runs["run-1"] = &fakeRun{
		history: []core.Event{
			{RunID: "run-1", Kind: "log", Text: "dòng cũ 1"},
			{RunID: "run-1", Kind: "log", Text: "dòng cũ 2"},
		},
		subs: make(map[chan core.Event]struct{}),
	}
	fake.mu.Unlock()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, ts.URL+"/api/events?runID=run-1", nil)
	if err != nil {
		t.Fatal(err)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if got := resp.Header.Get("Content-Type"); !strings.HasPrefix(got, "text/event-stream") {
		t.Fatalf("Content-Type = %q", got)
	}

	lines := make(chan string, 32)
	go func() {
		reader := bufio.NewReader(resp.Body)
		for {
			line, err := reader.ReadString('\n')
			if err != nil {
				close(lines)
				return
			}
			lines <- strings.TrimRight(line, "\n")
		}
	}()

	expect := func(want string) {
		t.Helper()
		deadline := time.After(5 * time.Second)
		for {
			select {
			case line, ok := <-lines:
				if !ok {
					t.Fatalf("hết stream khi chờ %q", want)
				}
				if strings.Contains(line, want) {
					return
				}
			case <-deadline:
				t.Fatalf("hết giờ chờ %q", want)
			}
		}
	}

	expect(`"kind":"log"`)
	expect(`"text":"dòng cũ 2"`)

	fake.publish("run-1", core.Event{RunID: "run-1", Kind: "progress", Text: "[download] 50%", Pct: 50})
	expect("[download] 50%")

	if fake.subCount("run-1") != 1 {
		t.Errorf("subCount = %d, want 1", fake.subCount("run-1"))
	}

	// Client ngắt kết nối: handler phải hủy đăng ký, không ảnh hưởng lần chạy.
	cancel()
	deadline := time.Now().Add(5 * time.Second)
	for fake.subCount("run-1") > 0 && time.Now().Before(deadline) {
		time.Sleep(20 * time.Millisecond)
	}
	if fake.subCount("run-1") != 0 {
		t.Errorf("subCount sau khi ngắt = %d, want 0", fake.subCount("run-1"))
	}
}

func TestEventsUnknownRun(t *testing.T) {
	_, ts := newTestServer(t, newFakeRunner())

	resp, err := http.Get(ts.URL + "/api/events?runID=khong-co")
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("status = %d, want 404", resp.StatusCode)
	}
}

func TestAppAddEndpoint(t *testing.T) {
	s, ts := newTestServer(t, newFakeRunner())
	fake := s.runner.(*fakeRunner)

	post := func(body string, withToken bool) (*http.Response, map[string]any) {
		t.Helper()
		req, err := http.NewRequest(http.MethodPost, ts.URL+"/api/app/add", strings.NewReader(body))
		if err != nil {
			t.Fatal(err)
		}
		if withToken {
			req.Header.Set("X-MT-Token", s.Token())
		}
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		var parsed map[string]any
		_ = json.NewDecoder(resp.Body).Decode(&parsed)
		return resp, parsed
	}

	resp, parsed := post(`{"source":"./testdata/mock-tools/cli-tool"}`, true)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("add: %d %v", resp.StatusCode, parsed)
	}
	if parsed["run_id"] != "install-1" {
		t.Errorf("run_id = %v, want install-1", parsed["run_id"])
	}
	fake.mu.Lock()
	sources := append([]string(nil), fake.installSources...)
	fake.mu.Unlock()
	if len(sources) != 1 || sources[0] != "./testdata/mock-tools/cli-tool" {
		t.Errorf("installSources = %v", sources)
	}

	if resp, _ := post(`{"source":""}`, false); resp.StatusCode != http.StatusForbidden {
		t.Errorf("add thiếu token: %d", resp.StatusCode)
	}
	if resp, _ := post(`không phải JSON`, true); resp.StatusCode != http.StatusBadRequest {
		t.Errorf("add JSON hỏng: %d", resp.StatusCode)
	}
}

func TestAppAddEndpointInstallError(t *testing.T) {
	fake := newFakeRunner()
	fake.installErr = fmt.Errorf("app %q đã tồn tại", "cli-tool")
	s, ts := newTestServer(t, fake)

	req, err := http.NewRequest(http.MethodPost, ts.URL+"/api/app/add",
		strings.NewReader(`{"source":"./testdata/mock-tools/cli-tool"}`))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("X-MT-Token", s.Token())
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", resp.StatusCode)
	}
	var parsed map[string]any
	_ = json.NewDecoder(resp.Body).Decode(&parsed)
	if parsed["error"] == nil || !strings.Contains(parsed["error"].(string), "đã tồn tại") {
		t.Errorf("error = %v", parsed["error"])
	}
}

func TestAppInstallManualEndpoint(t *testing.T) {
	s, ts := newTestServer(t, newFakeRunner())
	fake := s.runner.(*fakeRunner)

	post := func(body string, withToken bool) (*http.Response, map[string]any) {
		t.Helper()
		req, err := http.NewRequest(http.MethodPost, ts.URL+"/api/app/install-manual", strings.NewReader(body))
		if err != nil {
			t.Fatal(err)
		}
		if withToken {
			req.Header.Set("X-MT-Token", s.Token())
		}
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		var parsed map[string]any
		_ = json.NewDecoder(resp.Body).Decode(&parsed)
		return resp, parsed
	}

	resp, parsed := post(`{"source":"https://github.com/x/y.git","name":"Rea","entry":"python main.py","gui":true}`, true)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("install-manual: %d %v", resp.StatusCode, parsed)
	}
	if parsed["run_id"] != "install-1" {
		t.Errorf("run_id = %v, want install-1", parsed["run_id"])
	}
	fake.mu.Lock()
	calls := append([]manualCall(nil), fake.manualCalls...)
	fake.mu.Unlock()
	if len(calls) != 1 || calls[0].source != "https://github.com/x/y.git" || calls[0].name != "Rea" ||
		calls[0].entry != "python main.py" || !calls[0].gui {
		t.Errorf("manualCalls = %+v", calls)
	}

	if resp, _ := post(`{"source":"x","name":"Rea","entry":"python main.py"}`, false); resp.StatusCode != http.StatusForbidden {
		t.Errorf("install-manual thiếu token: %d", resp.StatusCode)
	}
	if resp, _ := post(`không phải JSON`, true); resp.StatusCode != http.StatusBadRequest {
		t.Errorf("install-manual JSON hỏng: %d", resp.StatusCode)
	}
}

func TestAppInstallManualEndpointError(t *testing.T) {
	fake := newFakeRunner()
	fake.installErr = fmt.Errorf("lệnh chạy phải bắt đầu bằng \"python\"")
	s, ts := newTestServer(t, fake)

	req, err := http.NewRequest(http.MethodPost, ts.URL+"/api/app/install-manual",
		strings.NewReader(`{"source":"https://github.com/x/y.git","name":"Rea","entry":"main.py"}`))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("X-MT-Token", s.Token())
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", resp.StatusCode)
	}
	var parsed map[string]any
	_ = json.NewDecoder(resp.Body).Decode(&parsed)
	if parsed["error"] == nil || !strings.Contains(parsed["error"].(string), "python") {
		t.Errorf("error = %v", parsed["error"])
	}
}

func TestAppRemoveEndpoint(t *testing.T) {
	s, ts := newTestServer(t, newFakeRunner())
	fake := s.runner.(*fakeRunner)

	del := func(body string, token string) (*http.Response, map[string]any) {
		t.Helper()
		req, err := http.NewRequest(http.MethodDelete, ts.URL+"/api/app/remove", strings.NewReader(body))
		if err != nil {
			t.Fatal(err)
		}
		if token != "" {
			req.Header.Set("X-MT-Token", token)
		}
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		var parsed map[string]any
		_ = json.NewDecoder(resp.Body).Decode(&parsed)
		return resp, parsed
	}

	resp, parsed := del(`{"id":"movies"}`, s.Token())
	if resp.StatusCode != http.StatusOK || parsed["ok"] != true {
		t.Fatalf("remove: %d %v", resp.StatusCode, parsed)
	}
	fake.mu.Lock()
	removed := append([]string(nil), fake.removedIDs...)
	fake.mu.Unlock()
	if len(removed) != 1 || removed[0] != "movies" {
		t.Errorf("removedIDs = %v", removed)
	}

	if resp, _ := del(`{"id":"movies"}`, ""); resp.StatusCode != http.StatusForbidden {
		t.Errorf("remove thiếu token: %d", resp.StatusCode)
	}
	if resp, _ := del(`{"id":"movies"}`, "sai-token"); resp.StatusCode != http.StatusForbidden {
		t.Errorf("remove sai token: %d", resp.StatusCode)
	}
	if resp, _ := del(`không phải JSON`, s.Token()); resp.StatusCode != http.StatusBadRequest {
		t.Errorf("remove JSON hỏng: %d", resp.StatusCode)
	}
}

func TestAgentChatEndpoint(t *testing.T) {
	s, ts := newTestServer(t, newFakeRunner())
	fake := s.runner.(*fakeRunner)
	fake.agentReply = "Dùng mt run yt-dlp <url>."

	post := func(body string, token string) (*http.Response, map[string]any) {
		t.Helper()
		req, err := http.NewRequest(http.MethodPost, ts.URL+"/api/agent/chat", strings.NewReader(body))
		if err != nil {
			t.Fatal(err)
		}
		if token != "" {
			req.Header.Set("X-MT-Token", token)
		}
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		var parsed map[string]any
		_ = json.NewDecoder(resp.Body).Decode(&parsed)
		return resp, parsed
	}

	body := `{"base_url":"https://api.groq.com/openai/v1","api_key":"gsk-abc","model":"m","messages":[{"role":"user","content":"cách tải video?"}]}`
	resp, parsed := post(body, s.Token())
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("agent: %d %v", resp.StatusCode, parsed)
	}
	if parsed["content"] != "Dùng mt run yt-dlp <url>." {
		t.Errorf("content = %v", parsed["content"])
	}

	fake.mu.Lock()
	cfg := fake.agentCfg
	msgs := append([]core.AgentMessage(nil), fake.agentMsgs...)
	fake.mu.Unlock()
	if cfg.BaseURL != "https://api.groq.com/openai/v1" || cfg.APIKey != "gsk-abc" || cfg.Model != "m" {
		t.Errorf("cfg = %+v", cfg)
	}
	if len(msgs) != 1 || msgs[0].Content != "cách tải video?" {
		t.Errorf("msgs = %v", msgs)
	}

	if resp, _ := post(body, ""); resp.StatusCode != http.StatusForbidden {
		t.Errorf("agent thiếu token: %d", resp.StatusCode)
	}
	if resp, _ := post(`không phải JSON`, s.Token()); resp.StatusCode != http.StatusBadRequest {
		t.Errorf("agent JSON hỏng: %d", resp.StatusCode)
	}

	fake.agentErr = fmt.Errorf("API agent trả HTTP 401: invalid api key")
	if resp, parsed := post(body, s.Token()); resp.StatusCode != http.StatusBadRequest {
		t.Errorf("agent lỗi upstream: %d %v", resp.StatusCode, parsed)
	}
}

func TestServerURLAndPort(t *testing.T) {
	s, _ := newTestServer(t, newFakeRunner())
	if s.Port() <= 0 || s.Port() > 65535 {
		t.Errorf("Port = %d", s.Port())
	}
	want := fmt.Sprintf("http://127.0.0.1:%d/?token=%s", s.Port(), s.Token())
	if s.URL() != want {
		t.Errorf("URL() = %q, want %q", s.URL(), want)
	}
	if len(s.Token()) != 32 {
		t.Errorf("token dài %d, want 32", len(s.Token()))
	}
}

func TestToolsError(t *testing.T) {
	fake := newFakeRunner()
	fake.toolErr = fmt.Errorf("tools.toml hỏng")
	_, ts := newTestServer(t, fake)

	resp, err := http.Get(ts.URL + "/api/tools")
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusInternalServerError {
		t.Errorf("status = %d, want 500", resp.StatusCode)
	}
}

func TestAppAddEndpointRichPayload(t *testing.T) {
	s, ts := newTestServer(t, newFakeRunner())
	fake := s.runner.(*fakeRunner)

	body := `{"id":"web-tool","name":"Web Tool","kind":"git","target":"https://github.com/x/y.git",` +
		`"entry":"main.py","tags":["web","python"],"venv":true}`
	req, err := http.NewRequest(http.MethodPost, ts.URL+"/api/app/add", strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("X-MT-Token", s.Token())
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var parsed map[string]any
	_ = json.NewDecoder(resp.Body).Decode(&parsed)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("rich add: %d %v", resp.StatusCode, parsed)
	}
	if parsed["run_id"] != "install-1" {
		t.Errorf("run_id = %v, want install-1", parsed["run_id"])
	}

	fake.mu.Lock()
	calls := append([]customCall(nil), fake.customCalls...)
	fake.mu.Unlock()
	if len(calls) != 1 {
		t.Fatalf("customCalls = %+v", calls)
	}
	got := calls[0]
	if got.id != "web-tool" || got.name != "Web Tool" || got.kind != "git" ||
		got.target != "https://github.com/x/y.git" || got.entry != "main.py" || !got.venv {
		t.Errorf("customCall = %+v", got)
	}
	if len(got.tags) != 2 || got.tags[0] != "web" || got.tags[1] != "python" {
		t.Errorf("tags = %v", got.tags)
	}
}

func TestAppAddEndpointRichURLTool(t *testing.T) {
	s, ts := newTestServer(t, newFakeRunner())
	fake := s.runner.(*fakeRunner)

	body := `{"id":"docs","name":"Tài liệu","kind":"url","target":"https://example.com/docs"}`
	req, err := http.NewRequest(http.MethodPost, ts.URL+"/api/app/add", strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("X-MT-Token", s.Token())
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var parsed map[string]any
	_ = json.NewDecoder(resp.Body).Decode(&parsed)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("url add: %d %v", resp.StatusCode, parsed)
	}
	if parsed["ok"] != true || parsed["run_id"] != nil {
		t.Errorf("response = %v", parsed)
	}

	fake.mu.Lock()
	calls := append([]customCall(nil), fake.customCalls...)
	fake.mu.Unlock()
	if len(calls) != 1 || calls[0].kind != "url" || calls[0].venv != true {
		t.Errorf("customCalls = %+v", calls)
	}
}

func TestOpenToolDirEndpoint(t *testing.T) {
	s, ts := newTestServer(t, newFakeRunner())
	fake := s.runner.(*fakeRunner)

	origOpenDir := openDir
	opened := make([]string, 0, 4)
	openDir = func(path string) error {
		opened = append(opened, path)
		return nil
	}
	defer func() { openDir = origOpenDir }()

	post := func(body string, token string) (*http.Response, map[string]any) {
		t.Helper()
		req, err := http.NewRequest(http.MethodPost, ts.URL+"/api/tools/dl/open-dir", strings.NewReader(body))
		if err != nil {
			t.Fatal(err)
		}
		if token != "" {
			req.Header.Set("X-MT-Token", token)
		}
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		var parsed map[string]any
		_ = json.NewDecoder(resp.Body).Decode(&parsed)
		return resp, parsed
	}

	// Có dir trong body: tạo thư mục rồi mở đúng đường dẫn đó.
	target := filepath.Join(t.TempDir(), "mt-open-dir-test")
	body, err := json.Marshal(map[string]string{"dir": target})
	if err != nil {
		t.Fatal(err)
	}
	resp, parsed := post(string(body), s.Token())
	if resp.StatusCode != http.StatusOK || parsed["ok"] != "true" || parsed["dir"] != target {
		t.Fatalf("open-dir có dir: %d %v", resp.StatusCode, parsed)
	}
	if len(opened) != 1 || opened[0] != target {
		t.Errorf("opened = %v, want [%s]", opened, target)
	}
	if _, err := os.Stat(target); err != nil {
		t.Errorf("thư mục chưa được tạo: %v", err)
	}

	// Body rỗng: lấy thư mục từ runner.ToolOutDir.
	base := t.TempDir()
	fake.outDirReply = filepath.Join(base, "downloads")
	resp, parsed = post(`{}`, s.Token())
	if resp.StatusCode != http.StatusOK || parsed["dir"] != fake.outDirReply {
		t.Fatalf("open-dir body rỗng: %d %v", resp.StatusCode, parsed)
	}
	fake.mu.Lock()
	gotID := fake.outDirID
	fake.mu.Unlock()
	if gotID != "dl" {
		t.Errorf("outDirID = %q, want dl", gotID)
	}
	if len(opened) != 2 || opened[1] != fake.outDirReply {
		t.Errorf("opened = %v", opened)
	}

	// Runner lỗi → 400.
	fake.outDirErr = fmt.Errorf("không có tool")
	if resp, _ := post(`{}`, s.Token()); resp.StatusCode != http.StatusBadRequest {
		t.Errorf("open-dir runner lỗi: %d", resp.StatusCode)
	}
	fake.outDirErr = nil

	// Đường dẫn tương đối → 400.
	relBody, err := json.Marshal(map[string]string{"dir": "relative/path"})
	if err != nil {
		t.Fatal(err)
	}
	if resp, _ := post(string(relBody), s.Token()); resp.StatusCode != http.StatusBadRequest {
		t.Errorf("open-dir đường dẫn tương đối: %d", resp.StatusCode)
	}

	// Thiếu token → 403.
	if resp, _ := post(`{}`, ""); resp.StatusCode != http.StatusForbidden {
		t.Errorf("open-dir thiếu token: %d", resp.StatusCode)
	}
}
