package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestResolveDir(t *testing.T) {
	t.Run("portable next to exe", func(t *testing.T) {
		exeDir := filepath.Join(t.TempDir(), "Thư mục cài đặt")
		if err := os.MkdirAll(exeDir, 0o755); err != nil {
			t.Fatal(err)
		}
		exePath := filepath.Join(exeDir, "mt.exe")
		if err := os.WriteFile(exePath, nil, 0o644); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(exeDir, "portable.txt"), []byte("portable"), 0o644); err != nil {
			t.Fatal(err)
		}

		home := filepath.Join(t.TempDir(), "Nguyễn Văn A")
		got, err := resolveDir(exePath, home)
		if err != nil {
			t.Fatal(err)
		}
		if got != exeDir {
			t.Fatalf("resolveDir = %q, want %q", got, exeDir)
		}
	})

	t.Run("home fallback", func(t *testing.T) {
		exeDir := filepath.Join(t.TempDir(), "bin")
		if err := os.MkdirAll(exeDir, 0o755); err != nil {
			t.Fatal(err)
		}
		home := filepath.Join(t.TempDir(), "Nguyễn Văn A")

		got, err := resolveDir(filepath.Join(exeDir, "mt.exe"), home)
		if err != nil {
			t.Fatal(err)
		}
		want := filepath.Join(home, ".mytools")
		if got != want {
			t.Fatalf("resolveDir = %q, want %q", got, want)
		}
	})
}

func TestEnsureInit(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "dữ liệu có dấu cách")
	if err := ensureInit(dir); err != nil {
		t.Fatal(err)
	}

	configPath := filepath.Join(dir, "config.toml")
	toolsPath := filepath.Join(dir, "tools.toml")

	configData, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(configData), "schema_version = 1") {
		t.Errorf("config.toml missing schema_version: %s", configData)
	}
	if !strings.Contains(string(configData), `update_check_url = ""`) {
		t.Errorf("config.toml missing empty update_check_url: %s", configData)
	}

	toolsData, err := os.ReadFile(toolsPath)
	if err != nil {
		t.Fatal(err)
	}
	movies := strings.Index(string(toolsData), `id = "movies"`)
	webmy := strings.Index(string(toolsData), `id = "webmy"`)
	if movies < 0 || webmy < 0 {
		t.Fatalf("tools.toml missing default tools:\n%s", toolsData)
	}
	if movies > webmy {
		t.Errorf("default tools are not in declaration order")
	}

	matches, err := filepath.Glob(filepath.Join(dir, "*.tmp"))
	if err != nil {
		t.Fatal(err)
	}
	if len(matches) != 0 {
		t.Errorf("temporary files left behind: %v", matches)
	}

	t.Run("does not overwrite existing files", func(t *testing.T) {
		configMarker := []byte("config marker\n")
		toolsMarker := []byte("tools marker\n")
		if err := os.WriteFile(configPath, configMarker, 0o644); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(toolsPath, toolsMarker, 0o644); err != nil {
			t.Fatal(err)
		}

		if err := ensureInit(dir); err != nil {
			t.Fatal(err)
		}

		gotConfig, err := os.ReadFile(configPath)
		if err != nil {
			t.Fatal(err)
		}
		if string(gotConfig) != string(configMarker) {
			t.Errorf("config.toml was overwritten: %q", gotConfig)
		}
		gotTools, err := os.ReadFile(toolsPath)
		if err != nil {
			t.Fatal(err)
		}
		if string(gotTools) != string(toolsMarker) {
			t.Errorf("tools.toml was overwritten: %q", gotTools)
		}
	})

	t.Run("recreates only the missing file", func(t *testing.T) {
		configMarker := []byte("config marker\n")
		if err := os.WriteFile(configPath, configMarker, 0o644); err != nil {
			t.Fatal(err)
		}
		if err := os.Remove(toolsPath); err != nil {
			t.Fatal(err)
		}

		if err := ensureInit(dir); err != nil {
			t.Fatal(err)
		}

		gotConfig, err := os.ReadFile(configPath)
		if err != nil {
			t.Fatal(err)
		}
		if string(gotConfig) != string(configMarker) {
			t.Errorf("existing config.toml changed: %q", gotConfig)
		}
		gotTools, err := os.ReadFile(toolsPath)
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(string(gotTools), `id = "movies"`) {
			t.Errorf("missing tools.toml was not recreated: %s", gotTools)
		}
	})
}

func TestLoad(t *testing.T) {
	path := filepath.Join(t.TempDir(), "thư mục cấu hình", "config.toml")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	content := `# cấu hình thử
schema_version = 1
default_ui = "web"
web_port = "auto"
python = "py -3.12"
keep_logs_days = 7
update_check_url = ""
`
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	cfg, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.SchemaVersion != 1 {
		t.Errorf("SchemaVersion = %d, want 1", cfg.SchemaVersion)
	}
	if cfg.DefaultUI != "web" {
		t.Errorf("DefaultUI = %q, want web", cfg.DefaultUI)
	}
	if cfg.WebPort != "auto" {
		t.Errorf("WebPort = %v, want auto", cfg.WebPort)
	}
	if cfg.Python != "py -3.12" {
		t.Errorf("Python = %q", cfg.Python)
	}
	if cfg.KeepLogsDays != 7 {
		t.Errorf("KeepLogsDays = %d, want 7", cfg.KeepLogsDays)
	}
	if cfg.UpdateCheckURL != "" {
		t.Errorf("UpdateCheckURL = %q, want empty", cfg.UpdateCheckURL)
	}
}

func TestLoadErrors(t *testing.T) {
	t.Run("wrong schema version", func(t *testing.T) {
		path := writeConfigFile(t, `# cấu hình thử
default_ui = "cli"
schema_version = 2
`)
		_, err := Load(path)
		assertErrorContains(t, err, "config.toml:3:", "schema_version", "phải bằng 1 (đang là 2)")
	})

	t.Run("missing schema version", func(t *testing.T) {
		path := writeConfigFile(t, `default_ui = "cli"
`)
		_, err := Load(path)
		assertErrorContains(t, err, "config.toml:1:", "schema_version", `thiếu trường bắt buộc "schema_version"`)
	})

	t.Run("syntax error", func(t *testing.T) {
		path := writeConfigFile(t, `schema_version = 1
default_ui = [bad
`)
		_, err := Load(path)
		assertErrorContains(t, err, "config.toml:2: toml:", "start of value")
	})
}

func writeConfigFile(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "config.toml")
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func assertErrorContains(t *testing.T, err error, parts ...string) {
	t.Helper()
	if err == nil {
		t.Fatal("expected an error, got nil")
	}
	message := err.Error()
	for _, part := range parts {
		if !strings.Contains(message, part) {
			t.Fatalf("error %q does not contain %q", message, part)
		}
	}
}
