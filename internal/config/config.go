package config

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/pelletier/go-toml/v2"
)

const (
	appDirName   = ".mytools"
	portableFile = "portable.txt"
	configFile   = "config.toml"
	toolsFile    = "tools.toml"
)

type Config struct {
	SchemaVersion  int    `toml:"schema_version"`
	DefaultUI      string `toml:"default_ui"`
	WebPort        any    `toml:"web_port"`
	Python         string `toml:"python"`
	KeepLogsDays   int    `toml:"keep_logs_days"`
	UpdateCheckURL string `toml:"update_check_url"`
}

func ResolveDir() (string, error) {
	exePath, err := os.Executable()
	if err != nil {
		return "", fmt.Errorf("xác định đường dẫn exe: %w", err)
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("xác định thư mục người dùng: %w", err)
	}
	return resolveDir(exePath, home)
}

func resolveDir(exePath, home string) (string, error) {
	exeDir := filepath.Dir(exePath)
	portablePath := filepath.Join(exeDir, portableFile)

	info, err := os.Stat(portablePath)
	switch {
	case err == nil && !info.IsDir():
		return exeDir, nil
	case err == nil:
		return filepath.Join(home, appDirName), nil
	case errors.Is(err, fs.ErrNotExist):
		return filepath.Join(home, appDirName), nil
	default:
		return "", fmt.Errorf("kiểm tra %s: %w", portablePath, err)
	}
}

func EnsureInit() error {
	dir, err := ResolveDir()
	if err != nil {
		return err
	}
	return ensureInit(dir)
}

func ensureInit(dir string) error {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("tạo thư mục %s: %w", dir, err)
	}
	if err := os.MkdirAll(filepath.Join(dir, "apps"), 0o700); err != nil {
		return fmt.Errorf("tạo thư mục apps trong %s: %w", dir, err)
	}
	if err := ensureFile(filepath.Join(dir, configFile), defaultConfigTOML); err != nil {
		return err
	}
	return ensureFile(filepath.Join(dir, toolsFile), defaultToolsTOML)
}

func ensureFile(path, content string) error {
	_, err := os.Stat(path)
	switch {
	case err == nil:
		return nil
	case !errors.Is(err, fs.ErrNotExist):
		return fmt.Errorf("kiểm tra %s: %w", path, err)
	}

	if err := writeFileAtomic(path, []byte(content), 0o644); err != nil {
		return fmt.Errorf("ghi %s: %w", path, err)
	}
	return nil
}

func writeFileAtomic(path string, data []byte, perm fs.FileMode) error {
	tmpPath := path + ".tmp"
	file, err := os.OpenFile(tmpPath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, perm)
	if err != nil {
		return err
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
		return err
	}
	if err := file.Sync(); err != nil {
		return err
	}
	if err := file.Close(); err != nil {
		return err
	}
	closed = true

	if err := os.Rename(tmpPath, path); err != nil {
		return err
	}
	success = true
	return nil
}

func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("đọc %s: %w", path, err)
	}

	var cfg Config
	if err := toml.Unmarshal(data, &cfg); err != nil {
		return nil, configDecodeError(filepath.Base(path), err)
	}

	if cfg.SchemaVersion != 1 {
		line := schemaVersionLine(data)
		if cfg.SchemaVersion == 0 {
			return nil, fmt.Errorf("%s:%d: schema_version: thiếu trường bắt buộc %q", filepath.Base(path), line, "schema_version")
		}
		return nil, fmt.Errorf("%s:%d: schema_version: phải bằng 1 (đang là %d)", filepath.Base(path), line, cfg.SchemaVersion)
	}

	return &cfg, nil
}

func configDecodeError(file string, err error) error {
	var decodeErr *toml.DecodeError
	if !errors.As(err, &decodeErr) {
		return fmt.Errorf("%s: %w", file, err)
	}

	line, _ := decodeErr.Position()
	field := strings.Join(decodeErr.Key(), ".")
	if field == "" {
		field = "toml"
	}
	return fmt.Errorf("%s:%d: %s: %s", file, line, field, strings.TrimPrefix(decodeErr.Error(), "toml: "))
}

func schemaVersionLine(data []byte) int {
	for i, line := range strings.Split(string(data), "\n") {
		trimmed := strings.TrimSpace(line)
		if !strings.HasPrefix(trimmed, "schema_version") {
			continue
		}
		rest := strings.TrimSpace(strings.TrimPrefix(trimmed, "schema_version"))
		if strings.HasPrefix(rest, "=") {
			return i + 1
		}
	}
	return 1
}

const defaultConfigTOML = `schema_version = 1
default_ui = "cli"
web_port = 7878
python = "auto"
keep_logs_days = 14

# Để trống dùng GitHub Releases chính thức của mt; đổi URL để trỏ nơi khác.
update_check_url = ""
`

const defaultToolsTOML = `schema_version = 1

[[tool]]
id = "movies"
name = "Movie Tracker"
kind = "url"
target = "https://movie-tracker-v2-jade.vercel.app/"
tags = ["web", "phim"]

[[tool]]
id = "webmy"
name = "webmy"
kind = "url"
target = "https://webmy-8gf.pages.dev/"
tags = ["web"]

[[tool]]
id = "dl"
name = "Tải video"
kind = "app"
app = "yt-dlp"
args = ["{url}", "--newline", "--no-colors", "-o", "{out}/%(title)s.%(ext)s"]
tags = ["media"]

  [[tool.param]]
  name = "url"
  type = "string"
  required = true

  [[tool.param]]
  name = "out"
  type = "path"
  default = "~/Downloads"

[[tool]]
id = "auto-music-studio"
name = "Auto Music Studio"
kind = "app"
app = "auto-music-studio"
tags = ["media", "music", "electron"]
`
