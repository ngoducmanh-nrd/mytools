package appmgr

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

const runDirName = "run"

var ErrNotRunning = errors.New("không có tiến trình đang chạy")

type RunState struct {
	ID        string `json:"id"`
	PID       int    `json:"pid"`
	StartTime string `json:"start_time"`
	Cmd       string `json:"cmd"`
}

func (m *Manager) SaveRunState(state RunState) error {
	if err := validateRunState(state); err != nil {
		return err
	}

	dir, err := m.runDir()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("tạo thư mục run %s: %w", dir, err)
	}

	path, err := m.runStatePath(state.ID)
	if err != nil {
		return err
	}

	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return fmt.Errorf("mã hóa trạng thái chạy của %q: %w", state.ID, err)
	}
	data = append(data, '\n')

	if err := writeFileAtomic(path, data, 0o600); err != nil {
		return fmt.Errorf("ghi %s: %w", path, err)
	}
	return nil
}

func (m *Manager) LoadRunState(id string) (*RunState, error) {
	path, err := m.runStatePath(id)
	if err != nil {
		return nil, err
	}

	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil, fmt.Errorf("run %q: %w", id, ErrNotRunning)
		}
		return nil, fmt.Errorf("đọc %s: %w", path, err)
	}

	var state RunState
	if err := json.Unmarshal(data, &state); err != nil {
		return nil, fmt.Errorf("%s: JSON không hợp lệ: %w", filepath.Base(path), err)
	}
	if state.ID != id {
		return nil, fmt.Errorf("%s: id %q không khớp tên file", filepath.Base(path), state.ID)
	}
	if err := validateRunState(state); err != nil {
		return nil, fmt.Errorf("%s: %w", filepath.Base(path), err)
	}
	return &state, nil
}

func (m *Manager) RemoveRunState(id string) error {
	path, err := m.runStatePath(id)
	if err != nil {
		return err
	}
	if err := os.Remove(path); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return fmt.Errorf("xóa %s: %w", path, err)
	}
	return nil
}

func (m *Manager) ListRunning() ([]RunState, error) {
	dir, err := m.runDir()
	if err != nil {
		return nil, err
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil, nil
		}
		return nil, fmt.Errorf("đọc thư mục run %s: %w", dir, err)
	}

	states := make([]RunState, 0, len(entries))
	for _, entry := range entries {
		if !entry.Type().IsRegular() || !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}
		id := strings.TrimSuffix(entry.Name(), ".json")
		if validateID(id) != nil {
			_ = os.Remove(filepath.Join(dir, entry.Name()))
			continue
		}

		state, err := m.LoadRunState(id)
		if err != nil {
			_ = m.RemoveRunState(id)
			continue
		}
		if !IsProcessAlive(state.PID, state.StartTime) {
			_ = m.RemoveRunState(id)
			continue
		}
		states = append(states, *state)
	}

	sort.Slice(states, func(i, j int) bool {
		return states[i].ID < states[j].ID
	})
	return states, nil
}

func IsProcessAlive(pid int, startTime string) bool {
	if pid <= 0 || strings.TrimSpace(startTime) == "" {
		return false
	}
	actual, err := ProcessStartTime(pid)
	if err != nil {
		return false
	}
	return actual == startTime
}

func StopProcess(state RunState) error {
	if !IsProcessAlive(state.PID, state.StartTime) {
		return fmt.Errorf("tiến trình %d không còn sống hoặc start_time không khớp", state.PID)
	}

	if err := killProcessTree(state.PID); err != nil {
		if !IsProcessAlive(state.PID, state.StartTime) {
			return nil
		}
		return err
	}

	const timeout = 5 * time.Second
	deadline := time.Now().Add(timeout)
	for {
		if !IsProcessAlive(state.PID, state.StartTime) {
			return nil
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("tiến trình %d vẫn còn sống sau %s", state.PID, timeout)
		}
		time.Sleep(50 * time.Millisecond)
	}
}

func validateRunState(state RunState) error {
	if err := validateID(state.ID); err != nil {
		return fmt.Errorf("run id: %w", err)
	}
	if state.PID <= 0 {
		return fmt.Errorf("run %q: pid %d không hợp lệ", state.ID, state.PID)
	}
	if strings.TrimSpace(state.StartTime) == "" {
		return fmt.Errorf("run %q: thiếu start_time", state.ID)
	}
	if _, err := time.Parse(time.RFC3339Nano, state.StartTime); err != nil {
		return fmt.Errorf("run %q: start_time %q không hợp lệ: %w", state.ID, state.StartTime, err)
	}
	return nil
}

func (m *Manager) runDir() (string, error) {
	if strings.TrimSpace(m.dataDir) == "" {
		return "", errors.New("thư mục dữ liệu app rỗng")
	}
	abs, err := filepath.Abs(m.dataDir)
	if err != nil {
		return "", fmt.Errorf("xác định đường dẫn dữ liệu: %w", err)
	}
	return filepath.Join(abs, runDirName), nil
}

func (m *Manager) runStatePath(id string) (string, error) {
	if err := validateID(id); err != nil {
		return "", err
	}

	dir, err := m.runDir()
	if err != nil {
		return "", err
	}
	path := filepath.Join(dir, id+".json")
	absPath, err := filepath.Abs(path)
	if err != nil {
		return "", fmt.Errorf("xác định đường dẫn run %q: %w", id, err)
	}
	if filepath.Dir(absPath) != dir {
		return "", fmt.Errorf("từ chối đường dẫn run %q nằm ngoài %s", id, dir)
	}
	return absPath, nil
}
