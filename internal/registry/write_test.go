package registry

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"mytools/internal/core"
)

func TestAddToolAppends(t *testing.T) {
	path := writeToolsFile(t, `schema_version = 1

[[tool]]
id = "movies"
name = "Movie Tracker"
kind = "url"
target = "https://example.com/"
tags = ["web"]
`)
	tool := core.Tool{
		ID:   "calc",
		Name: "Máy tính",
		Kind: "app",
		App:  "calc",
		Params: []core.Param{
			{Name: "flag", Type: "bool", Default: false},
			{Name: "count", Type: "int", Default: int64(3)},
			{Name: "mode", Type: "enum", Choices: []string{"a", "b"}, Default: "a"},
		},
		Lifecycle: "oneshot",
	}
	if err := AddTool(path, tool); err != nil {
		t.Fatalf("AddTool: %v", err)
	}

	tools, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(tools) != 2 {
		t.Fatalf("got %d tools, want 2", len(tools))
	}
	got := tools[1]
	if got.ID != "calc" || got.Name != "Máy tính" || got.App != "calc" || got.Lifecycle != "oneshot" {
		t.Fatalf("tool = %+v", got)
	}
	if len(got.Params) != 3 {
		t.Fatalf("params = %+v", got.Params)
	}
	if got.Params[0].Default != false {
		t.Errorf("param flag default = %v, want false", got.Params[0].Default)
	}
	if got.Params[1].Default != int64(3) {
		t.Errorf("param count default = %v (%T), want 3", got.Params[1].Default, got.Params[1].Default)
	}
	if got.Params[2].Default != "a" || len(got.Params[2].Choices) != 2 {
		t.Errorf("param mode = %+v", got.Params[2])
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(data, []byte(`target = "https://example.com/"`)) {
		t.Error("nội dung tool cũ bị thay đổi")
	}
	if !bytes.Contains(data, []byte("[[tool]]\nid = 'calc'")) {
		t.Errorf("file thiếu block tool mới:\n%s", data)
	}
	if bytes.Contains(data, []byte("\n\n\n")) {
		t.Errorf("file có 3 dòng trống liên tiếp:\n%s", data)
	}
}

func TestAddToolDuplicate(t *testing.T) {
	path := writeToolsFile(t, `schema_version = 1

[[tool]]
id = "a"
name = "A"
kind = "native"
`)
	err := AddTool(path, core.Tool{ID: "a", Name: "A2", Kind: "native"})
	assertContains(t, err, `tool "a" đã có`)
}

func TestAddToolInvalid(t *testing.T) {
	path := writeToolsFile(t, "schema_version = 1\n")
	err := AddTool(path, core.Tool{ID: "x"})
	assertContains(t, err, `tool "x" không hợp lệ`, "tool[0].name")
}

func TestAddToolCreatesFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "tools.toml")
	if err := AddTool(path, core.Tool{ID: "a", Name: "A", Kind: "native"}); err != nil {
		t.Fatalf("AddTool: %v", err)
	}
	tools, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(tools) != 1 || tools[0].ID != "a" {
		t.Fatalf("tools = %+v", tools)
	}
}

func TestRemoveTool(t *testing.T) {
	path := writeToolsFile(t, `schema_version = 1

[[tool]]
id = "a"
name = "A"
kind = "native"

[[tool]]
id = "b"
name = "B"
kind = "native"

[[tool]]
id = "c"
name = "C"
kind = "native"
`)
	if err := RemoveTool(path, "b"); err != nil {
		t.Fatalf("RemoveTool: %v", err)
	}
	tools, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(tools) != 2 || tools[0].ID != "a" || tools[1].ID != "c" {
		t.Fatalf("tools = %+v", tools)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(data, []byte("id = \"b\"")) {
		t.Errorf("tool b vẫn còn trong file:\n%s", data)
	}
	if bytes.Contains(data, []byte("\n\n\n")) {
		t.Errorf("file có 3 dòng trống liên tiếp:\n%s", data)
	}
	if !bytes.HasSuffix(data, []byte("kind = \"native\"\n")) {
		t.Errorf("file không kết thúc gọn gàng:\n%q", data)
	}
}

func TestRemoveToolFirstAndLast(t *testing.T) {
	path := writeToolsFile(t, `schema_version = 1

[[tool]]
id = "a"
name = "A"
kind = "native"

[[tool]]
id = "b"
name = "B"
kind = "native"
`)
	if err := RemoveTool(path, "a"); err != nil {
		t.Fatalf("RemoveTool a: %v", err)
	}
	tools, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(tools) != 1 || tools[0].ID != "b" {
		t.Fatalf("tools = %+v", tools)
	}

	if err := RemoveTool(path, "b"); err != nil {
		t.Fatalf("RemoveTool b: %v", err)
	}
	tools, err = Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(tools) != 0 {
		t.Fatalf("tools = %+v", tools)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "schema_version = 1\n" {
		t.Errorf("file sau khi xóa hết:\n%q", data)
	}
}

func TestRemoveToolMissing(t *testing.T) {
	path := writeToolsFile(t, "schema_version = 1\n")
	err := RemoveTool(path, "z")
	assertContains(t, err, `tool "z" không có`)
}

func TestRemoveToolFromInvalidFile(t *testing.T) {
	path := writeToolsFile(t, "schema_version = 1\n\n[[tool]]\n")
	err := RemoveTool(path, "x")
	assertContains(t, err, "tool[0].id")
}
