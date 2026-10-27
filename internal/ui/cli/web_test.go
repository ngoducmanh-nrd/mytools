package cli

import (
	"bytes"
	"strings"
	"testing"

	"github.com/spf13/cobra"

	"mytools/internal/config"
)

func TestWebPort(t *testing.T) {
	tests := []struct {
		name    string
		value   any
		want    int
		wantErr string
	}{
		{name: "mặc định khi thiếu", value: nil, want: 7878},
		{name: "số nguyên", value: int64(9000), want: 9000},
		{name: "auto", value: "auto", want: 0},
		{name: "chuỗi số", value: "8123", want: 8123},
		{name: "chuỗi sai", value: "abc", wantErr: `web_port "abc" không hợp lệ`},
		{name: "số quá lớn", value: int64(70000), wantErr: "ngoài khoảng 1-65535"},
		{name: "số âm", value: int64(-1), wantErr: "ngoài khoảng 1-65535"},
		{name: "số 0", value: int64(0), wantErr: "ngoài khoảng 1-65535"},
		{name: "kiểu lạ", value: 3.5, wantErr: "web_port không hợp lệ"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			port, err := webPort(test.value)
			if test.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), test.wantErr) {
					t.Fatalf("webPort(%v) = (%d, %v), want error chứa %q", test.value, port, err, test.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("webPort(%v): %v", test.value, err)
			}
			if port != test.want {
				t.Errorf("webPort(%v) = %d, want %d", test.value, port, test.want)
			}
		})
	}
}

func TestDefaultUI(t *testing.T) {
	cmd := &cobra.Command{Use: "mt", RunE: func(cmd *cobra.Command, args []string) error { return nil }}
	var out bytes.Buffer
	cmd.SetOut(&out)

	t.Run("cli hiện trợ giúp", func(t *testing.T) {
		out.Reset()
		cfg := &config.Config{DefaultUI: "cli"}
		if err := defaultUI(cmd, cfg); err != nil {
			t.Fatalf("defaultUI: %v", err)
		}
		if !strings.Contains(out.String(), "Usage:") {
			t.Errorf("output = %q, thiếu Usage", out.String())
		}
	})

	t.Run("trống cũng hiện trợ giúp", func(t *testing.T) {
		out.Reset()
		cfg := &config.Config{DefaultUI: ""}
		if err := defaultUI(cmd, cfg); err != nil {
			t.Fatalf("defaultUI: %v", err)
		}
		if !strings.Contains(out.String(), "Usage:") {
			t.Errorf("output = %q, thiếu Usage", out.String())
		}
	})

	t.Run("web chạy server và mở trình duyệt", func(t *testing.T) {
		original := startWeb
		defer func() { startWeb = original }()

		var gotPort int
		var gotOpen bool
		startWeb = func(cmd *cobra.Command, port int, open bool) error {
			gotPort, gotOpen = port, open
			return nil
		}

		cfg := &config.Config{DefaultUI: "web", WebPort: int64(9000)}
		if err := defaultUI(cmd, cfg); err != nil {
			t.Fatalf("defaultUI: %v", err)
		}
		if gotPort != 9000 || !gotOpen {
			t.Errorf("startWeb(%d, %v), want (9000, true)", gotPort, gotOpen)
		}
	})

	t.Run("web port auto", func(t *testing.T) {
		original := startWeb
		defer func() { startWeb = original }()

		var gotPort int
		startWeb = func(cmd *cobra.Command, port int, open bool) error {
			gotPort = port
			return nil
		}

		cfg := &config.Config{DefaultUI: "web", WebPort: "auto"}
		if err := defaultUI(cmd, cfg); err != nil {
			t.Fatalf("defaultUI: %v", err)
		}
		if gotPort != 0 {
			t.Errorf("port = %d, want 0 (auto)", gotPort)
		}
	})

	t.Run("web port sai báo lỗi", func(t *testing.T) {
		cfg := &config.Config{DefaultUI: "web", WebPort: int64(70000)}
		if err := defaultUI(cmd, cfg); err == nil || !strings.Contains(err.Error(), "ngoài khoảng") {
			t.Errorf("defaultUI = %v, want lỗi port", err)
		}
	})

	t.Run("default_ui lạ báo lỗi", func(t *testing.T) {
		cfg := &config.Config{DefaultUI: "gui"}
		if err := defaultUI(cmd, cfg); err == nil || !strings.Contains(err.Error(), `"gui" không hợp lệ`) {
			t.Errorf("defaultUI = %v, want lỗi default_ui", err)
		}
	})
}
