package service

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"mytools/internal/appmgr"
	"mytools/internal/core"
	"mytools/internal/registry"
	"mytools/internal/runner"
)

const (
	historyCap      = 1000
	subBuffer       = 128
	runExpiry       = 30 * time.Minute
	janitorInterval = time.Minute
)

// Service chạy tool thay mặt giao diện web: mỗi lần chạy có runID, lịch sử event
// và các subscriber nhận event theo thời gian thực (SSE).
type Service struct {
	dir string
	mgr *appmgr.Manager

	mu      sync.Mutex
	runs    map[string]*run
	closed  bool
	stopJan chan struct{}
}

type run struct {
	id       string
	history  []core.Event
	subs     map[chan core.Event]struct{}
	stdin    io.WriteCloser
	cancel   context.CancelFunc
	process  *exec.Cmd
	done     bool
	finished time.Time
}

func New(dataDir string) *Service {
	s := &Service{
		dir:     dataDir,
		mgr:     appmgr.New(dataDir),
		runs:    make(map[string]*run),
		stopJan: make(chan struct{}),
	}
	go s.janitor()
	return s
}

func (s *Service) ListTools() ([]core.Tool, error) {
	return registry.Load(filepath.Join(s.dir, "tools.toml"))
}

// RunTool chạy tool và trả về runID để theo dõi qua Subscribe.
func (s *Service) RunTool(ctx context.Context, id string, params map[string]string) (string, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if strings.TrimSpace(id) == "" {
		return "", errors.New("thiếu id tool")
	}

	tools, err := s.ListTools()
	if err != nil {
		return "", err
	}
	tool := findTool(tools, id)
	if tool == nil {
		return "", fmt.Errorf("không có tool %q (xem \"mt list\")", id)
	}
	if tool.Kind != "app" {
		return "", fmt.Errorf("tool %q có kind %q: web chỉ chạy được kind \"app\"", id, tool.Kind)
	}
	if tool.App == "" {
		return "", fmt.Errorf("tool %q thiếu app", id)
	}

	values, err := resolveParams(tool, params)
	if err != nil {
		return "", err
	}
	runArgs, err := runner.ExpandArgs(tool.Args, values)
	if err != nil {
		return "", fmt.Errorf("tool %q: %w", id, err)
	}

	app, err := s.mgr.LoadApp(tool.App)
	if err != nil {
		return "", err
	}
	registry.ApplyLifecycle(tool, app)

	if err := s.checkTrusted(ctx, app); err != nil {
		return "", err
	}
	process, err := buildAppCommand(context.Background(), *app, runArgs)
	if err != nil {
		return "", err
	}

	runID, err := newRunID()
	if err != nil {
		return "", err
	}

	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return "", errors.New("server đã dừng")
	}
	s.runs[runID] = &run{id: runID, subs: make(map[chan core.Event]struct{})}
	s.mu.Unlock()

	if registry.IsDetached(tool.Lifecycle, app) {
		go s.runDetached(runID, tool.ID, app, process, runArgs)
	} else {
		go s.runForeground(runID, tool.ID, app, process, runArgs)
	}
	return runID, nil
}

// Subscribe trả về kênh event của một lần chạy: phát lại toàn bộ lịch sử rồi
// tiếp nối event mới. Hàm hủy trả về sẽ ngắt kênh; bỏ lỡ event nếu kênh đầy.
func (s *Service) Subscribe(runID string) (<-chan core.Event, func(), error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	r := s.runs[runID]
	if r == nil {
		return nil, nil, fmt.Errorf("không có lần chạy %q", runID)
	}

	ch := make(chan core.Event, len(r.history)+subBuffer)
	for _, event := range r.history {
		ch <- event
	}
	if r.done {
		// Run đã xong: replay lịch sử rồi đóng ngay để client nhận "closed".
		close(ch)
		return ch, func() {}, nil
	}
	r.subs[ch] = struct{}{}

	unsubscribe := func() {
		s.mu.Lock()
		defer s.mu.Unlock()
		if _, ok := r.subs[ch]; !ok {
			return
		}
		delete(r.subs, ch)
		close(ch)
	}
	return ch, unsubscribe, nil
}

// Answer gửi một dòng câu trả lời vào stdin của lần chạy đang chờ nhập liệu.
func (s *Service) Answer(runID, answer string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	r := s.runs[runID]
	if r == nil {
		return fmt.Errorf("không có lần chạy %q", runID)
	}
	if r.done || r.stdin == nil {
		return fmt.Errorf("lần chạy %q không chờ câu trả lời", runID)
	}

	if _, err := io.WriteString(r.stdin, strings.TrimRight(answer, "\r\n")+"\n"); err != nil {
		return fmt.Errorf("gửi câu trả lời cho %q: %w", runID, err)
	}
	return nil
}

// Cancel hủy lần chạy foreground: hủy context và giết tiến trình nếu còn sống.
func (s *Service) Cancel(runID string) error {
	s.mu.Lock()
	r := s.runs[runID]
	if r == nil {
		s.mu.Unlock()
		return fmt.Errorf("không có lần chạy %q", runID)
	}
	if r.done {
		s.mu.Unlock()
		return nil
	}
	cancel := r.cancel
	process := r.process
	s.mu.Unlock()

	if cancel != nil {
		cancel()
	}
	if process != nil && process.Process != nil {
		_ = process.Process.Kill()
	}
	return nil
}

// Close dừng dọn dẹp định kỳ và đóng kênh của mọi subscriber.
func (s *Service) Close() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return
	}
	s.closed = true
	close(s.stopJan)
	for _, r := range s.runs {
		for ch := range r.subs {
			close(ch)
		}
		r.subs = make(map[chan core.Event]struct{})
	}
}

func (s *Service) publish(runID string, event core.Event) {
	s.mu.Lock()
	defer s.mu.Unlock()
	r := s.runs[runID]
	if r == nil {
		return
	}
	r.history = append(r.history, event)
	if len(r.history) > historyCap {
		r.history = append([]core.Event(nil), r.history[len(r.history)-historyCap:]...)
	}
	for ch := range r.subs {
		select {
		case ch <- event:
		default:
		}
	}
}

func (s *Service) finishRun(runID string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	r := s.runs[runID]
	if r == nil {
		return
	}
	r.done = true
	r.finished = time.Now()
	// Đóng kênh subscriber để client SSE nhận sự kiện "closed" ngay khi run xong.
	for ch := range r.subs {
		close(ch)
	}
	r.subs = make(map[chan core.Event]struct{})
}

// sweep xóa các lần chạy đã xong quá hạn; kênh subscriber bị đóng khi xóa.
func (s *Service) sweep(now time.Time) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for id, r := range s.runs {
		if r.done && now.Sub(r.finished) > runExpiry {
			for ch := range r.subs {
				close(ch)
			}
			delete(s.runs, id)
		}
	}
}

func (s *Service) janitor() {
	ticker := time.NewTicker(janitorInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			s.sweep(time.Now())
		case <-s.stopJan:
			return
		}
	}
}

func (s *Service) runForeground(runID, toolID string, app *core.App, process *exec.Cmd, runArgs []string) {
	logFile, _, err := s.mgr.OpenRunLog(app.ID, "mt web", runArgs)
	if err != nil {
		s.publish(runID, core.Event{RunID: runID, Kind: "error", Text: fmt.Sprintf("Không mở được log: %v", err)})
		s.finishRun(runID)
		return
	}
	defer logFile.Close()

	// Dùng os.Pipe thay vì io.Pipe: exec.Cmd với stdin không phải *os.File
	// tạo goroutine copy chờ EOF của pipe, khiến cmd.Wait treo tới khi đóng cả hai đầu.
	pr, pw, err := os.Pipe()
	if err != nil {
		s.publish(runID, core.Event{RunID: runID, Kind: "error", Text: fmt.Sprintf("Tạo kênh nhập liệu: %v", err)})
		s.finishRun(runID)
		return
	}
	process.Stdin = pr

	runCtx, cancel := context.WithCancel(context.Background())
	s.mu.Lock()
	r := s.runs[runID]
	if r == nil {
		s.mu.Unlock()
		cancel()
		return
	}
	r.stdin = pw
	r.cancel = cancel
	r.process = process
	s.mu.Unlock()

	events := make(chan core.Event, 64)
	done := make(chan error, 1)
	go func() {
		done <- runner.Run(runCtx, process, runID, events)
		close(events)
	}()

	for event := range events {
		_, _ = fmt.Fprintln(logFile, event.Text)
		s.publish(runID, event)
	}
	runErr := <-done
	_ = pw.Close()
	_ = pr.Close()

	result := core.Event{
		RunID: runID,
		Kind:  "result",
		Text:  fmt.Sprintf("Hoàn thành %q.", toolID),
		Data:  map[string]any{"ok": true},
	}
	if runErr != nil {
		var exitErr *exec.ExitError
		switch {
		case errors.As(runErr, &exitErr):
			result.Text = fmt.Sprintf("Thất bại: %q thoát với mã %d", app.ID, exitErr.ExitCode())
		case errors.Is(runErr, context.Canceled):
			result.Text = "Đã hủy."
		default:
			result.Text = fmt.Sprintf("Thất bại: %v", runErr)
		}
		result.Data = map[string]any{"ok": false}
	}
	_, _ = fmt.Fprintln(logFile, result.Text)
	s.publish(runID, result)
	s.finishRun(runID)
}

func (s *Service) runDetached(runID, toolID string, app *core.App, process *exec.Cmd, runArgs []string) {
	fail := func(text string) {
		s.publish(runID, core.Event{RunID: runID, Kind: "error", Text: text})
		s.publish(runID, core.Event{RunID: runID, Kind: "result", Text: "Thất bại.", Data: map[string]any{"ok": false}})
		s.finishRun(runID)
	}

	if err := s.mgr.EnsureNotRunning(toolID); err != nil {
		fail(fmt.Sprintf("Không chạy được: %v", err))
		return
	}
	logFile, logPath, err := s.mgr.OpenRunLog(app.ID, "mt web", runArgs)
	if err != nil {
		fail(fmt.Sprintf("Không mở được log: %v", err))
		return
	}

	process.Stdout = logFile
	process.Stderr = logFile
	process.Stdin = nil
	process.SysProcAttr = runner.DetachedSysProcAttr()

	if err := process.Start(); err != nil {
		_ = logFile.Close()
		fail(fmt.Sprintf("Không mở được %q: %v", app.ID, err))
		return
	}
	pid := process.Process.Pid

	startTime, err := appmgr.ProcessStartTime(pid)
	if err != nil {
		_ = process.Process.Kill()
		_ = process.Process.Release()
		_ = logFile.Close()
		fail(fmt.Sprintf("Không đọc được thời điểm bắt đầu của %q (PID %d): %v", toolID, pid, err))
		return
	}

	state := appmgr.RunState{
		ID:        toolID,
		PID:       pid,
		StartTime: startTime,
		Cmd:       appmgr.CommandLine(process),
	}
	if err := s.mgr.SaveRunState(state); err != nil {
		_ = appmgr.StopProcess(state)
		_ = process.Process.Kill()
		_ = process.Process.Release()
		_ = logFile.Close()
		fail(fmt.Sprintf("Không lưu được trạng thái chạy của %q: %v", toolID, err))
		return
	}

	_ = logFile.Close()
	_ = process.Process.Release()

	s.mu.Lock()
	r := s.runs[runID]
	if r != nil {
		r.process = process
	}
	s.mu.Unlock()

	s.publish(runID, core.Event{
		RunID: runID,
		Kind:  "result",
		Text:  fmt.Sprintf("Đã chạy %q (PID %d). Log: %s", toolID, pid, logPath),
		Data:  map[string]any{"ok": true},
	})
	s.finishRun(runID)
}

func (s *Service) checkTrusted(ctx context.Context, app *core.App) error {
	if app.Source == "" {
		return nil
	}
	head, err := s.mgr.HeadCommit(ctx, app.ID)
	if err != nil {
		return err
	}
	if app.TrustedCommit == head {
		return nil
	}
	return fmt.Errorf("app %q có commit mới chưa được tin cậy; chạy \"mt run %s\" trong terminal để xác nhận", app.ID, app.ID)
}

func buildAppCommand(ctx context.Context, app core.App, args []string) (*exec.Cmd, error) {
	switch app.Runtime {
	case "binary":
		return runner.BinaryCommand(ctx, app, args)
	case "python":
		return runner.PythonCommand(ctx, app, args)
	case "node":
		return runner.NodeCommand(ctx, app, args)
	default:
		return nil, fmt.Errorf("runtime %q chưa hỗ trợ", app.Runtime)
	}
}

func findTool(tools []core.Tool, id string) *core.Tool {
	for i := range tools {
		if tools[i].ID == id {
			return &tools[i]
		}
	}
	return nil
}

func newRunID() (string, error) {
	var buf [8]byte
	if _, err := rand.Read(buf[:]); err != nil {
		return "", fmt.Errorf("tạo mã lần chạy: %w", err)
	}
	return hex.EncodeToString(buf[:]), nil
}

func resolveParams(tool *core.Tool, given map[string]string) (map[string]string, error) {
	if given == nil {
		given = map[string]string{}
	}

	values := make(map[string]string, len(tool.Params))
	var missing []string
	for _, param := range tool.Params {
		value := strings.TrimSpace(given[param.Name])
		if value == "" {
			def, err := paramDefault(param)
			if err != nil {
				return nil, fmt.Errorf("tool %q: %w", tool.ID, err)
			}
			value = def
		}

		if param.Required && param.Type != "bool" && value == "" {
			missing = append(missing, param.Name)
			continue
		}

		switch param.Type {
		case "path":
			expanded, err := runner.ExpandHome(value)
			if err != nil {
				return nil, fmt.Errorf("tool %q: %w", tool.ID, err)
			}
			value = expanded
		case "bool":
			if value == "" {
				value = "false"
			}
			if value != "true" && value != "false" {
				return nil, fmt.Errorf("tool %q: param %q phải là true hoặc false", tool.ID, param.Name)
			}
		case "int":
			if _, err := strconv.Atoi(value); err != nil {
				return nil, fmt.Errorf("tool %q: param %q phải là số nguyên", tool.ID, param.Name)
			}
		case "enum":
			if value != "" && !slices.Contains(param.Choices, value) {
				return nil, fmt.Errorf("tool %q: param %q: %q không hợp lệ (chọn: %s)", tool.ID, param.Name, value, strings.Join(param.Choices, ", "))
			}
		}
		values[param.Name] = value
	}

	if len(missing) > 0 {
		return nil, fmt.Errorf("tool %q: thiếu tham số bắt buộc: %s", tool.ID, strings.Join(missing, ", "))
	}
	return values, nil
}

func paramDefault(param core.Param) (string, error) {
	if param.Default == nil {
		return "", nil
	}
	switch param.Type {
	case "bool":
		value, ok := param.Default.(bool)
		if !ok {
			return "", fmt.Errorf("param %q: default phải là bool", param.Name)
		}
		return strconv.FormatBool(value), nil
	case "int":
		switch value := param.Default.(type) {
		case int64:
			return strconv.FormatInt(value, 10), nil
		case int:
			return strconv.Itoa(value), nil
		default:
			return "", fmt.Errorf("param %q: default phải là số nguyên", param.Name)
		}
	default:
		value, ok := param.Default.(string)
		if !ok {
			return "", fmt.Errorf("param %q: default phải là chuỗi", param.Name)
		}
		return value, nil
	}
}
