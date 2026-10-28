package cli

import (
	"errors"
	"fmt"
	"io"
	"slices"
	"strconv"
	"strings"

	"github.com/spf13/pflag"

	"mytools/internal/core"
	"mytools/internal/runner"
)

type paramBinding struct {
	param    core.Param
	flagName string
	str      *string
	boolean  *bool
	number   *int
}

// parseToolArgs sinh cờ CLI từ param của tool, trả về giá trị tham số và các đối số còn lại.
func parseToolArgs(tool *core.Tool, args []string) (map[string]string, []string, error) {
	if tool == nil {
		return nil, nil, errors.New("thiếu tool để đọc tham số")
	}
	if len(tool.Params) == 0 {
		return map[string]string{}, stripDashDash(args), nil
	}

	flags := pflag.NewFlagSet("run "+tool.ID, pflag.ContinueOnError)
	flags.SetOutput(io.Discard)
	flags.SortFlags = false

	bindings := make([]paramBinding, 0, len(tool.Params))
	for _, param := range tool.Params {
		name := flagName(param.Name)
		if flags.Lookup(name) != nil {
			return nil, nil, fmt.Errorf("tool %q: param %q trùng cờ --%s", tool.ID, param.Name, name)
		}
		binding := paramBinding{param: param, flagName: name}
		switch param.Type {
		case "string", "path", "enum":
			def, err := stringDefault(param)
			if err != nil {
				return nil, nil, fmt.Errorf("tool %q: %w", tool.ID, err)
			}
			binding.str = flags.String(name, def, "")
		case "bool":
			def, err := boolDefault(param)
			if err != nil {
				return nil, nil, fmt.Errorf("tool %q: %w", tool.ID, err)
			}
			binding.boolean = flags.Bool(name, def, "")
		case "int":
			def, err := intDefault(param)
			if err != nil {
				return nil, nil, fmt.Errorf("tool %q: %w", tool.ID, err)
			}
			binding.number = flags.Int(name, def, "")
		default:
			return nil, nil, fmt.Errorf("tool %q: param %q có type %q chưa hỗ trợ", tool.ID, param.Name, param.Type)
		}
		bindings = append(bindings, binding)
	}

	if err := flags.Parse(args); err != nil {
		return nil, nil, fmt.Errorf("tool %q: %w", tool.ID, err)
	}

	values := make(map[string]string, len(bindings))
	var missing []string
	for _, binding := range bindings {
		param := binding.param
		if param.Required && !flags.Changed(binding.flagName) {
			missing = append(missing, "--"+binding.flagName)
			continue
		}

		value, err := bindingValue(binding)
		if err != nil {
			return nil, nil, fmt.Errorf("tool %q: --%s: %w", tool.ID, binding.flagName, err)
		}
		if param.Required && value == "" && param.Type != "bool" {
			missing = append(missing, "--"+binding.flagName)
			continue
		}
		values[param.Name] = value
	}
	if len(missing) > 0 {
		return nil, nil, fmt.Errorf("tool %q: thiếu tham số bắt buộc: %s", tool.ID, strings.Join(missing, ", "))
	}

	return values, flags.Args(), nil
}

func bindingValue(binding paramBinding) (string, error) {
	param := binding.param
	switch param.Type {
	case "string":
		return *binding.str, nil
	case "path":
		return runner.ExpandHome(*binding.str)
	case "enum":
		value := *binding.str
		if value == "" {
			return "", nil
		}
		if !slices.Contains(param.Choices, value) {
			return "", fmt.Errorf("%q không hợp lệ (chọn: %s)", value, strings.Join(param.Choices, ", "))
		}
		return value, nil
	case "bool":
		return strconv.FormatBool(*binding.boolean), nil
	case "int":
		return strconv.Itoa(*binding.number), nil
	default:
		return "", fmt.Errorf("type %q chưa hỗ trợ", param.Type)
	}
}

// stripDashDash bỏ dấu "--" phân cách raw args khi tool không khai báo param.
func stripDashDash(args []string) []string {
	if len(args) > 0 && args[0] == "--" {
		return args[1:]
	}
	return args
}

func flagName(name string) string {
	return strings.ReplaceAll(name, "_", "-")
}

func stringDefault(param core.Param) (string, error) {
	if param.Default == nil {
		return "", nil
	}
	value, ok := param.Default.(string)
	if !ok {
		return "", fmt.Errorf("param %q: default phải là chuỗi", param.Name)
	}
	return value, nil
}

func boolDefault(param core.Param) (bool, error) {
	if param.Default == nil {
		return false, nil
	}
	value, ok := param.Default.(bool)
	if !ok {
		return false, fmt.Errorf("param %q: default phải là bool", param.Name)
	}
	return value, nil
}

func intDefault(param core.Param) (int, error) {
	if param.Default == nil {
		return 0, nil
	}
	switch value := param.Default.(type) {
	case int64:
		return int(value), nil
	case int:
		return value, nil
	default:
		return 0, fmt.Errorf("param %q: default phải là số nguyên", param.Name)
	}
}
