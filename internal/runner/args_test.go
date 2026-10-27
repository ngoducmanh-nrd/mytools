package runner

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestExpandArgs(t *testing.T) {
	params := map[string]string{"url": "https://example.com/v", "out": `C:\Users\me\Downloads`}
	got, err := ExpandArgs([]string{"{url}", "--newline", "-o", "{out}/%(title)s.%(ext)s"}, params)
	if err != nil {
		t.Fatalf("ExpandArgs: %v", err)
	}
	want := []string{"https://example.com/v", "--newline", "-o", `C:\Users\me\Downloads/%(title)s.%(ext)s`}
	if len(got) != len(want) {
		t.Fatalf("got %d args, want %d", len(got), len(want))
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("args[%d] = %q, want %q", i, got[i], want[i])
		}
	}
}

func TestExpandArgsEscapes(t *testing.T) {
	got, err := ExpandArgs([]string{"{{url}}", "{{{url}}}", "{url}-{{x}}"}, map[string]string{"url": "u"})
	if err != nil {
		t.Fatalf("ExpandArgs: %v", err)
	}
	want := []string{"{url}", "{u}", "u-{x}"}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("args[%d] = %q, want %q", i, got[i], want[i])
		}
	}
}

func TestExpandArgsErrors(t *testing.T) {
	tests := []struct {
		arg     string
		message string
	}{
		{"{missing}", "không được khai báo"},
		{"{url", `thiếu dấu "}"`},
		{"url}", `phải viết là "}}"`},
		{"{{url}", `phải viết là "}}"`},
	}
	for _, test := range tests {
		_, err := ExpandArgs([]string{test.arg}, map[string]string{"url": "u"})
		if err == nil {
			t.Fatalf("ExpandArgs(%q) không báo lỗi", test.arg)
		}
		if !strings.Contains(err.Error(), test.message) {
			t.Errorf("error %q không chứa %q", err, test.message)
		}
	}
}

func TestExpandHome(t *testing.T) {
	home, err := os.UserHomeDir()
	if err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		in   string
		want string
	}{
		{"~", home},
		{"~/Downloads", filepath.Join(home, "Downloads")},
		{"~/thư mục có dấu cách", filepath.Join(home, "thư mục có dấu cách")},
		{"./local", "./local"},
		{"", ""},
	}
	for _, test := range tests {
		got, err := ExpandHome(test.in)
		if err != nil {
			t.Errorf("ExpandHome(%q): %v", test.in, err)
			continue
		}
		if got != test.want {
			t.Errorf("ExpandHome(%q) = %q, want %q", test.in, got, test.want)
		}
	}
}
