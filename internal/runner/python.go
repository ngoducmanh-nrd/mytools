package runner

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"mytools/internal/core"
)

type pythonInterpreter struct {
	path string
	args []string
}

var pythonCandidates = []pythonInterpreter{
	{path: "py", args: []string{"-3"}},
	{path: "python3"},
	{path: "python"},
}

func InstallPython(ctx context.Context, app core.App, out chan<- core.Event) error {
	if app.Runtime != "python" {
		return fmt.Errorf("runtime %q không dùng runner python (nhận %q)", app.Runtime, "python")
	}
	if app.Dir == "" || !filepath.IsAbs(app.Dir) {
		return fmt.Errorf("thư mục app %q không phải đường dẫn tuyệt đối", app.Dir)
	}
	if len(app.Entry) < 2 || app.Entry[0] != "python" {
		return errors.New(`app.Entry phải bắt đầu bằng "python" và có script, ví dụ ["python", "main.py"]`)
	}

	reqPath, reqHash, err := requirementsFile(app)
	if err != nil {
		return err
	}

	venvPython, err := venvPythonPath(app.Dir)
	if err != nil {
		return err
	}
	envDir := filepath.Join(app.Dir, "env")

	venvCreated := false
	if info, statErr := os.Stat(venvPython); statErr != nil || info.IsDir() {
		interpreter, findErr := findPython(ctx)
		if findErr != nil {
			return findErr
		}
		emit(out, core.Event{RunID: app.ID, Kind: "log", Text: "Đang tạo venv tại " + envDir})
		if createErr := createVenv(ctx, interpreter, envDir); createErr != nil {
			return createErr
		}
		if _, statErr := os.Stat(venvPython); statErr != nil {
			return fmt.Errorf("venv đã tạo nhưng thiếu %s: %w", venvPython, statErr)
		}
		venvCreated = true
	}

	if reqPath != "" && (venvCreated || app.RequirementsHash != reqHash) {
		emit(out, core.Event{RunID: app.ID, Kind: "log", Text: "Đang cài requirements từ " + app.Requirements})
		if installErr := installRequirements(ctx, venvPython, reqPath, app.Dir); installErr != nil {
			return installErr
		}
	}

	state := app.AppState
	state.URL = app.Source
	state.RequirementsHash = reqHash
	if err := writeState(app.Dir, state); err != nil {
		return err
	}

	emit(out, core.Event{
		RunID: app.ID,
		Kind:  "result",
		Text:  "Đã cài môi trường Python",
		Data:  map[string]any{"requirements_hash": reqHash},
	})
	return nil
}

func PythonCommand(ctx context.Context, app core.App, args []string) (*exec.Cmd, error) {
	if app.Runtime != "python" {
		return nil, fmt.Errorf("runtime %q không phải python", app.Runtime)
	}
	if len(app.Entry) == 0 || app.Entry[0] != "python" {
		return nil, errors.New(`app.Entry phải bắt đầu bằng "python"`)
	}

	venvPython, err := venvPythonPath(app.Dir)
	if err != nil {
		return nil, err
	}
	info, err := os.Stat(venvPython)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, fmt.Errorf("app %q chưa có môi trường Python (thiếu %s); chạy \"mt app rebuild %s\"", app.ID, venvPython, app.ID)
		}
		return nil, fmt.Errorf("kiểm tra %s: %w", venvPython, err)
	}
	if info.IsDir() {
		return nil, fmt.Errorf("%s là thư mục, không phải python", venvPython)
	}

	commandArgs := append([]string{}, app.Entry[1:]...)
	commandArgs = append(commandArgs, args...)
	cmd := exec.CommandContext(ctx, venvPython, commandArgs...)
	cmd.Dir = app.Dir
	cmd.Env = append(os.Environ(), "PYTHONIOENCODING=utf-8")
	return cmd, nil
}

func PythonReady(app core.App) bool {
	if app.Runtime != "python" {
		return false
	}
	venvPython, err := venvPythonPath(app.Dir)
	if err != nil {
		return false
	}
	info, err := os.Stat(venvPython)
	return err == nil && !info.IsDir()
}

func venvPythonPath(appDir string) (string, error) {
	if appDir == "" || !filepath.IsAbs(appDir) {
		return "", fmt.Errorf("thư mục app %q không phải đường dẫn tuyệt đối", appDir)
	}
	if runtime.GOOS == "windows" {
		return filepath.Join(appDir, "env", "Scripts", "python.exe"), nil
	}
	return filepath.Join(appDir, "env", "bin", "python"), nil
}

func findPython(ctx context.Context) (pythonInterpreter, error) {
	attempts := make([]string, 0, len(pythonCandidates))
	for _, candidate := range pythonCandidates {
		path, err := exec.LookPath(candidate.path)
		if err != nil {
			continue
		}
		args := append(append([]string{}, candidate.args...), "--version")
		cmd := exec.CommandContext(ctx, path, args...)
		output, err := cmd.CombinedOutput()
		if err != nil {
			attempts = append(attempts, fmt.Sprintf("%s: %v", strings.Join(append([]string{candidate.path}, candidate.args...), " "), err))
			continue
		}
		version := firstNonEmptyLine(string(output))
		if strings.HasPrefix(version, "Python 3.") {
			return pythonInterpreter{path: path, args: candidate.args}, nil
		}
		attempts = append(attempts, fmt.Sprintf("%s: %q", candidate.path, version))
	}

	detail := ""
	if len(attempts) > 0 {
		detail = " (" + strings.Join(attempts, "; ") + ")"
	}
	return pythonInterpreter{}, fmt.Errorf("không tìm thấy Python 3 trong PATH%s; cài Python 3.11+ tại https://www.python.org/downloads/ rồi chạy \"mt doctor\"", detail)
}

func createVenv(ctx context.Context, interpreter pythonInterpreter, envDir string) error {
	args := append(append([]string{}, interpreter.args...), "-m", "venv", envDir)
	cmd := exec.CommandContext(ctx, interpreter.path, args...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		if ctx.Err() != nil {
			return fmt.Errorf("tạo venv bị hủy: %w", ctx.Err())
		}
		return fmt.Errorf("tạo venv tại %s: %w%s", envDir, err, tailBlock(string(output)))
	}
	return nil
}

func installRequirements(ctx context.Context, venvPython, reqPath, appDir string) error {
	cmd := exec.CommandContext(ctx, venvPython,
		"-m", "pip", "install",
		"--disable-pip-version-check",
		"--no-input",
		"--progress-bar", "off",
		"--timeout", "30",
		"--retries", "2",
		"-r", reqPath,
	)
	cmd.Dir = appDir
	output, err := cmd.CombinedOutput()
	if err != nil {
		if ctx.Err() != nil {
			return fmt.Errorf("cài requirements bị hủy (mạng chậm?): %w", ctx.Err())
		}
		return fmt.Errorf("pip install thất bại: %w%s", err, tailBlock(string(output)))
	}
	return nil
}

func requirementsFile(app core.App) (string, string, error) {
	if app.Requirements == "" {
		return "", "", nil
	}
	if app.Dir == "" || !filepath.IsAbs(app.Dir) {
		return "", "", fmt.Errorf("thư mục app %q không phải đường dẫn tuyệt đối", app.Dir)
	}

	appDir := filepath.Clean(app.Dir)
	path := filepath.Join(appDir, filepath.FromSlash(app.Requirements))
	rel, err := filepath.Rel(appDir, path)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", "", fmt.Errorf("requirements %q nằm ngoài thư mục app", app.Requirements)
	}

	info, err := os.Stat(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return "", "", fmt.Errorf("không thấy requirements %q trong thư mục app", app.Requirements)
		}
		return "", "", fmt.Errorf("kiểm tra requirements %q: %w", app.Requirements, err)
	}
	if !info.Mode().IsRegular() {
		return "", "", fmt.Errorf("requirements %q phải là file thường", app.Requirements)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return "", "", fmt.Errorf("đọc requirements %q: %w", app.Requirements, err)
	}
	sum := sha256.Sum256(data)
	return path, hex.EncodeToString(sum[:]), nil
}

func firstNonEmptyLine(text string) string {
	for _, line := range strings.Split(text, "\n") {
		line = strings.TrimSpace(strings.TrimSuffix(line, "\r"))
		if line != "" {
			return line
		}
	}
	return ""
}

func tailBlock(text string) string {
	text = strings.TrimSpace(text)
	if text == "" {
		return ""
	}
	const limit = 1200
	if len(text) > limit {
		text = "..." + text[len(text)-limit:]
	}
	return "\n" + text
}
