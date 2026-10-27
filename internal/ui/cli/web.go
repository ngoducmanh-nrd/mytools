package cli

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"syscall"

	"github.com/spf13/cobra"

	"mytools/internal/config"
	"mytools/internal/service"
	"mytools/internal/ui/web"
)

func newWebCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "web",
		Short: "Mở giao diện web local",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			port, err := resolveWebPort()
			if err != nil {
				return err
			}
			return startWebServer(cmd, port, false)
		},
	}
}

// runDefaultUI xử lý khi chạy "mt" không kèm lệnh con.
func runDefaultUI(cmd *cobra.Command) error {
	cfg, err := loadConfig()
	if err != nil {
		return err
	}
	return defaultUI(cmd, cfg)
}

func loadConfig() (*config.Config, error) {
	dir, err := config.ResolveDir()
	if err != nil {
		return nil, err
	}
	return config.Load(filepath.Join(dir, "config.toml"))
}

// startWeb là điểm nối để test thay thế việc chạy server thật.
var startWeb = startWebServer

func defaultUI(cmd *cobra.Command, cfg *config.Config) error {
	switch cfg.DefaultUI {
	case "web":
		port, err := webPort(cfg.WebPort)
		if err != nil {
			return err
		}
		return startWeb(cmd, port, true)
	case "", "cli":
		return cmd.Help()
	default:
		return fmt.Errorf("default_ui %q không hợp lệ (chỉ nhận cli|web)", cfg.DefaultUI)
	}
}

func resolveWebPort() (int, error) {
	cfg, err := loadConfig()
	if err != nil {
		return 0, err
	}
	return webPort(cfg.WebPort)
}

// webPort chuyển web_port trong config thành port: số nguyên 1-65535,
// "auto" (0) để hệ điều hành tự chọn port trống.
func webPort(value any) (int, error) {
	switch v := value.(type) {
	case nil:
		return 7878, nil
	case string:
		if v == "auto" {
			return 0, nil
		}
		n, err := strconv.Atoi(v)
		if err != nil {
			return 0, fmt.Errorf("web_port %q không hợp lệ (số nguyên hoặc \"auto\")", v)
		}
		return validWebPort(n)
	case int64:
		return validWebPort(int(v))
	case int:
		return validWebPort(v)
	default:
		return 0, fmt.Errorf("web_port không hợp lệ (số nguyên hoặc \"auto\")")
	}
}

func validWebPort(n int) (int, error) {
	if n < 1 || n > 65535 {
		return 0, fmt.Errorf("web_port %d ngoài khoảng 1-65535", n)
	}
	return n, nil
}

func startWebServer(cmd *cobra.Command, port int, openBrowser bool) error {
	dir, err := config.ResolveDir()
	if err != nil {
		return err
	}
	svc := service.New(dir)
	defer svc.Close()

	server, err := web.New(port, svc)
	if err != nil {
		return err
	}

	if _, err := fmt.Fprintf(cmd.OutOrStdout(), "Server đang chạy tại %s\n", server.URL()); err != nil {
		return err
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	if openBrowser {
		if err := web.OpenBrowser(server.URL()); err != nil {
			return fmt.Errorf("mở trình duyệt: %w", err)
		}
	}
	return server.Serve(ctx)
}
