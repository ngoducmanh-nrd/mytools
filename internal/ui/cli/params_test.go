package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"mytools/internal/core"
)

func TestParseToolArgsDefaultsAndFlags(t *testing.T) {
	home, err := os.UserHomeDir()
	if err != nil {
		t.Fatal(err)
	}
	tool := &core.Tool{
		ID: "dl",
		Params: []core.Param{
			{Name: "url", Type: "string", Required: true},
			{Name: "out", Type: "path", Default: "~/Downloads"},
			{Name: "audio_only", Type: "bool", Default: false},
			{Name: "quality", Type: "enum", Choices: []string{"best", "1080", "720"}, Default: "best"},
			{Name: "retries", Type: "int", Default: int64(3)},
		},
	}

	params, rest, err := parseToolArgs(tool, []string{
		"--url", "https://example.com/v",
		"--audio-only",
		"--quality", "720",
		"--retries", "5",
		"extra",
	})
	if err != nil {
		t.Fatalf("parseToolArgs: %v", err)
	}
	if params["url"] != "https://example.com/v" {
		t.Errorf("url = %q", params["url"])
	}
	if params["out"] != filepath.Join(home, "Downloads") {
		t.Errorf("out = %q, want default expanded", params["out"])
	}
	if params["audio_only"] != "true" {
		t.Errorf("audio_only = %q, want true", params["audio_only"])
	}
	if params["quality"] != "720" {
		t.Errorf("quality = %q, want 720", params["quality"])
	}
	if params["retries"] != "5" {
		t.Errorf("retries = %q, want 5", params["retries"])
	}
	if len(rest) != 1 || rest[0] != "extra" {
		t.Errorf("rest = %v, want [extra]", rest)
	}
}

func TestParseToolArgsPathFlagExpanded(t *testing.T) {
	home, err := os.UserHomeDir()
	if err != nil {
		t.Fatal(err)
	}
	tool := &core.Tool{
		ID:     "dl",
		Params: []core.Param{{Name: "out", Type: "path"}},
	}
	params, _, err := parseToolArgs(tool, []string{"--out", "~/Video tải về"})
	if err != nil {
		t.Fatalf("parseToolArgs: %v", err)
	}
	if want := filepath.Join(home, "Video tải về"); params["out"] != want {
		t.Errorf("out = %q, want %q", params["out"], want)
	}
}

func TestParseToolArgsErrors(t *testing.T) {
	base := &core.Tool{
		ID: "dl",
		Params: []core.Param{
			{Name: "url", Type: "string", Required: true},
			{Name: "quality", Type: "enum", Choices: []string{"best", "720"}, Default: "best"},
		},
	}
	tests := []struct {
		name    string
		args    []string
		message string
	}{
		{"missing required", nil, "thiếu tham số bắt buộc: --url"},
		{"empty required", []string{"--url", ""}, "thiếu tham số bắt buộc: --url"},
		{"bad enum", []string{"--url", "u", "--quality", "4k"}, `"4k" không hợp lệ (chọn: best, 720)`},
		{"unknown flag", []string{"--url", "u", "--wat"}, "unknown flag: --wat"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, _, err := parseToolArgs(base, test.args)
			if err == nil {
				t.Fatal("parseToolArgs không báo lỗi")
			}
			if !strings.Contains(err.Error(), test.message) {
				t.Fatalf("error %q không chứa %q", err, test.message)
			}
		})
	}
}

func TestParseToolArgsNoParamsPassThrough(t *testing.T) {
	tool := &core.Tool{ID: "demo"}
	params, rest, err := parseToolArgs(tool, []string{"--anything", "value"})
	if err != nil {
		t.Fatalf("parseToolArgs: %v", err)
	}
	if len(params) != 0 {
		t.Errorf("params = %v, want rỗng", params)
	}
	if len(rest) != 2 || rest[0] != "--anything" || rest[1] != "value" {
		t.Errorf("rest = %v", rest)
	}
}

func TestParseToolArgsBadDefaultType(t *testing.T) {
	tool := &core.Tool{
		ID:     "bad",
		Params: []core.Param{{Name: "count", Type: "int", Default: "ba"}},
	}
	_, _, err := parseToolArgs(tool, nil)
	if err == nil || !strings.Contains(err.Error(), "default phải là số nguyên") {
		t.Fatalf("error = %v", err)
	}
}
