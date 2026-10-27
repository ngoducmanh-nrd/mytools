package service

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"strings"

	"mytools/internal/appmgr"
	"mytools/internal/core"
	"mytools/internal/registry"
	"mytools/internal/runner"
)

// InstallTool cài tool từ nguồn (id built-in, URL file binary, thư mục tool
// cục bộ, hoặc git URL) và phát log tiến trình như một lần chạy thường.
func (s *Service) InstallTool(ctx context.Context, source string) (string, error) {
	if strings.TrimSpace(source) == "" {
		return "", errors.New("thiếu nguồn cài đặt")
	}
	return s.startInstall(ctx, source, "", func(ctx context.Context, out chan<- core.Event) (*core.App, error) {
		return s.installApp(ctx, source, out)
	})
}

// InstallManualTool cài repo git chưa có app.toml: sinh app.toml từ tên hiển
// thị, lệnh chạy và cờ GUI do người dùng cung cấp rồi cài như bình thường.
func (s *Service) InstallManualTool(ctx context.Context, source, name, entry string, gui bool) (string, error) {
	if strings.TrimSpace(source) == "" {
		return "", errors.New("thiếu nguồn cài đặt")
	}
	fields, err := manualEntry(entry)
	if err != nil {
		return "", err
	}
	id := appmgr.SanitizeAppID(name)
	if id == "" {
		return "", fmt.Errorf("không tạo được id app từ tên %q", name)
	}
	spec := core.AppSpec{
		SchemaVersion: 1,
		ID:            id,
		Runtime:       "python",
		Entry:         fields,
		GUI:           gui,
	}
	return s.startInstall(ctx, source, name, func(ctx context.Context, out chan<- core.Event) (*core.App, error) {
		return s.installGitSpec(ctx, source, spec, out)
	})
}

// startInstall tạo runID và chạy hàm cài đặt trong goroutine; install nhận
// kênh out để phát log, trả về app vừa cài (hoặc lỗi). toolName là tên hiển
// thị khi ghi tool vào registry (rỗng thì dùng id app).
func (s *Service) startInstall(ctx context.Context, source, toolName string, install func(context.Context, chan<- core.Event) (*core.App, error)) (string, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	// Cài đặt phải tiếp tục sau khi handler trả về, không bị hủy cùng request.
	ctx = context.WithoutCancel(ctx)

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

	go s.installRun(ctx, runID, source, toolName, install)
	return runID, nil
}

// RemoveTool xóa tool khỏi registry; với kind "app" xóa luôn app tương ứng
// nếu vẫn còn cài.
func (s *Service) RemoveTool(id string) error {
	if strings.TrimSpace(id) == "" {
		return errors.New("thiếu id tool")
	}
	tools, err := s.ListTools()
	if err != nil {
		return err
	}
	tool := findTool(tools, id)
	if tool == nil {
		return fmt.Errorf("không có tool %q", id)
	}

	if tool.Kind == "app" && tool.App != "" {
		apps, err := s.mgr.List()
		if err != nil {
			return err
		}
		for _, app := range apps {
			if app.ID == tool.App {
				if err := s.mgr.Remove(tool.App); err != nil {
					return err
				}
				break
			}
		}
	}

	return registry.RemoveTool(filepath.Join(s.dir, "tools.toml"), id)
}

func (s *Service) installRun(ctx context.Context, runID, source, toolName string, install func(context.Context, chan<- core.Event) (*core.App, error)) {
	out := make(chan core.Event, 64)
	forwardDone := make(chan struct{})
	go func() {
		for event := range out {
			event.RunID = runID
			if event.Kind == "result" {
				event.Kind = "log"
			}
			s.publish(runID, event)
		}
		close(forwardDone)
	}()

	app, err := install(ctx, out)
	close(out)
	<-forwardDone

	if errors.Is(err, core.ErrAppConfigMissing) {
		// Repo clone được nhưng không có app.toml: không coi là lỗi, để
		// người dùng bổ sung thông tin qua /api/app/install-manual.
		s.publish(runID, core.Event{RunID: runID, Kind: "manual", Text: "Cần tạo app.toml thủ công.", Data: map[string]any{"source": source}})
		s.publish(runID, core.Event{RunID: runID, Kind: "result", Text: "Repo không có app.toml. Điền tên và lệnh chạy bên dưới để tiếp tục.", Data: map[string]any{"ok": false, "manual": true}})
		s.finishRun(runID)
		return
	}

	fail := func(text string) {
		s.publish(runID, core.Event{RunID: runID, Kind: "error", Text: text})
		s.publish(runID, core.Event{RunID: runID, Kind: "result", Text: "Cài đặt thất bại.", Data: map[string]any{"ok": false}})
		s.finishRun(runID)
	}

	if err != nil {
		fail(fmt.Sprintf("Cài đặt thất bại: %v", err))
		return
	}

	if err := s.registerTool(app.ID, toolName); err != nil {
		// Rollback app vừa cài để người dùng có thể cài lại từ đầu.
		if cleanupErr := s.mgr.Remove(app.ID); cleanupErr != nil {
			err = fmt.Errorf("%w (không dọn được thư mục app: %v)", err, cleanupErr)
		}
		fail(fmt.Sprintf("Đã cài app nhưng không ghi được tool: %v", err))
		return
	}

	s.publish(runID, core.Event{
		RunID: runID,
		Kind:  "result",
		Text:  fmt.Sprintf("Đã cài tool %q.", app.ID),
		Data:  map[string]any{"ok": true, "id": app.ID},
	})
	s.finishRun(runID)
}

// manualEntry phân tích lệnh chạy do người dùng nhập, ví dụ "python main.py".
func manualEntry(entry string) ([]string, error) {
	fields := strings.Fields(entry)
	if len(fields) < 2 {
		return nil, fmt.Errorf("lệnh chạy phải có dạng \"python main.py\" (đang là %q)", entry)
	}
	if !strings.EqualFold(fields[0], "python") {
		return nil, fmt.Errorf("lệnh chạy phải bắt đầu bằng \"python\", ví dụ \"python main.py\"")
	}
	fields[0] = "python"
	return fields, nil
}

func (s *Service) installGitSpec(ctx context.Context, source string, spec core.AppSpec, out chan<- core.Event) (*core.App, error) {
	emit(out, core.Event{Kind: "log", Text: fmt.Sprintf("Đang clone %s...", source)})
	app, err := s.mgr.InstallGit(ctx, source, spec)
	if err != nil {
		return nil, err
	}
	emit(out, core.Event{Kind: "log", Text: fmt.Sprintf("Đã clone app %q và tạo app.toml.", app.ID)})
	if err := s.installRuntime(ctx, *app, out); err != nil {
		return nil, removeOnFailure(s.mgr, *app, err)
	}
	return app, nil
}

func (s *Service) registerTool(id, name string) error {
	path := filepath.Join(s.dir, "tools.toml")
	tools, err := registry.Load(path)
	if err != nil {
		return err
	}
	if findTool(tools, id) != nil {
		return nil
	}
	if name == "" {
		name = id
	}
	return registry.AddTool(path, core.Tool{ID: id, Name: name, Kind: "app", App: id})
}

func (s *Service) installApp(ctx context.Context, source string, out chan<- core.Event) (*core.App, error) {
	if spec, ok := appmgr.BuiltinSpec(source); ok {
		emit(out, core.Event{Kind: "log", Text: fmt.Sprintf("Đang cài app %q...", spec.ID)})
		app, err := s.mgr.Add(spec)
		if err != nil {
			return nil, err
		}
		if err := runner.Install(ctx, *app, out); err != nil {
			return nil, removeOnFailure(s.mgr, *app, err)
		}
		return app, nil
	}

	if rawURL, ok := appmgr.DirectBinaryURL(source); ok {
		spec, err := appmgr.BinarySpec(rawURL)
		if err != nil {
			return nil, err
		}
		emit(out, core.Event{Kind: "log", Text: fmt.Sprintf("Đang cài app %q từ %s...", spec.ID, rawURL)})
		app, err := s.mgr.Add(spec)
		if err != nil {
			return nil, err
		}
		if err := runner.Install(ctx, *app, out); err != nil {
			return nil, removeOnFailure(s.mgr, *app, err)
		}
		return app, nil
	}

	if appmgr.IsLocalToolDir(source) {
		emit(out, core.Event{Kind: "log", Text: fmt.Sprintf("Đang cài từ %s...", source)})
		app, err := s.mgr.InstallLocal(source)
		if err != nil {
			return nil, err
		}
		if err := s.installRuntime(ctx, *app, out); err != nil {
			return nil, removeOnFailure(s.mgr, *app, err)
		}
		return app, nil
	}

	emit(out, core.Event{Kind: "log", Text: fmt.Sprintf("Đang clone %s...", source)})
	app, err := s.mgr.CloneGit(ctx, source, "")
	if err != nil {
		return nil, err
	}
	emit(out, core.Event{Kind: "log", Text: fmt.Sprintf("Đã clone app %q.", app.ID)})
	if err := s.installRuntime(ctx, *app, out); err != nil {
		return nil, removeOnFailure(s.mgr, *app, err)
	}
	return app, nil
}

func (s *Service) installRuntime(ctx context.Context, app core.App, out chan<- core.Event) error {
	switch app.Runtime {
	case "binary":
		return runner.Install(ctx, app, out)
	case "python":
		return runner.InstallPython(ctx, app, out)
	default:
		return fmt.Errorf("runtime %q chưa hỗ trợ", app.Runtime)
	}
}

// removeOnFailure dọn app vừa tạo khi bước cài sau thất bại; giữ app python
// để người dùng còn chạy "mt app rebuild".
func removeOnFailure(mgr *appmgr.Manager, app core.App, err error) error {
	if app.Runtime == "python" {
		return err
	}
	if cleanupErr := mgr.Remove(app.ID); cleanupErr != nil {
		return fmt.Errorf("%w (không dọn được thư mục app: %v)", err, cleanupErr)
	}
	return err
}

func emit(out chan<- core.Event, event core.Event) {
	if out == nil {
		return
	}
	select {
	case out <- event:
	default:
	}
}
