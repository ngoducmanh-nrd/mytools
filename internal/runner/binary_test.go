package runner

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"mytools/internal/core"
)

func TestDownloadSuccess(t *testing.T) {
	content := []byte("nội dung binary giả")
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/octet-stream")
		_, _ = w.Write(content)
	}))
	defer server.Close()

	dest := filepath.Join(t.TempDir(), "thư mục có dấu cách", "tool.exe")
	sum, err := Download(server.URL+"/tool.exe", dest)
	if err != nil {
		t.Fatalf("Download: %v", err)
	}

	want := sha256.Sum256(content)
	if sum != hex.EncodeToString(want[:]) {
		t.Fatalf("sha256 = %q", sum)
	}
	got, err := os.ReadFile(dest)
	if err != nil {
		t.Fatalf("đọc file tải: %v", err)
	}
	if string(got) != string(content) {
		t.Fatalf("nội dung = %q", got)
	}

	entries, err := os.ReadDir(filepath.Dir(dest))
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 || entries[0].Name() != "tool.exe" {
		t.Fatalf("còn file tạm: %v", entries)
	}
}

func TestDownloadFollowsRedirect(t *testing.T) {
	content := []byte("redirected")
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/download" {
			http.Redirect(w, r, "/real/tool.exe", http.StatusFound)
			return
		}
		w.Header().Set("Content-Type", "application/octet-stream")
		_, _ = w.Write(content)
	}))
	defer server.Close()

	dest := filepath.Join(t.TempDir(), "tool.exe")
	sum, err := Download(server.URL+"/download", dest)
	if err != nil {
		t.Fatalf("Download: %v", err)
	}
	want := sha256.Sum256(content)
	if sum != hex.EncodeToString(want[:]) {
		t.Fatalf("sha256 = %q", sum)
	}
	if _, err := os.Stat(dest); err != nil {
		t.Fatalf("file đích: %v", err)
	}
}

func TestDownloadRejectsNon200(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "không có", http.StatusNotFound)
	}))
	defer server.Close()

	dest := filepath.Join(t.TempDir(), "tool.exe")
	if _, err := Download(server.URL+"/tool.exe", dest); err == nil {
		t.Fatal("Download phải lỗi với HTTP 404")
	}
	if _, err := os.Stat(dest); !os.IsNotExist(err) {
		t.Fatalf("file đích không được tồn tại: %v", err)
	}
}

func TestDownloadRejectsHTML(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write([]byte("<html>không phải binary</html>"))
	}))
	defer server.Close()

	dest := filepath.Join(t.TempDir(), "tool.exe")
	if _, err := Download(server.URL+"/tool.exe", dest); err == nil {
		t.Fatal("Download phải từ chối HTML")
	}
	if _, err := os.Stat(dest); !os.IsNotExist(err) {
		t.Fatalf("file đích không được tồn tại: %v", err)
	}
}

func TestDownloadRejectsEmptyBody(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	dest := filepath.Join(t.TempDir(), "tool.exe")
	if _, err := Download(server.URL+"/tool.exe", dest); err == nil {
		t.Fatal("Download phải lỗi với file rỗng")
	}
	entries, err := os.ReadDir(filepath.Dir(dest))
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Fatalf("còn file lại sau lỗi: %v", entries)
	}
}

func TestInstallWritesStateAndEvents(t *testing.T) {
	content := []byte("binary thật")
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/octet-stream")
		_, _ = w.Write(content)
	}))
	defer server.Close()

	appDir := filepath.Join(t.TempDir(), "apps", "demo")
	app := core.App{
		AppSpec: core.AppSpec{
			SchemaVersion: 1,
			ID:            "demo",
			Runtime:       "binary",
			Download:      server.URL + "/tool.exe",
			Entry:         []string{"tool.exe"},
		},
		Dir: appDir,
	}

	events := make(chan core.Event, 4)
	if err := Install(context.Background(), app, events); err != nil {
		t.Fatalf("Install: %v", err)
	}
	close(events)

	var kinds []string
	for event := range events {
		kinds = append(kinds, event.Kind)
	}
	if len(kinds) != 2 || kinds[0] != "log" || kinds[1] != "result" {
		t.Fatalf("events = %v", kinds)
	}

	if _, err := os.Stat(filepath.Join(appDir, "tool.exe")); err != nil {
		t.Fatalf("file binary: %v", err)
	}
	raw, err := os.ReadFile(filepath.Join(appDir, "state.json"))
	if err != nil {
		t.Fatalf("state.json: %v", err)
	}
	var state core.AppState
	if err := json.Unmarshal(raw, &state); err != nil {
		t.Fatalf("state.json: %v", err)
	}
	if state.URL != app.Download {
		t.Fatalf("URL = %q", state.URL)
	}
	want := sha256.Sum256(content)
	if state.Sha256 != hex.EncodeToString(want[:]) {
		t.Fatalf("sha256 = %q", state.Sha256)
	}
	if state.DownloadedAt.IsZero() {
		t.Fatal("downloaded_at rỗng")
	}
	if _, err := os.Stat(filepath.Join(appDir, "state.json.tmp")); !os.IsNotExist(err) {
		t.Fatalf("còn file .tmp: %v", err)
	}
}

func TestInstallRejectsEntryTraversal(t *testing.T) {
	root := t.TempDir()
	appDir := filepath.Join(root, "apps", "demo")
	outside := filepath.Join(root, "evil.exe")
	app := core.App{
		AppSpec: core.AppSpec{
			ID:       "demo",
			Runtime:  "binary",
			Download: "https://example.com/tool.exe",
			Entry:    []string{"../evil.exe"},
		},
		Dir: appDir,
	}

	err := Install(context.Background(), app, nil)
	if err == nil {
		t.Fatal("Install phải từ chối entry traversal")
	}
	if !strings.Contains(err.Error(), "không chứa đường dẫn") {
		t.Fatalf("lỗi không đúng: %v", err)
	}
	if _, err := os.Stat(outside); !os.IsNotExist(err) {
		t.Fatalf("file ngoài phạm vi bị tạo: %v", err)
	}
}

func TestInstallRejectsNonBinaryRuntime(t *testing.T) {
	app := core.App{
		AppSpec: core.AppSpec{ID: "demo", Runtime: "python", Entry: []string{"python", "app.py"}},
		Dir:     filepath.Join(t.TempDir(), "app"),
	}
	if err := Install(context.Background(), app, nil); err == nil {
		t.Fatal("Install phải từ chối runtime python")
	}
}
