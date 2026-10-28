package runner

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// ExpandHome thay "~" hoặc "~/..." bằng thư mục home của người dùng.
func ExpandHome(path string) (string, error) {
	if path != "~" && !strings.HasPrefix(path, "~/") && !strings.HasPrefix(path, `~\`) {
		return path, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("xác định thư mục người dùng: %w", err)
	}
	if path == "~" {
		return home, nil
	}
	return filepath.Join(home, path[2:]), nil
}

// SplitArgs tách chuỗi tham số thô thành các đối số, hỗ trợ nhóm bằng dấu
// nháy kép: --flag "giá trị có dấu cách". Không hỗ trợ escape bằng backslash.
func SplitArgs(raw string) ([]string, error) {
	var args []string
	var current strings.Builder
	inQuote := false
	hasToken := false
	for i := 0; i < len(raw); i++ {
		ch := raw[i]
		switch {
		case ch == '"':
			inQuote = !inQuote
			hasToken = true
		case ch == ' ' || ch == '\t' || ch == '\r' || ch == '\n':
			if inQuote {
				current.WriteByte(ch)
			} else if hasToken {
				args = append(args, current.String())
				current.Reset()
				hasToken = false
			}
		default:
			current.WriteByte(ch)
			hasToken = true
		}
	}
	if inQuote {
		return nil, errors.New(`thiếu dấu " đóng`)
	}
	if hasToken {
		args = append(args, current.String())
	}
	return args, nil
}

// ExpandArgs thay {tên} trong từng đối số bằng giá trị tham số.
// Viết {{ và }} để có ngoặc thật.
func ExpandArgs(template []string, params map[string]string) ([]string, error) {
	expanded := make([]string, len(template))
	for i, arg := range template {
		value, err := expandArg(arg, params)
		if err != nil {
			return nil, fmt.Errorf("args[%d] = %q: %w", i, arg, err)
		}
		expanded[i] = value
	}
	return expanded, nil
}

func expandArg(arg string, params map[string]string) (string, error) {
	var builder strings.Builder
	for i := 0; i < len(arg); {
		switch {
		case arg[i] == '{' && i+1 < len(arg) && arg[i+1] == '{':
			builder.WriteByte('{')
			i += 2
		case arg[i] == '}' && i+1 < len(arg) && arg[i+1] == '}':
			builder.WriteByte('}')
			i += 2
		case arg[i] == '{':
			end := strings.IndexByte(arg[i+1:], '}')
			if end < 0 {
				return "", errors.New(`thiếu dấu "}" đóng tham số`)
			}
			name := arg[i+1 : i+1+end]
			value, ok := params[name]
			if !ok {
				return "", fmt.Errorf("tham số {%s} không được khai báo", name)
			}
			builder.WriteString(value)
			i += end + 2
		case arg[i] == '}':
			return "", errors.New(`dấu "}" phải viết là "}}"`)
		default:
			builder.WriteByte(arg[i])
			i++
		}
	}
	return builder.String(), nil
}
