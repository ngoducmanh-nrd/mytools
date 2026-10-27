package registry

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/pelletier/go-toml/v2"

	"mytools/internal/core"
)

// toolBlock là dạng marshal gọn của core.Tool: chỉ ghi trường có giá trị,
// tránh làm file registry đầy trường rỗng.
type toolBlock struct {
	ID        string       `toml:"id"`
	Name      string       `toml:"name"`
	Kind      string       `toml:"kind"`
	Target    string       `toml:"target,omitempty"`
	App       string       `toml:"app,omitempty"`
	Command   string       `toml:"command,omitempty"`
	Args      []string     `toml:"args,omitempty"`
	Tags      []string     `toml:"tags,omitempty"`
	Params    []paramBlock `toml:"param,omitempty"`
	Lifecycle string       `toml:"lifecycle,omitempty"`
}

type paramBlock struct {
	Name     string   `toml:"name"`
	Type     string   `toml:"type"`
	Required bool     `toml:"required,omitempty"`
	Default  any      `toml:"default,omitempty"`
	Choices  []string `toml:"choices,omitempty"`
}

type toolsAppend struct {
	Tools []toolBlock `toml:"tool"`
}

// AddTool nối thêm một block [[tool]] vào cuối file registry, giữ nguyên nội dung cũ.
func AddTool(path string, tool core.Tool) error {
	file := filepath.Base(path)
	data, err := os.ReadFile(path)
	switch {
	case err == nil:
		existing, parseErr := parse(data, file)
		if parseErr != nil {
			return parseErr
		}
		for _, t := range existing {
			if t.ID == tool.ID {
				return fmt.Errorf("tool %q đã có trong %s", tool.ID, file)
			}
		}
	case errors.Is(err, os.ErrNotExist):
		data = []byte("schema_version = 1\n")
	default:
		return fmt.Errorf("đọc %s: %w", path, err)
	}

	block, err := toml.Marshal(toolsAppend{Tools: []toolBlock{newToolBlock(tool)}})
	if err != nil {
		return fmt.Errorf("mã hóa tool %q: %w", tool.ID, err)
	}
	out := appendToolBlock(data, block)
	if _, err := parse(out, file); err != nil {
		return fmt.Errorf("tool %q không hợp lệ: %w", tool.ID, err)
	}
	return writeFileAtomic(path, out)
}

// RemoveTool cắt block [[tool]] của tool khỏi file, giữ nguyên phần còn lại.
func RemoveTool(path, id string) error {
	file := filepath.Base(path)
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("đọc %s: %w", path, err)
	}
	existing, err := parse(data, file)
	if err != nil {
		return err
	}

	removed := -1
	for i, t := range existing {
		if t.ID == id {
			removed = i
			break
		}
	}
	if removed < 0 {
		return fmt.Errorf("tool %q không có trong %s", id, file)
	}

	idx := indexSource(data)
	start := idx.toolLine(removed)
	end := 0
	if removed+1 < len(idx.tools) {
		end = idx.toolLine(removed + 1)
	}

	out, err := cutBlock(data, start, end)
	if err != nil {
		return fmt.Errorf("xóa tool %q khỏi %s: %w", id, file, err)
	}
	if _, err := parse(out, file); err != nil {
		return fmt.Errorf("sau khi xóa tool %q: %w", id, err)
	}
	return writeFileAtomic(path, out)
}

func newToolBlock(tool core.Tool) toolBlock {
	block := toolBlock{
		ID:        tool.ID,
		Name:      tool.Name,
		Kind:      tool.Kind,
		Target:    tool.Target,
		App:       tool.App,
		Command:   tool.Command,
		Args:      tool.Args,
		Tags:      tool.Tags,
		Lifecycle: tool.Lifecycle,
	}
	for _, param := range tool.Params {
		block.Params = append(block.Params, paramBlock{
			Name:     param.Name,
			Type:     param.Type,
			Required: param.Required,
			Default:  param.Default,
			Choices:  param.Choices,
		})
	}
	return block
}

func appendToolBlock(data, block []byte) []byte {
	out := append([]byte{}, data...)
	if len(out) == 0 || out[len(out)-1] != '\n' {
		out = append(out, '\n')
	}
	if len(out) < 2 || out[len(out)-2] != '\n' {
		out = append(out, '\n')
	}
	out = append(out, block...)
	return out
}

// cutBlock xóa các dòng từ start (1-based, bao gồm) đến end (1-based, không bao gồm).
// end = 0 nghĩa là tới hết file.
func cutBlock(data []byte, start, end int) ([]byte, error) {
	lines := bytes.Split(data, []byte("\n"))
	if start < 1 || start > len(lines) || (end > 0 && end < start) {
		return nil, fmt.Errorf("vị trí xóa không hợp lệ (start=%d, end=%d, dòng=%d)", start, end, len(lines))
	}
	if end == 0 || end > len(lines) {
		end = len(lines)
	}

	kept := make([][]byte, 0, len(lines)-(end-start))
	kept = append(kept, lines[:start-1]...)
	kept = append(kept, lines[end-1:]...)
	return normalizeLines(kept), nil
}

func normalizeLines(lines [][]byte) []byte {
	out := bytes.Join(lines, []byte("\n"))
	out = bytes.TrimRight(out, "\n")
	out = append(out, '\n')
	for bytes.Contains(out, []byte("\n\n\n")) {
		out = bytes.ReplaceAll(out, []byte("\n\n\n"), []byte("\n\n"))
	}
	return out
}

func writeFileAtomic(path string, data []byte) error {
	dir := filepath.Dir(path)
	tmp, err := os.CreateTemp(dir, "."+filepath.Base(path)+".tmp-*")
	if err != nil {
		return fmt.Errorf("tạo file tạm trong %s: %w", dir, err)
	}
	tmpPath := tmp.Name()
	done := false
	defer func() {
		if done {
			return
		}
		_ = tmp.Close()
		_ = os.Remove(tmpPath)
	}()

	if _, err := tmp.Write(data); err != nil {
		return fmt.Errorf("ghi %s: %w", tmpPath, err)
	}
	if err := tmp.Sync(); err != nil {
		return fmt.Errorf("đồng bộ %s: %w", tmpPath, err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("đóng %s: %w", tmpPath, err)
	}
	if err := os.Rename(tmpPath, path); err != nil {
		return fmt.Errorf("đổi tên %s: %w", tmpPath, err)
	}
	done = true
	return nil
}
