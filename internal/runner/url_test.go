package runner

import (
	"bytes"
	"context"
	"runtime"
	"strings"
	"testing"
)

func TestOpenDryRun(t *testing.T) {
	t.Setenv("MT_DRY_RUN", "1")

	var buffered bytes.Buffer
	previous := stdout
	stdout = &buffered
	t.Cleanup(func() {
		stdout = previous
	})

	url := "https://example.com/tìm-kiếm?q=phim 2026"
	if err := Open(context.Background(), url); err != nil {
		t.Fatal(err)
	}
	if got := strings.TrimSpace(buffered.String()); got != url {
		t.Fatalf("dry-run output = %q, want %q", got, url)
	}
}

func TestBrowserCommand(t *testing.T) {
	if runtime.GOOS != "windows" && runtime.GOOS != "linux" {
		t.Skip("chỉ kiểm tra trên Windows và Linux")
	}

	url := "https://example.com/"
	cmd, err := browserCommand(context.Background(), url)
	if err != nil {
		t.Fatal(err)
	}
	if len(cmd.Args) == 0 {
		t.Fatal("browser command has no arguments")
	}
	if got := cmd.Args[len(cmd.Args)-1]; got != url {
		t.Fatalf("last argument = %q, want %q", got, url)
	}

	switch runtime.GOOS {
	case "windows":
		if len(cmd.Args) != 3 || cmd.Args[0] != "rundll32" || cmd.Args[1] != "url.dll,FileProtocolHandler" {
			t.Fatalf("unexpected Windows command: %v", cmd.Args)
		}
	case "linux":
		if len(cmd.Args) != 2 || cmd.Args[0] != "xdg-open" {
			t.Fatalf("unexpected Linux command: %v", cmd.Args)
		}
	}
}
