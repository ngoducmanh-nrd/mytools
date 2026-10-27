package appmgr

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDirectBinaryURL(t *testing.T) {
	cases := []struct {
		arg  string
		want bool
	}{
		{"https://example.com/tools/My%20Tool.exe?download=1", true},
		{"https://example.com/file.zip", true},
		{"https://github.com/user/repo.git", false},
		{"https://github.com/user/repo", false},
		{"https://example.com/", false},
		{"ftp://example.com/tool.exe", false},
		{"./app", false},
		{"C:\\tool\\app.exe", false},
		{"", false},
	}
	for _, tc := range cases {
		if _, got := DirectBinaryURL(tc.arg); got != tc.want {
			t.Fatalf("DirectBinaryURL(%q) = %v, muốn %v", tc.arg, got, tc.want)
		}
	}
}

func TestBinarySpec(t *testing.T) {
	rawURL := "https://example.com/tools/My%20Tool.exe?download=1"
	spec, err := BinarySpec(rawURL)
	if err != nil {
		t.Fatalf("BinarySpec: %v", err)
	}
	if spec.ID != "my-tool" {
		t.Fatalf("ID = %q", spec.ID)
	}
	if spec.Runtime != "binary" || spec.Download != rawURL {
		t.Fatalf("spec = %+v", spec)
	}
	if len(spec.Entry) != 1 || spec.Entry[0] != "My Tool.exe" {
		t.Fatalf("Entry = %v", spec.Entry)
	}
}

func TestBinarySpecRejectsMissingName(t *testing.T) {
	if _, err := BinarySpec("https://example.com/"); err == nil {
		t.Fatal("BinarySpec phải lỗi khi URL không có tên file")
	}
}

func TestIsLocalToolDir(t *testing.T) {
	dir := t.TempDir()
	withAppTOML := filepath.Join(dir, "có app.toml")
	if err := os.MkdirAll(withAppTOML, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(withAppTOML, "app.toml"), []byte("schema_version = 1\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	empty := filepath.Join(dir, "trống")
	if err := os.MkdirAll(empty, 0o700); err != nil {
		t.Fatal(err)
	}

	cases := []struct {
		arg  string
		want bool
	}{
		{withAppTOML, true},
		{empty, false},
		{filepath.Join(dir, "không tồn tại"), false},
		{"https://example.com/repo", false},
		{"", false},
	}
	for _, tc := range cases {
		if got := IsLocalToolDir(tc.arg); got != tc.want {
			t.Fatalf("IsLocalToolDir(%q) = %v, muốn %v", tc.arg, got, tc.want)
		}
	}
}
