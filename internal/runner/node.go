package runner

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"mytools/internal/core"
)

// NodeCommand dựng lệnh chạy app Node từ entry, ví dụ ["npm", "start"].
func NodeCommand(ctx context.Context, app core.App, args []string) (*exec.Cmd, error) {
	if app.Runtime != "node" {
		return nil, fmt.Errorf("runtime %q không phải node", app.Runtime)
	}
	if len(app.Entry) == 0 {
		return nil, errors.New(`app.Entry: thiếu trường "entry"`)
	}
	if app.Dir == "" || !filepath.IsAbs(app.Dir) {
		return nil, fmt.Errorf("thư mục app %q không phải đường dẫn tuyệt đối", app.Dir)
	}
	if ctx == nil {
		ctx = context.Background()
	}

	commandPath, err := resolveNodeTool(app.Entry[0])
	if err != nil {
		return nil, err
	}

	commandArgs := append([]string{}, app.Entry[1:]...)
	commandArgs = append(commandArgs, args...)
	cmd := exec.CommandContext(ctx, commandPath, commandArgs...)
	cmd.Dir = app.Dir
	return cmd, nil
}

// InstallNode cài dependencies bằng npm nếu app chưa có node_modules, sau đó
// chạy lệnh build trong app.Build (nếu có).
func InstallNode(ctx context.Context, app core.App, out chan<- core.Event) error {
	if ctx == nil {
		ctx = context.Background()
	}
	if app.Runtime != "node" {
		return fmt.Errorf("runtime %q không dùng runner node (nhận %q)", app.Runtime, "node")
	}
	if app.Dir == "" || !filepath.IsAbs(app.Dir) {
		return fmt.Errorf("thư mục app %q không phải đường dẫn tuyệt đối", app.Dir)
	}
	if len(app.Entry) == 0 {
		return errors.New(`app.Entry: thiếu trường "entry"`)
	}

	nodeModules := filepath.Join(app.Dir, "node_modules")
	hasNodeModules := false
	if info, err := os.Stat(nodeModules); err == nil {
		if !info.IsDir() {
			return fmt.Errorf("%s là file, không phải thư mục", nodeModules)
		}
		hasNodeModules = true
		emit(out, core.Event{RunID: app.ID, Kind: "log", Text: "Đã có node_modules, bỏ qua npm install."})
	} else if !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("kiểm tra %s: %w", nodeModules, err)
	}

	if !hasNodeModules {
		packageJSON := filepath.Join(app.Dir, "package.json")
		if info, err := os.Stat(packageJSON); err != nil {
			if errors.Is(err, os.ErrNotExist) {
				emit(out, core.Event{RunID: app.ID, Kind: "log", Text: "Không có package.json, bỏ qua npm install."})
			} else {
				return fmt.Errorf("kiểm tra %s: %w", packageJSON, err)
			}
		} else if info.IsDir() {
			return fmt.Errorf("%s là thư mục, không phải file", packageJSON)
		} else {
			npmPath, err := resolveNodeTool("npm")
			if err != nil {
				return err
			}
			emit(out, core.Event{RunID: app.ID, Kind: "log", Text: "Đang chạy npm install --no-audit --no-fund"})
			cmd := exec.CommandContext(ctx, npmPath, "install", "--no-audit", "--no-fund")
			cmd.Dir = app.Dir
			if err := Run(ctx, cmd, app.ID, out); err != nil {
				return fmt.Errorf("npm install thất bại: %w", err)
			}
			emit(out, core.Event{RunID: app.ID, Kind: "log", Text: "Đã cài dependencies Node"})
		}
	}

	if len(app.Build) > 0 {
		buildPath, err := resolveNodeTool(app.Build[0])
		if err != nil {
			return fmt.Errorf("build: %w", err)
		}
		buildCommand := strings.Join(app.Build, " ")
		emit(out, core.Event{RunID: app.ID, Kind: "log", Text: "Đang chạy build: " + buildCommand})
		buildCmd := exec.CommandContext(ctx, buildPath, app.Build[1:]...)
		buildCmd.Dir = app.Dir
		if err := Run(ctx, buildCmd, app.ID, out); err != nil {
			return fmt.Errorf("build %q thất bại: %w", buildCommand, err)
		}
		emit(out, core.Event{RunID: app.ID, Kind: "log", Text: "Đã build app Node"})
	}

	emit(out, core.Event{RunID: app.ID, Kind: "result", Text: "Môi trường Node đã sẵn sàng"})
	return nil
}

// resolveNodeTool tìm lệnh trong PATH; trên Windows LookPath tự nhận npm.cmd.
func resolveNodeTool(name string) (string, error) {
	if name == "" || filepath.Base(name) != name {
		return "", fmt.Errorf("lệnh %q phải là tên lệnh trong PATH", name)
	}
	path, err := exec.LookPath(name)
	if err != nil {
		return "", fmt.Errorf("không tìm thấy %q trong PATH; cài Node.js tại https://nodejs.org/", name)
	}
	return path, nil
}
