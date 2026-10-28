package runner

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"mytools/internal/core"
)

const helpOutput = `SỬ DỤNG: helper [tùy chọn]

Công cụ mẫu để kiểm tra trích xuất --help.

  --force           Ép chạy lại dù đã có kết quả
  --force           (dòng lặp lại để kiểm tra trùng)
  --output <file>   Ghi kết quả ra file
  --output-dir <thư mục>  Thư mục chứa kết quả
  --no-mtime        Không ghi thời gian sửa đổi
`

// TestMain biến test binary thành helper cho HelpFlags: khi GO_WANT_HELP_HELPER
// được đặt, tiến trình con in help giả lập theo cờ nhận được thay vì chạy test.
func TestMain(m *testing.M) {
	switch os.Getenv("GO_WANT_HELP_HELPER") {
	case "help":
		for _, arg := range os.Args[1:] {
			if arg == "--help" || arg == "-h" {
				fmt.Print(helpOutput)
				os.Exit(0)
			}
		}
		os.Exit(2)
	case "fallback":
		for _, arg := range os.Args[1:] {
			if arg == "-h" {
				fmt.Print(helpOutput)
				os.Exit(0)
			}
		}
		os.Exit(2)
	case "fail":
		os.Exit(2)
	}
	os.Exit(m.Run())
}

func helperApp(t *testing.T) core.App {
	t.Helper()
	dir := t.TempDir()
	entry := filepath.Base(os.Args[0])
	data, err := os.ReadFile(os.Args[0])
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, entry), data, 0o755); err != nil {
		t.Fatal(err)
	}
	return core.App{AppSpec: core.AppSpec{ID: "helper", Runtime: "binary", Entry: []string{entry}}, Dir: dir}
}

func TestHelpFlagsExtractsFlagsAndWritesFile(t *testing.T) {
	t.Setenv("GO_WANT_HELP_HELPER", "help")
	app := helperApp(t)

	summary, err := HelpFlags(context.Background(), app)
	if err != nil {
		t.Fatalf("HelpFlags: %v", err)
	}
	for _, want := range []string{"--force", "--output", "--output-dir", "--no-mtime"} {
		if !strings.Contains(summary, want) {
			t.Errorf("summary %q thiếu %q", summary, want)
		}
	}
	if strings.Contains(summary, "và ") {
		t.Errorf("summary không nên cắt cờ: %q", summary)
	}

	data, err := os.ReadFile(filepath.Join(app.Dir, "help.txt"))
	if err != nil {
		t.Fatalf("help.txt: %v", err)
	}
	if !strings.Contains(string(data), "SỬ DỤNG: helper") {
		t.Errorf("help.txt = %q", data)
	}
	matches, err := filepath.Glob(filepath.Join(app.Dir, "*.tmp"))
	if err != nil {
		t.Fatal(err)
	}
	if len(matches) != 0 {
		t.Errorf("file tạm sót lại: %v", matches)
	}
}

func TestHelpFlagsFallbackToShortH(t *testing.T) {
	t.Setenv("GO_WANT_HELP_HELPER", "fallback")
	app := helperApp(t)

	summary, err := HelpFlags(context.Background(), app)
	if err != nil {
		t.Fatalf("HelpFlags: %v", err)
	}
	if !strings.Contains(summary, "--force") {
		t.Errorf("summary = %q, want chứa --force", summary)
	}
}

func TestHelpFlagsSkipsGUI(t *testing.T) {
	app := core.App{
		AppSpec: core.AppSpec{ID: "gui", Runtime: "binary", GUI: true, Entry: []string{"x"}},
		Dir:     t.TempDir(),
	}
	summary, err := HelpFlags(context.Background(), app)
	if err != nil || summary != "" {
		t.Fatalf("GUI phải bỏ qua: summary = %q, err = %v", summary, err)
	}
	if _, err := os.Stat(filepath.Join(app.Dir, "help.txt")); !os.IsNotExist(err) {
		t.Errorf("GUI không được ghi help.txt: %v", err)
	}
}

func TestHelpFlagsFailureIsNonFatalError(t *testing.T) {
	t.Setenv("GO_WANT_HELP_HELPER", "fail")
	app := helperApp(t)

	if _, err := HelpFlags(context.Background(), app); err == nil || !strings.Contains(err.Error(), "thoát với mã 2") {
		t.Fatalf("HelpFlags error = %v", err)
	}
	if _, err := os.Stat(filepath.Join(app.Dir, "help.txt")); !os.IsNotExist(err) {
		t.Errorf("help.txt không được ghi khi help thất bại: %v", err)
	}
}

func TestExtractFlagsDedupesInOrder(t *testing.T) {
	got := extractFlags([]byte("--a --b --a --ab --c"))
	want := []string{"--a", "--b", "--ab", "--c"}
	if len(got) != len(want) {
		t.Fatalf("extractFlags = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("extractFlags = %v, want %v", got, want)
		}
	}
	if got := extractFlags([]byte("không có cờ nào, chỉ có --  và --- và https://x")); len(got) != 0 {
		t.Errorf("extractFlags = %v, want rỗng", got)
	}
}

func TestExtractFlagsSkipsLineWrapped(t *testing.T) {
	// Cờ bị cắt dòng kiểu "--no-js-\n    runtimes" không phải cờ thật.
	got := extractFlags([]byte("--help --no-js-\n    runtimes --output -"))
	want := []string{"--help", "--output"}
	if len(got) != len(want) {
		t.Fatalf("extractFlags = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("extractFlags = %v, want %v", got, want)
		}
	}
}

func TestFormatFlagsSummaryCaps(t *testing.T) {
	flags := make([]string, 35)
	for i := range flags {
		flags[i] = fmt.Sprintf("--flag-%d", i)
	}
	summary := formatFlagsSummary(flags, `C:\apps\x\help.txt`)
	if !strings.Contains(summary, "và 5 cờ khác") {
		t.Errorf("summary = %q", summary)
	}
	if !strings.Contains(summary, "--flag-29") || strings.Contains(summary, "--flag-30") {
		t.Errorf("summary cắt sai chỗ: %q", summary)
	}
	if !strings.Contains(summary, `C:\apps\x\help.txt`) {
		t.Errorf("summary thiếu đường dẫn help.txt: %q", summary)
	}
}
