package runner

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
	"sync"

	"mytools/internal/core"
)

var progressPattern = regexp.MustCompile(`^\[download\]\s+([0-9]{1,3}(?:\.[0-9]+)?)%`)

// Run chạy lệnh, chuyển từng dòng stdout/stderr thành Event cho tới khi tiến trình thoát.
func Run(ctx context.Context, cmd *exec.Cmd, runID string, events chan<- core.Event) error {
	if cmd == nil {
		return errors.New("thiếu lệnh để chạy")
	}
	if ctx == nil {
		ctx = context.Background()
	}

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return fmt.Errorf("mở stdout của %s: %w", cmd.Path, err)
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return fmt.Errorf("mở stderr của %s: %w", cmd.Path, err)
	}
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("chạy %s: %w", cmd.Path, err)
	}

	var (
		wg      sync.WaitGroup
		mu      sync.Mutex
		readErr error
	)
	record := func(err error) {
		if err == nil {
			return
		}
		mu.Lock()
		if readErr == nil {
			readErr = err
		}
		mu.Unlock()
	}

	wg.Add(2)
	go func() {
		defer wg.Done()
		record(streamLines(ctx, stdout, runID, events, false))
	}()
	go func() {
		defer wg.Done()
		record(streamLines(ctx, stderr, runID, events, true))
	}()
	wg.Wait()

	waitErr := cmd.Wait()
	if ctx.Err() != nil {
		return ctx.Err()
	}
	if waitErr != nil {
		return waitErr
	}
	return readErr
}

func streamLines(ctx context.Context, r io.Reader, runID string, events chan<- core.Event, isStderr bool) error {
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 64*1024), 1024*1024)

	for scanner.Scan() {
		line := strings.TrimRight(scanner.Text(), "\r")
		if strings.TrimSpace(line) == "" {
			continue
		}

		event := core.Event{RunID: runID, Kind: "log", Text: line}
		switch {
		case isStderr:
			event.Kind = "error"
		default:
			if pct, ok := parseProgress(line); ok {
				event.Kind = "progress"
				event.Pct = pct
			}
		}

		if !sendEvent(ctx, events, event) {
			return nil
		}
	}
	return scanner.Err()
}

func sendEvent(ctx context.Context, events chan<- core.Event, event core.Event) bool {
	if events == nil {
		return true
	}
	select {
	case events <- event:
		return true
	case <-ctx.Done():
		return false
	}
}

func parseProgress(line string) (float64, bool) {
	match := progressPattern.FindStringSubmatch(line)
	if match == nil {
		return 0, false
	}
	pct, err := strconv.ParseFloat(match[1], 64)
	if err != nil {
		return 0, false
	}
	return pct, true
}
