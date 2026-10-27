package appmgr

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"testing"
)

func TestRunStateLifecycle(t *testing.T) {
	mgr := New(t.TempDir())
	start, err := ProcessStartTime(os.Getpid())
	if err != nil {
		t.Fatalf("ProcessStartTime: %v", err)
	}

	state := RunState{ID: "demo", PID: os.Getpid(), StartTime: start, Cmd: "demo --flag"}
	if err := mgr.SaveRunState(state); err != nil {
		t.Fatalf("SaveRunState: %v", err)
	}

	loaded, err := mgr.LoadRunState("demo")
	if err != nil {
		t.Fatalf("LoadRunState: %v", err)
	}
	if *loaded != state {
		t.Fatalf("loaded = %+v, want %+v", *loaded, state)
	}

	running, err := mgr.ListRunning()
	if err != nil {
		t.Fatalf("ListRunning: %v", err)
	}
	if len(running) != 1 || running[0] != state {
		t.Fatalf("ListRunning = %+v, want [%+v]", running, state)
	}

	if err := mgr.RemoveRunState("demo"); err != nil {
		t.Fatalf("RemoveRunState: %v", err)
	}
	if _, err := mgr.LoadRunState("demo"); !errors.Is(err, ErrNotRunning) {
		t.Fatalf("LoadRunState sau khi xóa = %v, want ErrNotRunning", err)
	}
	if err := mgr.RemoveRunState("demo"); err != nil {
		t.Fatalf("RemoveRunState lần hai: %v", err)
	}
}

func TestListRunningCleansStale(t *testing.T) {
	mgr := New(t.TempDir())
	stale := RunState{ID: "stale", PID: os.Getpid(), StartTime: "2000-01-01T00:00:00Z", Cmd: "stale"}
	if err := mgr.SaveRunState(stale); err != nil {
		t.Fatalf("SaveRunState: %v", err)
	}

	running, err := mgr.ListRunning()
	if err != nil {
		t.Fatalf("ListRunning: %v", err)
	}
	if len(running) != 0 {
		t.Fatalf("ListRunning = %+v, want rỗng", running)
	}

	path := filepath.Join(mgr.DataDir(), "run", "stale.json")
	if _, err := os.Stat(path); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("stale.json vẫn còn: %v", err)
	}
}

func TestListRunningCleansCorrupt(t *testing.T) {
	dataDir := t.TempDir()
	mgr := New(dataDir)
	dir := filepath.Join(dataDir, "run")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "broken.json"), []byte("{"), 0o600); err != nil {
		t.Fatal(err)
	}

	running, err := mgr.ListRunning()
	if err != nil {
		t.Fatalf("ListRunning: %v", err)
	}
	if len(running) != 0 {
		t.Fatalf("ListRunning = %+v, want rỗng", running)
	}
	if _, err := os.Stat(filepath.Join(dir, "broken.json")); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("broken.json vẫn còn: %v", err)
	}
}

func TestProcessStartTime(t *testing.T) {
	pid := os.Getpid()
	start, err := ProcessStartTime(pid)
	if err != nil {
		t.Fatalf("ProcessStartTime: %v", err)
	}
	if !IsProcessAlive(pid, start) {
		t.Fatal("tiến trình hiện tại phải còn sống với start_time vừa đọc")
	}
	if IsProcessAlive(pid, "2000-01-01T00:00:00Z") {
		t.Fatal("start_time khác phải bị coi là không sống")
	}
	if IsProcessAlive(0, start) {
		t.Fatal("pid 0 phải bị coi là không sống")
	}
	if IsProcessAlive(pid, "") {
		t.Fatal("start_time rỗng phải bị coi là không sống")
	}
}

func TestSaveRunStateValidation(t *testing.T) {
	mgr := New(t.TempDir())
	tests := []RunState{
		{ID: "../escape", PID: 1, StartTime: "2026-01-01T00:00:00Z"},
		{ID: "ok", PID: 0, StartTime: "2026-01-01T00:00:00Z"},
		{ID: "ok", PID: 1, StartTime: "không phải thời gian"},
	}
	for _, state := range tests {
		if err := mgr.SaveRunState(state); err == nil {
			t.Fatalf("SaveRunState(%+v) không báo lỗi", state)
		}
	}
}
