package cli

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/spf13/cobra"

	"mytools/internal/core"
)

type syncBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *syncBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *syncBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}

func TestLatestLogPath(t *testing.T) {
	dir := t.TempDir()
	older := filepath.Join(dir, "2026-10-26.log")
	newer := filepath.Join(dir, "2026-10-27.log")
	if err := os.WriteFile(older, []byte("old"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(newer, []byte("new"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "ignore.txt"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}

	oldTime := time.Now().Add(-time.Hour)
	newTime := time.Now()
	if err := os.Chtimes(older, oldTime, oldTime); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(newer, newTime, newTime); err != nil {
		t.Fatal(err)
	}

	got, err := latestLogPath(dir)
	if err != nil {
		t.Fatalf("latestLogPath: %v", err)
	}
	if got != newer {
		t.Fatalf("latestLogPath = %q, want %q", got, newer)
	}
}

func TestLatestLogPathMissing(t *testing.T) {
	if _, err := latestLogPath(filepath.Join(t.TempDir(), "missing")); err == nil {
		t.Fatal("latestLogPath không báo lỗi khi thư mục không tồn tại")
	}
}

func TestFormatUptime(t *testing.T) {
	tests := []struct {
		in   time.Duration
		want string
	}{
		{0, "0s"},
		{5 * time.Second, "5s"},
		{65 * time.Second, "1m05s"},
		{2*time.Hour + 3*time.Minute + 4*time.Second, "2h03m"},
		{26*time.Hour + 5*time.Minute, "1d02h"},
	}
	for _, test := range tests {
		if got := formatUptime(test.in); got != test.want {
			t.Errorf("formatUptime(%s) = %q, want %q", test.in, got, test.want)
		}
	}
}

func TestPrintLogFollowStopsOnCancel(t *testing.T) {
	path := filepath.Join(t.TempDir(), "test.log")
	if err := os.WriteFile(path, []byte("first\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	out := &syncBuffer{}
	cmd := &cobra.Command{}
	cmd.SetOut(out)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	cmd.SetContext(ctx)

	errCh := make(chan error, 1)
	go func() {
		errCh <- printLog(cmd, path, true)
	}()

	waitFor(t, out, "first\n")

	file, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := file.WriteString("second\n"); err != nil {
		file.Close()
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
	waitFor(t, out, "second\n")

	cancel()
	select {
	case err := <-errCh:
		if err != nil {
			t.Fatalf("printLog trả lỗi khi hủy: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("printLog không thoát sau khi hủy context")
	}
}

func waitFor(t *testing.T, out *syncBuffer, want string) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if strings.Contains(out.String(), want) {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("log không chứa %q: %q", want, out.String())
}

func TestIsDetached(t *testing.T) {
	gui := &core.App{AppSpec: core.AppSpec{GUI: true}}
	plain := &core.App{}
	tests := []struct {
		lifecycle string
		app       *core.App
		want      bool
	}{
		{"window", plain, true},
		{"service", plain, true},
		{"oneshot", gui, false},
		{"", gui, true},
		{"", plain, false},
	}
	for _, test := range tests {
		if got := isDetached(test.lifecycle, test.app); got != test.want {
			t.Errorf("isDetached(%q, gui=%v) = %v, want %v", test.lifecycle, test.app.GUI, got, test.want)
		}
	}
}
