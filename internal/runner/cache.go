package runner

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// cacheEnv đảm bảo thư mục cache riêng của mt tồn tại và trả về env đã thay
// biến varName thành "<cacheRoot>/<sub>" để nhốt cache thư viện (pip/npm)
// vào trong mytools thay vì vứt vào AppData của người dùng.
func cacheEnv(env []string, cacheRoot, sub, varName string) ([]string, error) {
	dir := filepath.Join(cacheRoot, sub)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, fmt.Errorf("tạo thư mục cache %s: %w", dir, err)
	}

	out := make([]string, 0, len(env)+1)
	for _, entry := range env {
		key, _, ok := strings.Cut(entry, "=")
		if ok && strings.EqualFold(key, varName) {
			continue
		}
		out = append(out, entry)
	}
	return append(out, varName+"="+dir), nil
}
