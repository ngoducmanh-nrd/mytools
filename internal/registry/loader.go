package registry

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/pelletier/go-toml/v2"

	"mytools/internal/core"
)

const maxValidationErrors = 10

const forbiddenIDList = "list,open,run,app,web,gui,doctor,version,data,ps,stop,restart,logs,help,completion,self-update"

// ForbiddenIDs are command names that cannot be used as tool IDs.
var ForbiddenIDs = strings.Split(forbiddenIDList, ",")

var forbiddenIDSet = func() map[string]struct{} {
	set := make(map[string]struct{}, len(ForbiddenIDs))
	for _, id := range ForbiddenIDs {
		set[id] = struct{}{}
	}
	return set
}()

var toolIDPattern = regexp.MustCompile(`^[a-z][a-z0-9_-]{0,31}$`)

type ValidationError struct {
	File    string
	Line    int
	Column  int
	Field   string
	Message string
}

func (e *ValidationError) Error() string {
	return fmt.Sprintf("%s:%d: %s: %s", e.File, e.Line, e.Field, e.Message)
}

type toolsDocument struct {
	SchemaVersion int         `toml:"schema_version"`
	Tools         []core.Tool `toml:"tool"`
}

func Load(path string) ([]core.Tool, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("đọc %s: %w", path, err)
	}

	var doc toolsDocument
	if err := toml.Unmarshal(data, &doc); err != nil {
		return nil, decodeError(filepath.Base(path), err)
	}

	errs := validateDocument(&doc, indexSource(data), filepath.Base(path))
	if len(errs) > 0 {
		return nil, errors.Join(errs...)
	}
	return doc.Tools, nil
}

func decodeError(file string, err error) error {
	var decodeErr *toml.DecodeError
	if !errors.As(err, &decodeErr) {
		return fmt.Errorf("%s: %w", file, err)
	}

	line, column := decodeErr.Position()
	field := strings.Join(decodeErr.Key(), ".")
	if field == "" {
		field = "toml"
	}
	return &ValidationError{
		File:    file,
		Line:    line,
		Column:  column,
		Field:   field,
		Message: strings.TrimPrefix(decodeErr.Error(), "toml: "),
	}
}

func validateDocument(doc *toolsDocument, idx *sourceIndex, file string) []error {
	var errs []error
	add := func(line int, field, format string, args ...any) {
		if len(errs) >= maxValidationErrors {
			return
		}
		if line < 1 {
			line = 1
		}
		errs = append(errs, &ValidationError{
			File:    file,
			Line:    line,
			Field:   field,
			Message: fmt.Sprintf(format, args...),
		})
	}

	switch {
	case doc.SchemaVersion == 0:
		add(idx.schemaVersionLine(), "schema_version", `thiếu trường bắt buộc "schema_version"`)
	case doc.SchemaVersion != 1:
		add(idx.schemaVersionLine(), "schema_version", "phải bằng 1 (đang là %d)", doc.SchemaVersion)
	}

	seen := make(map[string]int, len(doc.Tools))
	for i := range doc.Tools {
		tool := &doc.Tools[i]
		prefix := fmt.Sprintf("tool[%d]", i)
		toolLine := lineOr(idx.toolLine(i), 1)
		idLine := lineOr(idx.toolFieldLine(i, "id"), toolLine)

		switch {
		case tool.ID == "":
			add(toolLine, prefix+".id", `thiếu trường bắt buộc "id"`)
		case !toolIDPattern.MatchString(tool.ID):
			add(idLine, prefix+".id", "%q không hợp lệ (phải khớp ^[a-z][a-z0-9_-]{0,31}$)", tool.ID)
		case isForbiddenID(tool.ID):
			add(idLine, prefix+".id", "%q là tên lệnh dành riêng, không dùng làm id", tool.ID)
		default:
			if firstLine, ok := seen[tool.ID]; ok {
				add(idLine, prefix+".id", "trùng id %q (dòng %d)", tool.ID, firstLine)
			} else {
				seen[tool.ID] = idLine
			}
		}

		if tool.Name == "" {
			add(toolLine, prefix+".name", `thiếu trường bắt buộc "name"`)
		}

		kindLine := lineOr(idx.toolFieldLine(i, "kind"), toolLine)
		switch {
		case tool.Kind == "":
			add(toolLine, prefix+".kind", `thiếu trường bắt buộc "kind"`)
		case !validKind(tool.Kind):
			add(kindLine, prefix+".kind", "%q không hợp lệ (chỉ nhận url|process|app|native)", tool.Kind)
		case tool.Kind == "url":
			if strings.TrimSpace(tool.Target) == "" {
				add(lineOr(idx.toolFieldLine(i, "target"), toolLine), prefix+".target", `kind "url" bắt buộc có target`)
			}
		case tool.Kind == "app":
			if strings.TrimSpace(tool.App) == "" {
				add(lineOr(idx.toolFieldLine(i, "app"), toolLine), prefix+".app", `kind "app" bắt buộc có app`)
			}
		case tool.Kind == "process":
			if strings.TrimSpace(tool.Command) == "" {
				add(lineOr(idx.toolFieldLine(i, "command"), toolLine), prefix+".command", `kind "process" bắt buộc có command`)
			}
		}

		for j := range tool.Params {
			param := &tool.Params[j]
			paramPrefix := fmt.Sprintf("%s.param[%d]", prefix, j)
			paramLine := lineOr(idx.paramLine(i, j), toolLine)

			if param.Name == "" {
				add(paramLine, paramPrefix+".name", `thiếu trường bắt buộc "name"`)
			}

			switch {
			case param.Type == "":
				add(paramLine, paramPrefix+".type", `thiếu trường bắt buộc "type"`)
			case !validParamType(param.Type):
				add(lineOr(idx.paramFieldLine(i, j, "type"), paramLine), paramPrefix+".type", "%q không hợp lệ (chỉ nhận string|path|bool|int|enum)", param.Type)
			case param.Type == "enum" && len(param.Choices) == 0:
				add(lineOr(idx.paramFieldLine(i, j, "choices"), paramLine), paramPrefix+".choices", `type "enum" bắt buộc có choices`)
			}
		}
	}

	return errs
}

func ApplyLifecycle(tool *core.Tool, app *core.App) {
	if tool == nil || tool.Lifecycle != "" || app == nil || !app.GUI {
		return
	}
	tool.Lifecycle = "window"
}

func lineOr(line, fallback int) int {
	if line > 0 {
		return line
	}
	if fallback > 0 {
		return fallback
	}
	return 1
}

func validKind(kind string) bool {
	switch kind {
	case "url", "process", "app", "native":
		return true
	}
	return false
}

func validParamType(kind string) bool {
	switch kind {
	case "string", "path", "bool", "int", "enum":
		return true
	}
	return false
}

func isForbiddenID(id string) bool {
	_, ok := forbiddenIDSet[id]
	return ok
}
