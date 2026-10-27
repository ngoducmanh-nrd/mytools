package cli

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"mytools/internal/core"
)

func TestAppStatus(t *testing.T) {
	dir := t.TempDir()
	app := core.App{
		AppSpec: core.AppSpec{ID: "demo", Runtime: "binary", Entry: []string{"tool.exe"}},
		Dir:     dir,
	}
	if got := appStatus(app); got != "chưa cài" {
		t.Fatalf("status = %q", got)
	}

	app.Sha256 = "abc"
	app.DownloadedAt = time.Now()
	if got := appStatus(app); got != "chưa cài" {
		t.Fatalf("status khi thiếu file = %q", got)
	}

	if err := os.WriteFile(filepath.Join(dir, "tool.exe"), []byte("x"), 0o755); err != nil {
		t.Fatal(err)
	}
	if got := appStatus(app); got != "đã cài" {
		t.Fatalf("status = %q", got)
	}
}

func TestAppStatusPython(t *testing.T) {
	dir := t.TempDir()
	app := core.App{
		AppSpec: core.AppSpec{ID: "demo", Runtime: "python", Entry: []string{"python", "main.py"}},
		Dir:     dir,
	}
	if got := appStatus(app); got != "chưa cài" {
		t.Fatalf("status = %q", got)
	}

	venvPython, err := venvPythonForTest(dir)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(venvPython), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(venvPython, []byte("x"), 0o755); err != nil {
		t.Fatal(err)
	}
	if got := appStatus(app); got != "đã cài" {
		t.Fatalf("status = %q", got)
	}
}

func venvPythonForTest(appDir string) (string, error) {
	if runtime.GOOS == "windows" {
		return filepath.Join(appDir, "env", "Scripts", "python.exe"), nil
	}
	return filepath.Join(appDir, "env", "bin", "python"), nil
}
