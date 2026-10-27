package cli

import (
	"fmt"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"strings"

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
	cmd.AddCommand(newAppListCmd(), newAppAddCmd(), newAppRmCmd())
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
	return &cobra.Command{
		Use:   "add <id-builtin|url-binary>",
		Short: "Cài app",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			spec, err := specFromArgument(args[0])
			if err != nil {
				return err
			}
			mgr, err := newAppManager()
			if err != nil {
				return err
			}

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
	if app.DownloadedAt.IsZero() && app.Sha256 == "" {
		return "chưa cài"
	}
	if app.Runtime != "binary" || len(app.Entry) == 0 {
		return "đã cài"
	}
	info, err := os.Stat(filepath.Join(app.Dir, app.Entry[0]))
	if err == nil && !info.IsDir() {
		return "đã cài"
	}
	return "chưa cài"
}

func specFromArgument(arg string) (core.AppSpec, error) {
	if spec, ok := appmgr.BuiltinSpec(arg); ok {
		return spec, nil
	}

	parsed, err := url.Parse(arg)
	if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" {
		return core.AppSpec{}, fmt.Errorf("chưa hỗ trợ %q: dùng app builtin (yt-dlp) hoặc URL tải binary trực tiếp", arg)
	}

	name := path.Base(parsed.Path)
	if name == "." || name == "/" || name == "" {
		return core.AppSpec{}, fmt.Errorf("URL %q phải có tên file", arg)
	}
	ext := path.Ext(name)
	if ext == "" || ext == "." {
		return core.AppSpec{}, fmt.Errorf("URL %q phải trỏ trực tiếp tới file binary (có phần mở rộng)", arg)
	}

	id := sanitizeAppID(strings.TrimSuffix(name, ext))
	if id == "" {
		return core.AppSpec{}, fmt.Errorf("không suy ra được id app từ tên file %q", name)
	}

	return core.AppSpec{
		SchemaVersion: 1,
		ID:            id,
		Runtime:       "binary",
		Download:      arg,
		Entry:         []string{name},
	}, nil
}

func sanitizeAppID(name string) string {
	name = strings.ToLower(name)
	var builder strings.Builder
	for _, r := range name {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9', r == '-', r == '_':
			builder.WriteRune(r)
		default:
			builder.WriteRune('-')
		}
	}
	return strings.Trim(builder.String(), "-_")
}
