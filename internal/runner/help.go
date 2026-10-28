package runner

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"mytools/internal/core"
)

const (
	helpTimeout   = 15 * time.Second
	maxShownFlags = 30
)

var flagPattern = regexp.MustCompile(`--[A-Za-z0-9][A-Za-z0-9-]*`)

// HelpInfo chứa tóm tắt cờ hỗ trợ, toàn bộ output của "--help" và đường dẫn
// file help.txt đã ghi trong thư mục app.
type HelpInfo struct {
	Summary string
	Output  string
	Path    string
}

// HelpInfo chạy "<app> --help" (fallback "-h"), trích các cờ dạng --flag
// trong output, ghi toàn bộ output vào help.txt trong thư mục app và trả về
// tóm tắt kèm output đầy đủ. App GUI bị bỏ qua để không mở cửa sổ.
func ToolHelp(ctx context.Context, app core.App) (HelpInfo, error) {
	if app.GUI {
		return HelpInfo{}, nil
	}
	if ctx == nil {
		ctx = context.Background()
	}

	output, err := runHelp(ctx, app, "--help")
	if err != nil {
		firstErr := err
		output, err = runHelp(ctx, app, "-h")
		if err != nil {
			return HelpInfo{}, fmt.Errorf("%w (thử --help cũng thất bại: %v)", err, firstErr)
		}
	}

	flags := extractFlags(output)
	helpPath := filepath.Join(app.Dir, "help.txt")
	if err := writeHelpFile(helpPath, output); err != nil {
		return HelpInfo{}, err
	}
	return HelpInfo{
		Summary: formatFlagsSummary(flags, helpPath),
		Output:  string(output),
		Path:    helpPath,
	}, nil
}

// HelpFlags chạy "<app> --help" ngay sau khi cài và trả về chuỗi tóm tắt các
// cờ hỗ trợ để in ra màn hình; lỗi trả về không làm hỏng việc cài đặt.
func HelpFlags(ctx context.Context, app core.App) (string, error) {
	info, err := ToolHelp(ctx, app)
	if err != nil {
		return "", err
	}
	return info.Summary, nil
}

func runHelp(ctx context.Context, app core.App, flag string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(ctx, helpTimeout)
	defer cancel()

	cmd, err := helpCommand(ctx, app, flag)
	if err != nil {
		return nil, err
	}
	output, err := cmd.CombinedOutput()
	if err != nil {
		if ctx.Err() != nil {
			return nil, fmt.Errorf("%s hết giờ sau %s", flag, helpTimeout)
		}
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			return nil, fmt.Errorf("%s thoát với mã %d", flag, exitErr.ExitCode())
		}
		return nil, fmt.Errorf("%s: %w", flag, err)
	}
	return output, nil
}

func helpCommand(ctx context.Context, app core.App, flag string) (*exec.Cmd, error) {
	switch app.Runtime {
	case "binary":
		return BinaryCommand(ctx, app, []string{flag})
	case "python":
		return PythonCommand(ctx, app, []string{flag})
	case "node":
		return NodeCommand(ctx, app, []string{flag})
	default:
		return nil, fmt.Errorf("runtime %q chưa hỗ trợ", app.Runtime)
	}
}

func extractFlags(output []byte) []string {
	seen := make(map[string]struct{})
	var flags []string
	for _, loc := range flagPattern.FindAllStringIndex(string(output), -1) {
		match := string(output[loc[0]:loc[1]])
		// Bỏ qua cờ bị cắt dòng kiểu "--no-js-\n    runtimes".
		if strings.HasSuffix(match, "-") {
			rest := string(output[loc[1]:])
			idx := strings.IndexFunc(rest, func(r rune) bool {
				return r != ' ' && r != '\t' && r != '\r' && r != '\n'
			})
			if idx >= 0 {
				next := rune(rest[idx])
				if next == '-' || (next >= 'A' && next <= 'Z') || (next >= 'a' && next <= 'z') || (next >= '0' && next <= '9') {
					continue
				}
			}
		}
		if _, ok := seen[match]; ok {
			continue
		}
		seen[match] = struct{}{}
		flags = append(flags, match)
	}
	return flags
}

func formatFlagsSummary(flags []string, helpPath string) string {
	if len(flags) == 0 {
		return fmt.Sprintf("Không thấy cờ -- nào trong --help; xem %s", helpPath)
	}
	if len(flags) <= maxShownFlags {
		return fmt.Sprintf("Cờ hỗ trợ (từ --help): %s (xem %s)", strings.Join(flags, " "), helpPath)
	}
	return fmt.Sprintf("Cờ hỗ trợ (từ --help): %s … và %d cờ khác (xem %s)",
		strings.Join(flags[:maxShownFlags], " "), len(flags)-maxShownFlags, helpPath)
}

func writeHelpFile(path string, output []byte) error {
	tmpPath := path + ".tmp"
	file, err := os.OpenFile(tmpPath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o644)
	if err != nil {
		return fmt.Errorf("tạo %s: %w", tmpPath, err)
	}
	success := false
	defer func() {
		if success {
			return
		}
		_ = file.Close()
		_ = os.Remove(tmpPath)
	}()

	if _, err := file.Write(output); err != nil {
		return fmt.Errorf("ghi %s: %w", tmpPath, err)
	}
	if err := file.Sync(); err != nil {
		return fmt.Errorf("đồng bộ %s: %w", tmpPath, err)
	}
	if err := file.Close(); err != nil {
		return fmt.Errorf("đóng %s: %w", tmpPath, err)
	}
	if err := os.Rename(tmpPath, path); err != nil {
		return fmt.Errorf("đổi tên %s: %w", tmpPath, err)
	}
	success = true
	return nil
}
