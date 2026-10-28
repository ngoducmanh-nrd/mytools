package runner

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"

	"mytools/internal/core"
)

func TestInstallPythonRejectsInvalidApp(t *testing.T) {
	cases := []core.App{
		{AppSpec: core.AppSpec{ID: "demo", Runtime: "binary"}, Dir: t.TempDir()},
		{AppSpec: core.AppSpec{ID: "demo", Runtime: "python", Entry: []string{"main.py"}}, Dir: t.TempDir()},
		{AppSpec: core.AppSpec{ID: "demo", Runtime: "python", Entry: []string{"python", "main.py"}}, Dir: "tuong-doi"},
	}
	for _, app := range cases {
		if err := InstallPython(context.Background(), app, nil); err == nil {
			t.Fatalf("InstallPython(%+v) phải lỗi", app.AppSpec)
		}
	}
}

func TestRequirementsFile(t *testing.T) {
	appDir := t.TempDir()
	content := []byte("requests==2.32.3\n")
	if err := os.WriteFile(filepath.Join(appDir, "requirements.txt"), content, 0o644); err != nil {
		t.Fatal(err)
	}

	app := core.App{
		AppSpec: core.AppSpec{ID: "demo", Runtime: "python", Requirements: "requirements.txt"},
		Dir:     appDir,
	}
	path, sum, err := requirementsFile(app)
	if err != nil {
		t.Fatalf("requirementsFile: %v", err)
	}
	if path != filepath.Join(appDir, "requirements.txt") {
		t.Fatalf("path = %q", path)
	}
	want := sha256.Sum256(content)
	if sum != hex.EncodeToString(want[:]) {
		t.Fatalf("hash = %q", sum)
	}

	app.Requirements = ""
	path, sum, err = requirementsFile(app)
	if err != nil || path != "" || sum != "" {
		t.Fatalf("requirements rỗng = %q %q %v", path, sum, err)
	}
}

func TestRequirementsFileRejectsOutsideAndMissing(t *testing.T) {
	appDir := t.TempDir()
	outside := filepath.Join(t.TempDir(), "requirements.txt")
	if err := os.WriteFile(outside, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}

	bad := []string{
		"../requirements.txt",
		"..\\requirements.txt",
		outside,
		"missing.txt",
	}
	for _, requirements := range bad {
		app := core.App{
			AppSpec: core.AppSpec{ID: "demo", Runtime: "python", Requirements: requirements},
			Dir:     appDir,
		}
		if _, _, err := requirementsFile(app); err == nil {
			t.Fatalf("requirementsFile(%q) phải lỗi", requirements)
		}
	}
}

func TestPythonCommandRequiresVenv(t *testing.T) {
	app := core.App{
		AppSpec: core.AppSpec{ID: "demo", Runtime: "python", Entry: []string{"python", "main.py"}},
		Dir:     t.TempDir(),
	}
	_, err := PythonCommand(context.Background(), app, nil)
	if err == nil || !strings.Contains(err.Error(), "chưa có môi trường Python") {
		t.Fatalf("lỗi không đúng: %v", err)
	}
	if PythonReady(app) {
		t.Fatal("PythonReady phải false khi thiếu venv")
	}

	venvPython, err := venvPythonPath(app.Dir)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(venvPython), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(venvPython, []byte("x"), 0o755); err != nil {
		t.Fatal(err)
	}
	if !PythonReady(app) {
		t.Fatal("PythonReady phải true khi có venv python")
	}
	cmd, err := PythonCommand(context.Background(), app, []string{"--flag"})
	if err != nil {
		t.Fatalf("PythonCommand: %v", err)
	}
	if cmd.Dir != app.Dir {
		t.Fatalf("cmd.Dir = %q", cmd.Dir)
	}
	if len(cmd.Args) != 3 || cmd.Args[0] != venvPython || cmd.Args[1] != "main.py" || cmd.Args[2] != "--flag" {
		t.Fatalf("cmd.Args = %v", cmd.Args)
	}
	if !slices.Contains(cmd.Env, "PYTHONIOENCODING=utf-8") {
		t.Fatalf("cmd.Env thiếu PYTHONIOENCODING=utf-8: %v", cmd.Env)
	}
}

func TestPythonCommandIsolatesPipCache(t *testing.T) {
	cacheRoot := t.TempDir()
	app := core.App{
		AppSpec:  core.AppSpec{ID: "demo", Runtime: "python", Entry: []string{"python", "main.py"}},
		Dir:      t.TempDir(),
		CacheDir: cacheRoot,
	}
	venvPython, err := venvPythonPath(app.Dir)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(venvPython), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(venvPython, []byte("x"), 0o755); err != nil {
		t.Fatal(err)
	}

	cmd, err := PythonCommand(context.Background(), app, nil)
	if err != nil {
		t.Fatalf("PythonCommand: %v", err)
	}
	want := "PIP_CACHE_DIR=" + filepath.Join(cacheRoot, "pip")
	if !slices.Contains(cmd.Env, want) {
		t.Fatalf("cmd.Env thiếu %q: %v", want, cmd.Env)
	}
	if _, err := os.Stat(filepath.Join(cacheRoot, "pip")); err != nil {
		t.Fatalf("thư mục cache pip không được tạo: %v", err)
	}
}

func TestVenvPythonPath(t *testing.T) {
	path, err := venvPythonPath(filepath.Join(t.TempDir(), "app"))
	if err != nil {
		t.Fatal(err)
	}
	if runtime.GOOS == "windows" {
		if filepath.Base(filepath.Dir(path)) != "Scripts" || filepath.Base(path) != "python.exe" {
			t.Fatalf("path = %q", path)
		}
	} else if filepath.Base(filepath.Dir(path)) != "bin" || filepath.Base(path) != "python" {
		t.Fatalf("path = %q", path)
	}
	if _, err := venvPythonPath("tuong-doi"); err == nil {
		t.Fatal("venvPythonPath phải từ chối đường dẫn tương đối")
	}
}

func TestFindPythonMissing(t *testing.T) {
	t.Setenv("PATH", t.TempDir())
	_, err := findPython(context.Background())
	if err == nil || !strings.Contains(err.Error(), "không tìm thấy Python 3") {
		t.Fatalf("lỗi không đúng: %v", err)
	}
}

func TestBinaryCommandRequiresInstalledFile(t *testing.T) {
	app := core.App{
		AppSpec: core.AppSpec{ID: "demo", Runtime: "binary", Entry: []string{"tool.exe"}},
		Dir:     t.TempDir(),
	}
	if _, err := BinaryCommand(context.Background(), app, nil); err == nil {
		t.Fatal("BinaryCommand phải lỗi khi thiếu file")
	}

	dest := filepath.Join(app.Dir, "tool.exe")
	if err := os.WriteFile(dest, []byte("x"), 0o755); err != nil {
		t.Fatal(err)
	}
	cmd, err := BinaryCommand(context.Background(), app, []string{"a"})
	if err != nil {
		t.Fatalf("BinaryCommand: %v", err)
	}
	if len(cmd.Args) != 2 || cmd.Args[0] != dest || cmd.Args[1] != "a" {
		t.Fatalf("cmd.Args = %v", cmd.Args)
	}
}
