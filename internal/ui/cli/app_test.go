package cli

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"mytools/internal/core"
)

func TestSpecFromArgumentBuiltin(t *testing.T) {
	spec, err := specFromArgument("yt-dlp")
	if err != nil {
		t.Fatalf("specFromArgument: %v", err)
	}
	if spec.ID != "yt-dlp" || spec.Runtime != "binary" || spec.Download == "" || len(spec.Entry) != 1 {
		t.Fatalf("spec = %+v", spec)
	}
}

func TestSpecFromArgumentDirectURL(t *testing.T) {
	rawURL := "https://example.com/tools/My%20Tool.exe?download=1"
	spec, err := specFromArgument(rawURL)
	if err != nil {
		t.Fatalf("specFromArgument: %v", err)
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

func TestSpecFromArgumentRejectsUnsupported(t *testing.T) {
	cases := []string{
		"",
		"./app",
		"C:\\tool\\app.exe",
		"https://github.com/user/repo",
		"https://example.com/",
		"ftp://example.com/tool.exe",
	}
	for _, arg := range cases {
		if _, err := specFromArgument(arg); err == nil {
			t.Fatalf("specFromArgument(%q) phải lỗi", arg)
		}
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
