package runner

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"runtime"
)

var stdout io.Writer = os.Stdout

func Open(ctx context.Context, url string) error {
	if os.Getenv("MT_DRY_RUN") == "1" {
		_, err := fmt.Fprintln(stdout, url)
		return err
	}

	cmd, err := browserCommand(ctx, url)
	if err != nil {
		return err
	}
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("mở %s: %w", url, err)
	}
	return cmd.Process.Release()
}

func browserCommand(ctx context.Context, url string) (*exec.Cmd, error) {
	switch runtime.GOOS {
	case "windows":
		return exec.CommandContext(ctx, "rundll32", "url.dll,FileProtocolHandler", url), nil
	case "linux":
		return exec.CommandContext(ctx, "xdg-open", url), nil
	default:
		return nil, fmt.Errorf("hệ điều hành %s chưa hỗ trợ mở URL", runtime.GOOS)
	}
}
