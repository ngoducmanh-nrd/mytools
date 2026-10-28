package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"

	"github.com/spf13/cobra"

	"mytools/internal/appmgr"
	"mytools/internal/core"
	"mytools/internal/registry"
	"mytools/internal/runner"
)

func newRunCmd() *cobra.Command {
	return &cobra.Command{
		Use:                "run <tool-id> [args...]",
		Short:              "Chạy tool",
		DisableFlagParsing: true,
		Args:               cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			tools, err := loadTools()
			if err != nil {
				return err
			}
			tool := findTool(tools, args[0])
			if tool == nil {
				return fmt.Errorf("không có tool %q (xem \"mt list\")", args[0])
			}
			if tool.Kind != "app" {
				switch tool.Kind {
				case "url":
					return fmt.Errorf("tool %q có kind \"url\": dùng \"mt open %s\"", tool.ID, tool.ID)
				default:
					return fmt.Errorf("tool %q có kind %q: \"mt run\" hiện chỉ hỗ trợ kind \"app\"", tool.ID, tool.Kind)
				}
			}
			if tool.App == "" {
				return fmt.Errorf("tool %q thiếu app", tool.ID)
			}

			params, rest, err := parseToolArgs(tool, args[1:])
			if err != nil {
				return err
			}
			runArgs, err := runner.ExpandArgs(tool.Args, params)
			if err != nil {
				return fmt.Errorf("tool %q: %w", tool.ID, err)
			}
			runArgs = append(runArgs, rest...)

			mgr, err := newAppManager()
			if err != nil {
				return err
			}
			app, err := mgr.LoadApp(tool.App)
			if err != nil {
				return err
			}
			registry.ApplyLifecycle(tool, app)

			return runApp(cmd, mgr, app, runArgs, tool.ID, tool.Lifecycle, false)
		},
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

func runApp(cmd *cobra.Command, mgr *appmgr.Manager, app *core.App, args []string, targetID, lifecycle string, restart bool) error {
	runCtx := cmd.Context()
	if runCtx == nil {
		runCtx = context.Background()
	}
	detached := registry.IsDetached(lifecycle, app)
	buildCtx := runCtx
	if detached {
		buildCtx = context.Background()
	}

	process, err := buildAppCommand(buildCtx, *app, args)
	if err != nil {
		return err
	}

	trusted, err := confirmGitRepo(runCtx, cmd.InOrStdin(), cmd.OutOrStdout(), mgr, app)
	if err != nil {
		return err
	}
	if !trusted {
		_, err := fmt.Fprintln(cmd.OutOrStdout(), "Đã hủy.")
		return err
	}

	confirmed, err := confirmNpx(runCtx, cmd.InOrStdin(), cmd.OutOrStdout(), mgr, app)
	if err != nil {
		return err
	}
	if !confirmed {
		_, err := fmt.Fprintln(cmd.OutOrStdout(), "Đã hủy.")
		return err
	}

	if restart {
		if err := stopRun(cmd, mgr, targetID, false); err != nil {
			return err
		}
	}

	if detached && !restart {
		if err := mgr.EnsureNotRunning(targetID); err != nil {
			return err
		}
	}

	logFile, logPath, err := mgr.OpenRunLog(app.ID, "mt run", args)
	if err != nil {
		return err
	}

	if detached {
		return startDetached(cmd, mgr, process, logFile, logPath, targetID)
	}
	defer logFile.Close()

	process.Stdin = cmd.InOrStdin()
	if err := runForeground(cmd, process, app.ID, logFile); err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			return fmt.Errorf("app %q thoát với mã %d", app.ID, exitErr.ExitCode())
		}
		if errors.Is(err, context.Canceled) {
			return err
		}
		return fmt.Errorf("chạy app %q: %w", app.ID, err)
	}
	return nil
}

// runForeground chạy app ở foreground, in từng Event ra terminal và ghi toàn bộ vào file log.
func runForeground(cmd *cobra.Command, process *exec.Cmd, runID string, logFile *os.File) error {
	ctx := cmd.Context()
	if ctx == nil {
		ctx = context.Background()
	}

	events := make(chan core.Event, 64)
	done := make(chan error, 1)
	go func() {
		done <- runner.Run(ctx, process, runID, events)
		close(events)
	}()

	var writeErr error
	writeLine := func(w io.Writer, text string) {
		if w == nil {
			return
		}
		if _, err := fmt.Fprintln(w, text); err != nil && writeErr == nil {
			writeErr = err
		}
	}

	for event := range events {
		switch event.Kind {
		case "ask":
			writePrompt(cmd.OutOrStdout(), event.Text)
			writeLine(logFile, event.Text)
		case "error":
			writeLine(cmd.ErrOrStderr(), event.Text)
			writeLine(logFile, event.Text)
		default:
			writeLine(cmd.OutOrStdout(), event.Text)
			writeLine(logFile, event.Text)
		}
	}

	if err := <-done; err != nil {
		return err
	}
	return writeErr
}

func writePrompt(w io.Writer, text string) {
	if w == nil {
		return
	}
	_, _ = fmt.Fprint(w, text)
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

func confirmGitRepo(ctx context.Context, in io.Reader, out io.Writer, mgr *appmgr.Manager, app *core.App) (bool, error) {
	if app.Source == "" {
		return true, nil
	}

	head, err := mgr.HeadCommit(ctx, app.ID)
	if err != nil {
		return false, err
	}
	if app.TrustedCommit == head {
		return true, nil
	}

	if _, err := fmt.Fprint(out, "Repo lạ, chạy không? (y/N) "); err != nil {
		return false, err
	}
	answer, err := readAnswer(in)
	if err != nil {
		return false, err
	}

	switch strings.ToLower(strings.TrimSpace(answer)) {
	case "y", "yes":
		state := app.AppState
		state.URL = app.Source
		state.TrustedCommit = head
		if err := mgr.SaveState(app.ID, state); err != nil {
			return false, err
		}
		app.AppState = state
		return true, nil
	default:
		return false, nil
	}
}

// confirmNpx hỏi xác nhận khi tool node chạy bằng npx lần đầu: npx có thể tải
// mã từ Internet nên cần người dùng đồng ý; "y" ghi trusted_npx vào state.json
// để lần sau không hỏi lại.
func confirmNpx(ctx context.Context, in io.Reader, out io.Writer, mgr *appmgr.Manager, app *core.App) (bool, error) {
	if !runner.UsesNpx(*app) || app.TrustedNpx {
		return true, nil
	}

	if _, err := fmt.Fprint(out, runner.NpxWarning+" "); err != nil {
		return false, err
	}
	answer, err := readAnswer(in)
	if err != nil {
		return false, err
	}

	switch strings.ToLower(strings.TrimSpace(answer)) {
	case "y", "yes":
		state := app.AppState
		state.TrustedNpx = true
		if err := mgr.SaveState(app.ID, state); err != nil {
			return false, err
		}
		app.AppState = state
		return true, nil
	default:
		return false, nil
	}
}

func startDetached(cmd *cobra.Command, mgr *appmgr.Manager, process *exec.Cmd, logFile *os.File, logPath, id string) error {
	process.Stdout = logFile
	process.Stderr = logFile
	process.Stdin = nil
	process.SysProcAttr = runner.DetachedSysProcAttr()

	if err := process.Start(); err != nil {
		_ = logFile.Close()
		return fmt.Errorf("mở %q: %w", id, err)
	}
	pid := process.Process.Pid

	startTime, err := appmgr.ProcessStartTime(pid)
	if err != nil {
		_ = process.Process.Kill()
		_ = process.Process.Release()
		_ = logFile.Close()
		return fmt.Errorf("không đọc được thời điểm bắt đầu của %q (PID %d): %w", id, pid, err)
	}

	state := appmgr.RunState{
		ID:        id,
		PID:       pid,
		StartTime: startTime,
		Cmd:       appmgr.CommandLine(process),
	}
	if err := mgr.SaveRunState(state); err != nil {
		_ = appmgr.StopProcess(state)
		_ = process.Process.Kill()
		_ = process.Process.Release()
		_ = logFile.Close()
		return fmt.Errorf("lưu trạng thái chạy của %q: %w", id, err)
	}

	_ = logFile.Close()
	if err := process.Process.Release(); err != nil {
		return fmt.Errorf("%q đã chạy (PID %d) và đã ghi trạng thái, nhưng chưa giải phóng được tiến trình: %v", id, pid, err)
	}

	_, err = fmt.Fprintf(cmd.OutOrStdout(), "Đã chạy %q (PID %d). Log: %s\n", id, pid, logPath)
	return err
}

func readAnswer(r io.Reader) (string, error) {
	var builder strings.Builder
	var buf [1]byte
	for {
		n, err := r.Read(buf[:])
		if n > 0 {
			if buf[0] == '\n' {
				return builder.String(), nil
			}
			builder.WriteByte(buf[0])
		}
		if err != nil {
			if errors.Is(err, io.EOF) {
				return builder.String(), nil
			}
			return "", fmt.Errorf("đọc câu trả lời: %w", err)
		}
	}
}
