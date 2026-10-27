package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/spf13/cobra"

	"mytools/internal/appmgr"
	"mytools/internal/core"
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
			var tool *core.Tool
			for i := range tools {
				if tools[i].ID == args[0] {
					tool = &tools[i]
					break
				}
			}
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

			mgr, err := newAppManager()
			if err != nil {
				return err
			}
			app, err := mgr.LoadApp(tool.App)
			if err != nil {
				return err
			}

			runArgs := append([]string{}, tool.Args...)
			runArgs = append(runArgs, args[1:]...)
			return runApp(cmd, mgr, app, runArgs)
		},
	}
}

func runApp(cmd *cobra.Command, mgr *appmgr.Manager, app *core.App, args []string) error {
	runCtx := cmd.Context()
	if runCtx == nil {
		runCtx = context.Background()
	}
	buildCtx := runCtx
	if app.GUI {
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

	logFile, logPath, err := openRunLog(mgr.DataDir(), app.ID, args)
	if err != nil {
		return err
	}

	if app.GUI {
		return startDetached(cmd, process, logFile, logPath, app.ID)
	}
	defer logFile.Close()

	var mu sync.Mutex
	process.Stdout = &lockedWriter{mu: &mu, w: io.MultiWriter(cmd.OutOrStdout(), logFile)}
	process.Stderr = &lockedWriter{mu: &mu, w: io.MultiWriter(cmd.ErrOrStderr(), logFile)}
	process.Stdin = cmd.InOrStdin()

	if err := process.Run(); err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			return fmt.Errorf("app %q thoát với mã %d", app.ID, exitErr.ExitCode())
		}
		return fmt.Errorf("chạy app %q: %w", app.ID, err)
	}
	return nil
}

func buildAppCommand(ctx context.Context, app core.App, args []string) (*exec.Cmd, error) {
	switch app.Runtime {
	case "binary":
		return runner.BinaryCommand(ctx, app, args)
	case "python":
		return runner.PythonCommand(ctx, app, args)
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

func openRunLog(dataDir, appID string, args []string) (*os.File, string, error) {
	if strings.TrimSpace(dataDir) == "" {
		return nil, "", errors.New("thư mục dữ liệu app rỗng")
	}
	dir := filepath.Join(dataDir, "logs", appID)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, "", fmt.Errorf("tạo thư mục log %s: %w", dir, err)
	}

	logPath := filepath.Join(dir, time.Now().Format("2006-01-02")+".log")
	file, err := os.OpenFile(logPath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return nil, "", fmt.Errorf("mở log %s: %w", logPath, err)
	}

	header := fmt.Sprintf("[%s] mt run %s", time.Now().Format(time.RFC3339), appID)
	if len(args) > 0 {
		header += " " + strings.Join(args, " ")
	}
	if _, err := fmt.Fprintln(file, header); err != nil {
		_ = file.Close()
		return nil, "", fmt.Errorf("ghi log %s: %w", logPath, err)
	}
	return file, logPath, nil
}

func startDetached(cmd *cobra.Command, process *exec.Cmd, logFile *os.File, logPath, appID string) error {
	process.Stdout = logFile
	process.Stderr = logFile
	process.Stdin = nil
	process.SysProcAttr = guiSysProcAttr()

	if err := process.Start(); err != nil {
		_ = logFile.Close()
		return fmt.Errorf("mở app %q: %w", appID, err)
	}
	pid := process.Process.Pid
	_ = logFile.Close()

	if err := process.Process.Release(); err != nil {
		return fmt.Errorf("app %q đã chạy (PID %d) nhưng chưa giải phóng được tiến trình: %v", appID, pid, err)
	}
	_, err := fmt.Fprintf(cmd.OutOrStdout(), "Đã mở app %q (PID %d). Log: %s\n", appID, pid, logPath)
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

type lockedWriter struct {
	mu *sync.Mutex
	w  io.Writer
}

func (w *lockedWriter) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.w.Write(p)
}
