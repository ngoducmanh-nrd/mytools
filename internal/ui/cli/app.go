package cli

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"

	"mytools/internal/appmgr"
	"mytools/internal/config"
	"mytools/internal/core"
	"mytools/internal/runner"
)

func newAppCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "app",
		Short: "Quản lý app",
	}
	cmd.AddCommand(newAppListCmd(), newAppAddCmd(), newAppRunCmd(), newAppRebuildCmd(), newAppRmCmd())
	return cmd
}

func newAppListCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "Liệt kê app",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			mgr, err := newAppManager()
			if err != nil {
				return err
			}
			apps, err := mgr.List()
			if err != nil {
				return err
			}
			if len(apps) == 0 {
				_, err := fmt.Fprintln(cmd.OutOrStdout(), "Chưa có app nào.")
				return err
			}
			for _, app := range apps {
				if _, err := fmt.Fprintf(cmd.OutOrStdout(), "%-16s  %-8s  %s\n", app.ID, app.Runtime, appStatus(app)); err != nil {
					return err
				}
			}
			return nil
		},
	}
}

func newAppAddCmd() *cobra.Command {
	var ref string
	cmd := &cobra.Command{
		Use:   "add <id-builtin|url-binary|url-git>",
		Short: "Cài app",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			mgr, err := newAppManager()
			if err != nil {
				return err
			}

			arg := args[0]
			if spec, ok := appmgr.BuiltinSpec(arg); ok {
				return addBinaryApp(cmd, mgr, spec)
			}
			if rawURL, ok := appmgr.DirectBinaryURL(arg); ok {
				spec, err := appmgr.BinarySpec(rawURL)
				if err != nil {
					return err
				}
				return addBinaryApp(cmd, mgr, spec)
			}
			if appmgr.IsLocalToolDir(arg) {
				return addLocalApp(cmd, mgr, arg)
			}
			return addGitApp(cmd, mgr, arg, ref)
		},
	}
	cmd.Flags().StringVar(&ref, "ref", "", "commit, tag hoặc branch cho app clone từ git")
	return cmd
}

func addBinaryApp(cmd *cobra.Command, mgr *appmgr.Manager, spec core.AppSpec) error {
	app, err := mgr.Add(spec)
	if err != nil {
		return err
	}

	out := cmd.OutOrStdout()
	if _, err := fmt.Fprintf(out, "Đang tải %s...\n", spec.Download); err != nil {
		return err
	}
	if err := runner.Install(cmd.Context(), *app, nil); err != nil {
		if cleanupErr := mgr.Remove(spec.ID); cleanupErr != nil {
			return fmt.Errorf("%w (không dọn được thư mục app: %v)", err, cleanupErr)
		}
		return err
	}

	installed, err := mgr.LoadApp(spec.ID)
	if err != nil {
		return err
	}
	if _, err := fmt.Fprintf(out, "Đã cài app %q vào %s\n", installed.ID, installed.Dir); err != nil {
		return err
	}
	if installed.Sha256 != "" {
		_, err = fmt.Fprintf(out, "sha256: %s\n", installed.Sha256)
	}
	return err
}

func addLocalApp(cmd *cobra.Command, mgr *appmgr.Manager, srcDir string) error {
	out := cmd.OutOrStdout()
	if _, err := fmt.Fprintf(out, "Đang cài từ %s...\n", srcDir); err != nil {
		return err
	}

	app, err := mgr.InstallLocal(srcDir)
	if err != nil {
		return err
	}
	if _, err := fmt.Fprintf(out, "Đã cài app %q vào %s\n", app.ID, app.Dir); err != nil {
		return err
	}

	var installErr error
	switch app.Runtime {
	case "binary":
		if _, err := fmt.Fprintf(out, "Đang tải %s...\n", app.Download); err != nil {
			return err
		}
		installErr = runner.Install(cmd.Context(), *app, nil)
	case "python":
		if _, err := fmt.Fprintf(out, "Đang tạo môi trường Python...\n"); err != nil {
			return err
		}
		installErr = runner.InstallPython(cmd.Context(), *app, nil)
	case "node":
		installErr = runNodeInstall(cmd, *app)
	default:
		installErr = fmt.Errorf("runtime %q chưa hỗ trợ", app.Runtime)
	}
	if installErr != nil {
		hint := fmt.Sprintf("chạy \"mt app rebuild %s\"", app.ID)
		if app.Runtime != "python" {
			hint = fmt.Sprintf("xóa bằng \"mt app rm %s\" rồi thêm lại", app.ID)
		}
		return fmt.Errorf("%w\nApp %q đã được cài tại %s; sửa lỗi rồi %s.", installErr, app.ID, app.Dir, hint)
	}

	if app.Runtime == "python" {
		_, err = fmt.Fprintf(out, "Đã cài app %q vào %s (env: %s)\n", app.ID, app.Dir, filepath.Join(app.Dir, "env"))
		return err
	}

	installed, err := mgr.LoadApp(app.ID)
	if err != nil {
		return err
	}
	if _, err := fmt.Fprintf(out, "Đã cài app %q vào %s\n", installed.ID, installed.Dir); err != nil {
		return err
	}
	if installed.Sha256 != "" {
		_, err = fmt.Fprintf(out, "sha256: %s\n", installed.Sha256)
	}
	return err
}

func addGitApp(cmd *cobra.Command, mgr *appmgr.Manager, rawURL, ref string) error {
	out := cmd.OutOrStdout()
	if _, err := fmt.Fprintf(out, "Đang clone %s...\n", rawURL); err != nil {
		return err
	}

	app, err := mgr.CloneGit(cmd.Context(), rawURL, ref)
	if err != nil {
		return err
	}
	if _, err := fmt.Fprintf(out, "Đã clone app %q vào %s\n", app.ID, app.Dir); err != nil {
		return err
	}
	if app.Ref != "" {
		if _, err := fmt.Fprintf(out, "commit: %s\n", app.Ref); err != nil {
			return err
		}
	}

	var installErr error
	switch app.Runtime {
	case "binary":
		if _, err := fmt.Fprintf(out, "Đang tải %s...\n", app.Download); err != nil {
			return err
		}
		installErr = runner.Install(cmd.Context(), *app, nil)
	case "python":
		if _, err := fmt.Fprintf(out, "Đang tạo môi trường Python...\n"); err != nil {
			return err
		}
		installErr = runner.InstallPython(cmd.Context(), *app, nil)
	case "node":
		installErr = runNodeInstall(cmd, *app)
	default:
		installErr = fmt.Errorf("runtime %q chưa hỗ trợ", app.Runtime)
	}
	if installErr != nil {
		hint := fmt.Sprintf("chạy \"mt app rebuild %s\"", app.ID)
		if app.Runtime != "python" {
			hint = fmt.Sprintf("xóa bằng \"mt app rm %s\" rồi thêm lại", app.ID)
		}
		return fmt.Errorf("%w\nApp %q đã được clone tại %s; sửa lỗi rồi %s.", installErr, app.ID, app.Dir, hint)
	}

	if app.Runtime == "python" {
		_, err = fmt.Fprintf(out, "Đã cài app %q vào %s (env: %s)\n", app.ID, app.Dir, filepath.Join(app.Dir, "env"))
		return err
	}

	installed, err := mgr.LoadApp(app.ID)
	if err != nil {
		return err
	}
	if _, err := fmt.Fprintf(out, "Đã cài app %q vào %s\n", installed.ID, installed.Dir); err != nil {
		return err
	}
	if installed.Sha256 != "" {
		_, err = fmt.Fprintf(out, "sha256: %s\n", installed.Sha256)
	}
	return err
}

// runNodeInstall chạy npm install và in log ra terminal.
func runNodeInstall(cmd *cobra.Command, app core.App) error {
	ctx := cmd.Context()
	if ctx == nil {
		ctx = context.Background()
	}

	events := make(chan core.Event, 64)
	done := make(chan error, 1)
	go func() {
		done <- runner.InstallNode(ctx, app, events)
		close(events)
	}()

	for event := range events {
		writer := cmd.OutOrStdout()
		if event.Kind == "error" {
			writer = cmd.ErrOrStderr()
		}
		_, _ = fmt.Fprintln(writer, event.Text)
	}
	return <-done
}

func newAppRunCmd() *cobra.Command {
	return &cobra.Command{
		Use:                "run <id> [args...]",
		Short:              "Chạy app",
		DisableFlagParsing: true,
		Args:               cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			mgr, err := newAppManager()
			if err != nil {
				return err
			}
			app, err := mgr.LoadApp(args[0])
			if err != nil {
				return err
			}
			return runApp(cmd, mgr, app, args[1:], app.ID, "", false)
		},
	}
}

func newAppRebuildCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "rebuild <id>",
		Short: "Dựng lại môi trường app",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			mgr, err := newAppManager()
			if err != nil {
				return err
			}
			out := cmd.OutOrStdout()
			if _, err := fmt.Fprintf(out, "Đang dựng lại môi trường cho app %q...\n", args[0]); err != nil {
				return err
			}
			if err := mgr.Rebuild(cmd.Context(), args[0], nil); err != nil {
				return err
			}
			_, err = fmt.Fprintf(out, "Đã dựng lại môi trường app %q.\n", args[0])
			return err
		},
	}
}

func newAppRmCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "rm <id>",
		Short: "Xóa app",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			mgr, err := newAppManager()
			if err != nil {
				return err
			}
			if err := mgr.Remove(args[0]); err != nil {
				return err
			}
			_, err = fmt.Fprintf(cmd.OutOrStdout(), "Đã xóa app %q.\n", args[0])
			return err
		},
	}
}

func newAppManager() (*appmgr.Manager, error) {
	dir, err := config.ResolveDir()
	if err != nil {
		return nil, err
	}
	return appmgr.New(dir), nil
}

func appStatus(app core.App) string {
	switch app.Runtime {
	case "binary":
		if app.DownloadedAt.IsZero() && app.Sha256 == "" {
			return "chưa cài"
		}
		if len(app.Entry) == 0 {
			return "chưa cài"
		}
		info, err := os.Stat(filepath.Join(app.Dir, app.Entry[0]))
		if err == nil && !info.IsDir() {
			return "đã cài"
		}
		return "chưa cài"
	case "python":
		if runner.PythonReady(app) {
			return "đã cài"
		}
		return "chưa cài"
	case "node":
		if info, err := os.Stat(filepath.Join(app.Dir, "node_modules")); err == nil && info.IsDir() {
			return "đã cài"
		}
		return "chưa cài"
	default:
		return "đã cài"
	}
}
