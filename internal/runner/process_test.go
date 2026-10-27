package runner

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"

	"mytools/internal/core"
)

func TestParseProgress(t *testing.T) {
	tests := []struct {
		line string
		pct  float64
		ok   bool
	}{
		{"[download]   0.0% of 10MiB at 1MiB/s ETA 00:10", 0, true},
		{"[download]  50.0% of 10MiB at 1MiB/s ETA 00:05", 50, true},
		{"[download] 100% of 10MiB in 00:00:05", 100, true},
		{"[download]  99.9% of ~ 1.5MiB at Unknown B/s ETA Unknown", 99.9, true},
		{"[download] Destination: video.mp4", 0, false},
		{"[youtube] Extracting URL", 0, false},
		{" [download]  50.0%", 0, false},
	}
	for _, test := range tests {
		pct, ok := parseProgress(test.line)
		if ok != test.ok || (ok && pct != test.pct) {
			t.Errorf("parseProgress(%q) = (%v, %v), want (%v, %v)", test.line, pct, ok, test.pct, test.ok)
		}
	}
}

func TestRunStreamsEvents(t *testing.T) {
	cmd := helperCommand(t, "1")
	events := make(chan core.Event, 16)
	errCh := make(chan error, 1)
	go func() {
		errCh <- Run(context.Background(), cmd, "run-1", events)
		close(events)
	}()

	var received []core.Event
	for event := range events {
		received = append(received, event)
	}
	if err := <-errCh; err != nil {
		t.Fatalf("Run: %v", err)
	}

	var logs, errorsSeen []string
	var pcts []float64
	for _, event := range received {
		if event.RunID != "run-1" {
			t.Errorf("RunID = %q, want run-1", event.RunID)
		}
		switch event.Kind {
		case "log":
			logs = append(logs, event.Text)
		case "error":
			errorsSeen = append(errorsSeen, event.Text)
		case "progress":
			pcts = append(pcts, event.Pct)
		default:
			t.Errorf("Kind lạ: %q", event.Kind)
		}
	}

	if len(pcts) != 3 || pcts[0] != 0 || pcts[1] != 50 || pcts[2] != 100 {
		t.Errorf("progress = %v, want [0 50 100]", pcts)
	}
	if !containsText(logs, "[youtube] Extracting URL") {
		t.Errorf("thiếu log stdout: %v", logs)
	}
	if !containsText(errorsSeen, "WARNING: thử stderr") {
		t.Errorf("thiếu error event: %v", errorsSeen)
	}
}

func TestRunExitError(t *testing.T) {
	cmd := helperCommand(t, "7")
	events := make(chan core.Event, 8)

	err := Run(context.Background(), cmd, "run-2", events)
	close(events)
	var exitErr *exec.ExitError
	if !errors.As(err, &exitErr) {
		t.Fatalf("Run = %v, want *exec.ExitError", err)
	}
	if exitErr.ExitCode() != 7 {
		t.Fatalf("ExitCode = %d, want 7", exitErr.ExitCode())
	}
}

func TestRunEmitsAskForPromptWithoutNewline(t *testing.T) {
	cmd := helperCommand(t, "ask")
	cmd.Stdin = strings.NewReader("Minh\n")
	events := make(chan core.Event, 16)
	errCh := make(chan error, 1)
	go func() {
		errCh <- Run(context.Background(), cmd, "run-ask", events)
		close(events)
	}()

	var received []core.Event
	for event := range events {
		received = append(received, event)
	}
	if err := <-errCh; err != nil {
		t.Fatalf("Run: %v", err)
	}

	askIndex := -1
	greetingIndex := -1
	var errorTexts []string
	for i, event := range received {
		switch event.Kind {
		case "ask":
			if askIndex >= 0 {
				t.Fatalf("nhiều hơn một event ask: %+v", received)
			}
			askIndex = i
			if event.Text != "Nhập tên của bạn: " {
				t.Errorf("ask text = %q", event.Text)
			}
		case "log":
			if strings.Contains(event.Text, "Xin chào, Minh!") && greetingIndex < 0 {
				greetingIndex = i
			}
		case "error":
			errorTexts = append(errorTexts, event.Text)
		}
	}
	if askIndex < 0 {
		t.Fatalf("thiếu event ask: %+v", received)
	}
	if greetingIndex < 0 {
		t.Fatalf("thiếu lời chào: %+v", received)
	}
	if askIndex >= greetingIndex {
		t.Errorf("ask phải đến trước lời chào: ask=%d greeting=%d", askIndex, greetingIndex)
	}
	if !containsText(errorTexts, "lỗi cụt dòng") {
		t.Errorf("thiếu error từ dòng stderr cụt: %v", errorTexts)
	}
}

func TestRunProgressPartialLine(t *testing.T) {
	cmd := helperCommand(t, "prog-cr")
	events := make(chan core.Event, 8)
	errCh := make(chan error, 1)
	go func() {
		errCh <- Run(context.Background(), cmd, "run-prog", events)
		close(events)
	}()

	var pcts []float64
	for event := range events {
		if event.Kind == "progress" {
			pcts = append(pcts, event.Pct)
		} else if event.Kind == "ask" {
			t.Errorf("dòng tiến trình cụt không được thành ask: %+v", event)
		}
	}
	if err := <-errCh; err != nil {
		t.Fatalf("Run: %v", err)
	}
	if len(pcts) != 2 || pcts[0] != 10 || pcts[1] != 100 {
		t.Errorf("progress = %v, want [10 100]", pcts)
	}
}

func helperCommand(t *testing.T, mode string) *exec.Cmd {
	t.Helper()
	cmd := exec.Command(os.Args[0], "-test.run=^TestProcessHelper$")
	cmd.Env = append(os.Environ(), "GO_WANT_PROCESS_HELPER="+mode)
	return cmd
}

func TestProcessHelper(t *testing.T) {
	mode := os.Getenv("GO_WANT_PROCESS_HELPER")
	if mode == "" {
		return
	}
	if mode == "7" {
		fmt.Fprintln(os.Stderr, "lỗi giả lập")
		os.Exit(7)
	}
	if mode == "ask" {
		fmt.Print("Nhập tên của bạn: ")
		time.Sleep(300 * time.Millisecond)
		var name string
		fmt.Fscanln(os.Stdin, &name)
		fmt.Println("Xin chào, " + name + "!")
		fmt.Fprint(os.Stderr, "lỗi cụt dòng")
		os.Exit(0)
	}
	if mode == "prog-cr" {
		fmt.Print("[download]  10.0% of 1MiB\r")
		time.Sleep(300 * time.Millisecond)
		fmt.Println("[download] 100% of 1MiB")
		os.Exit(0)
	}
	fmt.Println("[download]   0.0% of 10MiB at 1MiB/s ETA 00:10")
	fmt.Println("[youtube] Extracting URL")
	fmt.Println("[download]  50.0% of 10MiB at 1MiB/s ETA 00:05")
	fmt.Fprintln(os.Stderr, "WARNING: thử stderr")
	fmt.Println("[download] 100% of 10MiB in 00:00:05")
	os.Exit(0)
}

func containsText(values []string, want string) bool {
	for _, value := range values {
		if strings.Contains(value, want) {
			return true
		}
	}
	return false
}
