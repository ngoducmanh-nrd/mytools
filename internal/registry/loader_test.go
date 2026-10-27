package registry

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"mytools/internal/core"
)

func TestLoadValid(t *testing.T) {
	tools, err := Load(filepath.Join("..", "..", "testdata", "tools_valid.toml"))
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	wantIDs := []string{"movies", "webmy", "dl", "hash", "native-demo"}
	if len(tools) != len(wantIDs) {
		t.Fatalf("got %d tools, want %d", len(tools), len(wantIDs))
	}
	for i, want := range wantIDs {
		if tools[i].ID != want {
			t.Errorf("tools[%d].ID = %q, want %q", i, tools[i].ID, want)
		}
	}

	if got := strings.Join(tools[0].Tags, ","); got != "web,phim" {
		t.Errorf("movies tags = %q", got)
	}
	if tools[2].App != "yt-dlp" {
		t.Errorf("dl app = %q, want yt-dlp", tools[2].App)
	}
	if len(tools[2].Params) != 4 {
		t.Fatalf("dl params = %d, want 4", len(tools[2].Params))
	}

	paramTypes := make(map[string]string)
	for _, tool := range tools {
		for _, param := range tool.Params {
			paramTypes[param.Name] = param.Type
		}
	}
	wantTypes := map[string]string{
		"url":        "string",
		"out":        "path",
		"audio_only": "bool",
		"quality":    "enum",
		"file":       "path",
		"retries":    "int",
	}
	for name, want := range wantTypes {
		if got := paramTypes[name]; got != want {
			t.Errorf("param %q type = %q, want %q", name, got, want)
		}
	}
}

func TestLoadInvalidFixtures(t *testing.T) {
	tests := []struct {
		file    string
		line    string
		field   string
		message string
	}{
		{
			file:    "tools_missing_id.toml",
			line:    ":3:",
			field:   "tool[0].id",
			message: `thiếu trường bắt buộc "id"`,
		},
		{
			file:    "tools_duplicate_id.toml",
			line:    ":10:",
			field:   "tool[1].id",
			message: `trùng id "movies" (dòng 4)`,
		},
		{
			file:    "tools_unknown_kind.toml",
			line:    ":6:",
			field:   "tool[0].kind",
			message: `"xyz" không hợp lệ (chỉ nhận url|process|app|native)`,
		},
		{
			file:    "tools_enum_no_choices.toml",
			line:    ":9:",
			field:   "tool[0].param[0].choices",
			message: `type "enum" bắt buộc có choices`,
		},
	}

	for _, test := range tests {
		t.Run(test.file, func(t *testing.T) {
			_, err := Load(filepath.Join("..", "..", "testdata", test.file))
			if err == nil {
				t.Fatal("Load returned nil error")
			}
			if got := len(joinedErrors(err)); got != 1 {
				t.Errorf("got %d errors, want 1: %v", got, err)
			}
			assertContains(t, err, test.file+test.line, test.field, test.message)
		})
	}
}

func TestLoadKindAppRequiresApp(t *testing.T) {
	path := writeToolsFile(t, `schema_version = 1

[[tool]]
id = "a"
name = "A"
kind = "app"
`)
	_, err := Load(path)
	assertContains(t, err, ":3:", "tool[0].app", `kind "app" bắt buộc có app`)
}

func TestLoadKindProcessRequiresCommand(t *testing.T) {
	path := writeToolsFile(t, `schema_version = 1

[[tool]]
id = "p"
name = "P"
kind = "process"
`)
	_, err := Load(path)
	assertContains(t, err, ":3:", "tool[0].command", `kind "process" bắt buộc có command`)
}

func TestLoadInvalidID(t *testing.T) {
	path := writeToolsFile(t, `schema_version = 1

[[tool]]
id = "Bad"
name = "Bad"
kind = "native"
`)
	_, err := Load(path)
	assertContains(t, err, ":4:", "tool[0].id", `"Bad" không hợp lệ`)
}

func TestLoadInvalidLifecycle(t *testing.T) {
	path := writeToolsFile(t, `schema_version = 1

[[tool]]
id = "a"
name = "A"
kind = "native"
lifecycle = "daemon"
`)
	_, err := Load(path)
	assertContains(t, err, ":7:", "tool[0].lifecycle", `"daemon" không hợp lệ`)
}

func TestLoadForbiddenID(t *testing.T) {
	if len(ForbiddenIDs) != 16 {
		t.Fatalf("ForbiddenIDs has %d entries, want 16", len(ForbiddenIDs))
	}

	path := writeToolsFile(t, `schema_version = 1

[[tool]]
id = "version"
name = "Version"
kind = "native"
`)
	_, err := Load(path)
	assertContains(t, err, ":4:", "tool[0].id", `"version" là tên lệnh dành riêng`)
}

func TestLoadSchemaVersion(t *testing.T) {
	t.Run("wrong", func(t *testing.T) {
		path := writeToolsFile(t, `schema_version = 2

[[tool]]
id = "a"
name = "A"
kind = "native"
`)
		_, err := Load(path)
		assertContains(t, err, ":1:", "schema_version", "phải bằng 1 (đang là 2)")
	})

	t.Run("missing", func(t *testing.T) {
		path := writeToolsFile(t, `[[tool]]
id = "a"
name = "A"
kind = "native"
`)
		_, err := Load(path)
		assertContains(t, err, ":1:", "schema_version", `thiếu trường bắt buộc "schema_version"`)
	})
}

func TestLoadAggregatesErrors(t *testing.T) {
	path := writeToolsFile(t, `schema_version = 1

[[tool]]
name = "Thiếu id"
kind = "url"
target = "https://example.com/"

[[tool]]
id = "bad"
name = "Sai kind"
kind = "xyz"
`)
	_, err := Load(path)
	if err == nil {
		t.Fatal("Load returned nil error")
	}

	errs := joinedErrors(err)
	if len(errs) != 2 {
		t.Fatalf("got %d errors, want 2: %v", len(errs), err)
	}
	assertContains(t, err, "tool[0].id", "tool[1].kind")
}

func TestLoadPathWithSpacesAndVietnamese(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("..", "..", "testdata", "tools_valid.toml"))
	if err != nil {
		t.Fatal(err)
	}

	dir := filepath.Join(t.TempDir(), "thư mục có dấu cách")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "tools.toml")
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatal(err)
	}

	tools, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(tools) != 5 {
		t.Fatalf("got %d tools, want 5", len(tools))
	}
}

func TestApplyLifecycle(t *testing.T) {
	tests := []struct {
		name      string
		lifecycle string
		app       *core.App
		want      string
	}{
		{
			name: "empty with GUI app",
			app:  &core.App{AppSpec: core.AppSpec{GUI: true}},
			want: "window",
		},
		{
			name:      "explicit lifecycle wins",
			lifecycle: "oneshot",
			app:       &core.App{AppSpec: core.AppSpec{GUI: true}},
			want:      "oneshot",
		},
		{
			name: "empty without app",
			want: "",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			tool := &core.Tool{Lifecycle: test.lifecycle}
			ApplyLifecycle(tool, test.app)
			if tool.Lifecycle != test.want {
				t.Errorf("lifecycle = %q, want %q", tool.Lifecycle, test.want)
			}
		})
	}
}

func writeToolsFile(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "tools.toml")
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func joinedErrors(err error) []error {
	if err == nil {
		return nil
	}
	if joined, ok := err.(interface{ Unwrap() []error }); ok {
		return joined.Unwrap()
	}
	return []error{err}
}

func assertContains(t *testing.T, err error, parts ...string) {
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
