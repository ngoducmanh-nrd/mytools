package cli

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"mytools/internal/runner"
)

const defaultUpdateURL = "https://api.github.com/repos/ngoducmanh-nrd/mytools/releases/latest"

var errNoRelease = errors.New("chưa có bản phát hành nào")

type releaseAsset struct {
	Name               string `json:"name"`
	BrowserDownloadURL string `json:"browser_download_url"`
}

type releaseInfo struct {
	TagName string         `json:"tag_name"`
	Assets  []releaseAsset `json:"assets"`
}

func newSelfUpdateCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "self-update",
		Short: "Cập nhật mt lên bản mới nhất từ GitHub Releases",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runSelfUpdate(cmd)
		},
	}
}

func runSelfUpdate(cmd *cobra.Command) error {
	out := cmd.OutOrStdout()
	cfg, err := loadConfig()
	if err != nil {
		return err
	}
	endpoint := strings.TrimSpace(cfg.UpdateCheckURL)
	if endpoint == "" {
		endpoint = defaultUpdateURL
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	release, err := fetchLatestRelease(ctx, http.DefaultClient, endpoint)
	if err != nil {
		if errors.Is(err, errNoRelease) {
			_, werr := fmt.Fprintf(out, "Bạn đang dùng bản mới nhất v%s (chưa có bản phát hành nào trên GitHub).\n", Version)
			return werr
		}
		return fmt.Errorf("không kiểm tra được bản mới: %w", err)
	}

	latest := strings.TrimSpace(release.TagName)
	newer, err := versionNewer(latest, Version)
	if err != nil {
		return fmt.Errorf("tag %q không hợp lệ: %w", release.TagName, err)
	}
	if !newer {
		_, werr := fmt.Fprintf(out, "Bạn đang dùng bản mới nhất v%s.\n", Version)
		return werr
	}

	name := assetName(runtime.GOOS, runtime.GOARCH)
	asset, err := findAsset(release, name)
	if err != nil {
		return err
	}

	exePath, err := os.Executable()
	if err != nil {
		return fmt.Errorf("xác định đường dẫn exe: %w", err)
	}
	exePath, err = filepath.Abs(exePath)
	if err != nil {
		return fmt.Errorf("xác định đường dẫn exe: %w", err)
	}
	newPath := exePath + ".new"

	_, _ = fmt.Fprintf(out, "Có bản mới %s (hiện tại v%s). Đang tải %s...\n", latest, Version, name)
	if err := downloadFile(ctx, asset.BrowserDownloadURL, newPath); err != nil {
		return fmt.Errorf("tải bản mới: %w", err)
	}

	display := strings.TrimPrefix(latest, "v")
	if runtime.GOOS == "windows" {
		batchPath := filepath.Join(filepath.Dir(exePath), "update.bat")
		script := buildUpdateBatch(os.Getpid(), exePath, newPath)
		if err := os.WriteFile(batchPath, []byte(script), 0o755); err != nil {
			return fmt.Errorf("ghi %s: %w", batchPath, err)
		}
		process := exec.Command("cmd.exe", "/C", batchPath)
		process.SysProcAttr = runner.DetachedSysProcAttr()
		if err := process.Start(); err != nil {
			return fmt.Errorf("chạy %s: %w", batchPath, err)
		}
		_ = process.Process.Release()
		_, werr := fmt.Fprintf(out, "Đã tải v%s. update.bat sẽ thay mt.exe sau khi tiến trình này thoát rồi khởi động lại mt.\n", display)
		return werr
	}

	if err := os.Chmod(newPath, 0o755); err != nil {
		return fmt.Errorf("cấp quyền chạy cho %s: %w", newPath, err)
	}
	if err := os.Rename(newPath, exePath); err != nil {
		return fmt.Errorf("thay thế %s: %w", exePath, err)
	}
	_, werr := fmt.Fprintf(out, "Đã cập nhật lên v%s. Chạy lại mt để dùng bản mới.\n", display)
	return werr
}

func fetchLatestRelease(ctx context.Context, client *http.Client, endpoint string) (*releaseInfo, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", "mt-self-update")

	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return nil, errNoRelease
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("GET %s: HTTP %d", endpoint, resp.StatusCode)
	}

	var release releaseInfo
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&release); err != nil {
		return nil, fmt.Errorf("đọc JSON từ %s: %w", endpoint, err)
	}
	if strings.TrimSpace(release.TagName) == "" {
		return nil, fmt.Errorf("%s: JSON thiếu tag_name", endpoint)
	}
	return &release, nil
}

func downloadFile(ctx context.Context, url, dest string) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", "mt-self-update")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("GET %s: HTTP %d", url, resp.StatusCode)
	}

	file, err := os.OpenFile(dest, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o755)
	if err != nil {
		return err
	}
	if _, err := io.Copy(file, resp.Body); err != nil {
		_ = file.Close()
		_ = os.Remove(dest)
		return fmt.Errorf("ghi %s: %w", dest, err)
	}
	if err := file.Sync(); err != nil {
		_ = file.Close()
		_ = os.Remove(dest)
		return fmt.Errorf("ghi %s: %w", dest, err)
	}
	if err := file.Close(); err != nil {
		_ = os.Remove(dest)
		return fmt.Errorf("ghi %s: %w", dest, err)
	}
	return nil
}

func assetName(goos, goarch string) string {
	name := fmt.Sprintf("mt-%s-%s", goos, goarch)
	if goos == "windows" {
		name += ".exe"
	}
	return name
}

func findAsset(release *releaseInfo, name string) (releaseAsset, error) {
	for _, asset := range release.Assets {
		if asset.Name == name && asset.BrowserDownloadURL != "" {
			return asset, nil
		}
	}
	return releaseAsset{}, fmt.Errorf("bản phát hành %s không có asset %q cho máy này", release.TagName, name)
}

func versionNewer(latest, current string) (bool, error) {
	compare, err := compareVersions(latest, current)
	if err != nil {
		return false, err
	}
	return compare > 0, nil
}

func compareVersions(a, b string) (int, error) {
	pa, err := parseVersion(a)
	if err != nil {
		return 0, err
	}
	pb, err := parseVersion(b)
	if err != nil {
		return 0, err
	}
	for i := 0; i < len(pa) || i < len(pb); i++ {
		x := segment(pa, i)
		y := segment(pb, i)
		if x != y {
			if x < y {
				return -1, nil
			}
			return 1, nil
		}
	}
	return 0, nil
}

func parseVersion(v string) ([]int, error) {
	v = strings.TrimSpace(v)
	v = strings.TrimPrefix(v, "v")
	v = strings.TrimPrefix(v, "V")
	if idx := strings.IndexAny(v, "-+"); idx >= 0 {
		v = v[:idx]
	}
	if v == "" {
		return nil, errors.New("rỗng")
	}
	parts := strings.Split(v, ".")
	numbers := make([]int, len(parts))
	for i, part := range parts {
		n, err := strconv.Atoi(part)
		if err != nil || n < 0 {
			return nil, fmt.Errorf("phần %q không phải số nguyên", part)
		}
		numbers[i] = n
	}
	return numbers, nil
}

func segment(parts []int, i int) int {
	if i < len(parts) {
		return parts[i]
	}
	return 0
}

// buildUpdateBatch sinh script chờ tiến trình hiện tại thoát rồi thay file exe.
// Windows khóa file exe đang chạy nên phải hoãn việc thay thế sang một tiến trình khác.
func buildUpdateBatch(pid int, exePath, newPath string) string {
	exeDir := filepath.Dir(exePath)
	return fmt.Sprintf(`@echo off
chcp 65001 >nul
rem Cho mt.exe (PID %d) thoat roi thay file.
:wait
tasklist /FI "PID eq %d" /NH 2>nul | find "%d" >nul
if not errorlevel 1 (
  timeout /t 1 /nobreak >nul
  goto wait
)
del "%s"
move /Y "%s" "%s"
start "" /D "%s" "%s"
`, pid, pid, pid, exePath, newPath, exePath, exeDir, exePath)
}
