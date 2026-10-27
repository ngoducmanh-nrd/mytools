package cli

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"mytools/internal/core"
)

func TestDirectBinaryURL(t *testing.T) {
	cases := []struct {
		arg  string
		want bool
	}{
		{"https://example.com/tools/My%20Tool.exe?download=1", true},
		{"https://example.com/file.zip", true},
		{"https://github.com/user/repo.git", false},
		{"https://github.com/user/repo", false},
		{"https://example.com/", false},
		{"ftp://example.com/tool.exe", false},
		{"./app", false},
		{"C:\\tool\\app.exe", false},
		{"", false},
	}
	for _, tc := range cases {
		if _, got := directBinaryURL(tc.arg); got != tc.want {
			t.Fatalf("directBinaryURL(%q) = %v, muốn %v", tc.arg, got, tc.want)
		}
	}
}

func TestBinarySpec(t *testing.T) {
	rawURL := "https://example.com/tools/My%20Tool.exe?download=1"
	spec, err := binarySpec(rawURL)
	if err != nil {
		t.Fatalf("binarySpec: %v", err)
	}
	if spec.ID != "my-tool" {
		t.Fatalf("ID = %q", spec.ID)
	}
	if spec.Runtime != "binary" || spec.Download != rawURL {
		t.Fatalf("spec = %+v", spec)
	}
	if len(spec.Entry) != 1 || spec.Entry[0] != "My Tool.exe" {
		t.Fatalf("Entry = %v", spec.Entry)
	}
}

func TestBinarySpecRejectsMissingName(t *testing.T) {
	if _, err := binarySpec("https://example.com/"); err == nil {
		t.Fatal("binarySpec phải lỗi khi URL không có tên file")
	}
}

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

func TestIsLocalToolDir(t *testing.T) {
	dir := t.TempDir()
	withAppTOML := filepath.Join(dir, "có app.toml")
	if err := os.MkdirAll(withAppTOML, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(withAppTOML, "app.toml"), []byte("schema_version = 1\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	empty := filepath.Join(dir, "trống")
	if err := os.MkdirAll(empty, 0o700); err != nil {
		t.Fatal(err)
	}

	cases := []struct {
		arg  string
		want bool
	}{
		{withAppTOML, true},
		{empty, false},
		{filepath.Join(dir, "không tồn tại"), false},
		{"https://example.com/repo", false},
		{"", false},
	}
	for _, tc := range cases {
		if got := isLocalToolDir(tc.arg); got != tc.want {
			t.Fatalf("isLocalToolDir(%q) = %v, muốn %v", tc.arg, got, tc.want)
		}
	}
}
