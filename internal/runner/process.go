package runner

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"mytools/internal/core"
)

var progressPattern = regexp.MustCompile(`^\[download\]\s+([0-9]{1,3}(?:\.[0-9]+)?)%`)

// askSettleDelay là khoảng lặng để coi một dòng stdout chưa xuống dòng là lời nhắc (ask).
const askSettleDelay = 150 * time.Millisecond

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
	type readResult struct {
		data []byte
		err  error
	}

	reader := bufio.NewReader(r)
	readCh := make(chan readResult, 1)
	go func() {
		buf := make([]byte, 32*1024)
		for {
			n, err := reader.Read(buf)
			if n > 0 {
				select {
				case readCh <- readResult{data: append([]byte(nil), buf[:n]...)}:
				case <-ctx.Done():
					return
				}
				if err != nil {
					select {
					case readCh <- readResult{err: err}:
					case <-ctx.Done():
					}
					return
				}
				continue
			}
			select {
			case readCh <- readResult{err: err}:
			case <-ctx.Done():
			}
			return
		}
	}()

	var pending []byte
	var timer *time.Timer
	var timerC <-chan time.Time
	stopTimer := func() {
		if timer == nil {
			return
		}
		if !timer.Stop() {
			select {
			case <-timer.C:
			default:
			}
		}
		timer = nil
		timerC = nil
	}

	emit := func(text string, kind string, pct float64) bool {
		if strings.TrimSpace(text) == "" {
			return true
		}
		return sendEvent(ctx, events, core.Event{RunID: runID, Kind: kind, Text: text, Pct: pct})
	}

	for {
		select {
		case result := <-readCh:
			stopTimer()
			if result.err != nil {
				kind := "log"
				if isStderr {
					kind = "error"
				}
				if !emit(strings.TrimRight(string(pending), "\r"), kind, 0) {
					return nil
				}
				if errors.Is(result.err, io.EOF) {
					return nil
				}
				return result.err
			}

			pending = append(pending, result.data...)
			for {
				idx := bytes.IndexByte(pending, '\n')
				if idx < 0 {
					break
				}
				line := strings.TrimRight(string(pending[:idx]), "\r")
				pending = pending[idx+1:]
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
			if len(pending) > 0 {
				timer = time.NewTimer(askSettleDelay)
				timerC = timer.C
			}

		case <-timerC:
			stopTimer()
			partial := strings.TrimRight(string(pending), "\r")
			pending = nil
			kind := "ask"
			pct := 0.0
			if isStderr {
				kind = "error"
			} else if progressPct, ok := parseProgress(partial); ok {
				kind = "progress"
				pct = progressPct
			}
			if !emit(partial, kind, pct) {
				return nil
			}

		case <-ctx.Done():
			stopTimer()
			return nil
		}
	}
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
