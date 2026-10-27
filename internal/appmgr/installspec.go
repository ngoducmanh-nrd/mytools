package appmgr

import (
	"fmt"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"strings"

	"mytools/internal/core"
)

// DirectBinaryURL cho biết arg có phải URL trỏ thẳng tới file binary (có phần
// mở rộng, không phải .git) hay không; dùng để phân loại nguồn khi cài app.
func DirectBinaryURL(arg string) (string, bool) {
	parsed, err := url.Parse(arg)
	if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" {
		return "", false
	}
	name := path.Base(parsed.Path)
	if name == "." || name == "/" || name == "" {
		return "", false
	}
	ext := path.Ext(name)
	if ext == "" || ext == "." || strings.EqualFold(ext, ".git") {
		return "", false
	}
	return arg, true
}

// BinarySpec dựng AppSpec binary từ URL trỏ thẳng tới file thực thi.
func BinarySpec(rawURL string) (core.AppSpec, error) {
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return core.AppSpec{}, fmt.Errorf("URL không hợp lệ: %w", err)
	}
	name := path.Base(parsed.Path)
	if name == "." || name == "/" || name == "" {
		return core.AppSpec{}, fmt.Errorf("URL %q phải có tên file", rawURL)
	}
	ext := path.Ext(name)
	if ext == "" || ext == "." {
		return core.AppSpec{}, fmt.Errorf("URL %q phải trỏ trực tiếp tới file binary (có phần mở rộng)", rawURL)
	}

	id := SanitizeAppID(strings.TrimSuffix(name, ext))
	if id == "" {
		return core.AppSpec{}, fmt.Errorf("không suy ra được id app từ tên file %q", name)
	}

	return core.AppSpec{
		SchemaVersion: 1,
		ID:            id,
		Runtime:       "binary",
		Download:      rawURL,
		Entry:         []string{name},
	}, nil
}

// SanitizeAppID chuyển tên thành id app hợp lệ: chữ thường, chỉ a-z 0-9 - _.
func SanitizeAppID(name string) string {
	name = strings.ToLower(name)
	var builder strings.Builder
	for _, r := range name {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9', r == '-', r == '_':
			builder.WriteRune(r)
		default:
			builder.WriteRune('-')
		}
	}
	return strings.Trim(builder.String(), "-_")
}

// IsLocalToolDir cho biết arg có phải thư mục local chứa app.toml hay không.
func IsLocalToolDir(arg string) bool {
	info, err := os.Stat(arg)
	if err != nil || !info.IsDir() {
		return false
	}
	_, err = os.Stat(filepath.Join(arg, appSpecName))
	return err == nil
}
