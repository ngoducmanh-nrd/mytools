package runner

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"mytools/internal/core"
)

func Download(rawURL, destPath string) (string, error) {
	return download(context.Background(), rawURL, destPath)
}

func Install(ctx context.Context, app core.App, out chan<- core.Event) error {
	if app.Runtime != "binary" {
		return fmt.Errorf("runtime %q chưa hỗ trợ (chỉ nhận %q)", app.Runtime, "binary")
	}
	if app.Download == "" {
		return errors.New(`app.Download: thiếu trường "download"`)
	}
	if len(app.Entry) == 0 {
		return errors.New(`app.Entry: thiếu trường "entry"`)
	}

	destPath, err := installDest(app.Dir, app.Entry[0])
	if err != nil {
		return err
	}

	emit(out, core.Event{RunID: app.ID, Kind: "log", Text: "Đang tải " + app.Download})
	sum, err := download(ctx, app.Download, destPath)
	if err != nil {
		return err
	}

	state := core.AppState{
		URL:          app.Download,
		Sha256:       sum,
		DownloadedAt: time.Now().UTC(),
	}
	if err := writeState(app.Dir, state); err != nil {
		return err
	}

	emit(out, core.Event{
		RunID: app.ID,
		Kind:  "result",
		Text:  "Đã cài " + app.Entry[0],
		Data:  map[string]any{"sha256": sum},
	})
	return nil
}

func download(ctx context.Context, rawURL, destPath string) (string, error) {
	if strings.TrimSpace(destPath) == "" {
		return "", errors.New("đường dẫn đích rỗng")
	}

	request, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return "", fmt.Errorf("tạo yêu cầu tải %s: %w", rawURL, err)
	}
	request.Header.Set("User-Agent", "mt/0.1")

	response, err := http.DefaultClient.Do(request)
	if err != nil {
		return "", fmt.Errorf("tải %s: %w", rawURL, err)
	}
	defer response.Body.Close()

	if response.StatusCode != http.StatusOK {
		return "", fmt.Errorf("tải %s: HTTP %s", rawURL, response.Status)
	}
	if contentType := response.Header.Get("Content-Type"); strings.HasPrefix(strings.ToLower(contentType), "text/html") {
		return "", fmt.Errorf("tải %s: máy chủ trả về HTML, không phải file binary", rawURL)
	}

	dir := filepath.Dir(destPath)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", fmt.Errorf("tạo thư mục %s: %w", dir, err)
	}

	tmp, err := os.CreateTemp(dir, "."+filepath.Base(destPath)+".tmp-*")
	if err != nil {
		return "", fmt.Errorf("tạo file tạm: %w", err)
	}
	tmpPath := tmp.Name()
	done := false
	defer func() {
		if done {
			return
		}
		_ = tmp.Close()
		_ = os.Remove(tmpPath)
	}()

	hasher := sha256.New()
	size, err := io.Copy(io.MultiWriter(tmp, hasher), response.Body)
	if err != nil {
		return "", fmt.Errorf("tải %s: %w", rawURL, err)
	}
	if size == 0 {
		return "", fmt.Errorf("tải %s: file rỗng", rawURL)
	}
	if err := tmp.Sync(); err != nil {
		return "", fmt.Errorf("đồng bộ file tải: %w", err)
	}
	if err := tmp.Chmod(0o755); err != nil {
		return "", fmt.Errorf("đặt quyền file tải: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return "", fmt.Errorf("đóng file tải: %w", err)
	}
	if err := os.Rename(tmpPath, destPath); err != nil {
		return "", fmt.Errorf("đặt file vào %s: %w", destPath, err)
	}
	done = true

	return hex.EncodeToString(hasher.Sum(nil)), nil
}

func installDest(appDir, entry string) (string, error) {
	if appDir == "" || !filepath.IsAbs(appDir) {
		return "", fmt.Errorf("thư mục app %q không phải đường dẫn tuyệt đối", appDir)
	}
	if entry == "" || entry == "." || entry == ".." || filepath.Base(entry) != entry || strings.ContainsAny(entry, `/\`) {
		return "", fmt.Errorf("entry %q phải là tên file trong thư mục app, không chứa đường dẫn", entry)
	}

	dir := filepath.Clean(appDir)
	destPath := filepath.Join(dir, entry)
	if filepath.Dir(destPath) != dir {
		return "", fmt.Errorf("entry %q nằm ngoài thư mục app", entry)
	}
	return destPath, nil
}

func writeState(appDir string, state core.AppState) error {
	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return fmt.Errorf("mã hóa state.json: %w", err)
	}
	data = append(data, '\n')

	path := filepath.Join(appDir, "state.json")
	tmpPath := path + ".tmp"
	file, err := os.OpenFile(tmpPath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o644)
	if err != nil {
		return fmt.Errorf("tạo %s: %w", tmpPath, err)
	}

	success := false
	closed := false
	defer func() {
		if success {
			return
		}
		if !closed {
			_ = file.Close()
		}
		_ = os.Remove(tmpPath)
	}()

	if _, err := file.Write(data); err != nil {
		return fmt.Errorf("ghi %s: %w", tmpPath, err)
	}
	if err := file.Sync(); err != nil {
		return fmt.Errorf("đồng bộ %s: %w", tmpPath, err)
	}
	if err := file.Close(); err != nil {
		return fmt.Errorf("đóng %s: %w", tmpPath, err)
	}
	closed = true
	if err := os.Rename(tmpPath, path); err != nil {
		return fmt.Errorf("đổi tên %s: %w", tmpPath, err)
	}
	success = true
	return nil
}

func emit(out chan<- core.Event, event core.Event) {
	if out == nil {
		return
	}
	select {
	case out <- event:
	default:
	}
}
