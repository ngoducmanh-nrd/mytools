package registry

import (
	"strings"

	"github.com/pelletier/go-toml/v2/unstable"
)

type sourceIndex struct {
	schemaVersion int
	tools         []toolLines
}

type toolLines struct {
	header int
	keys   map[string]int
	params []paramLines
}

type paramLines struct {
	header int
	keys   map[string]int
}

func indexSource(data []byte) *sourceIndex {
	idx := &sourceIndex{}
	var parser unstable.Parser
	parser.Reset(data)

	currentTool, currentParam := -1, -1
	for parser.NextExpression() {
		node := parser.Expression()
		if node == nil {
			break
		}

		switch node.Kind {
		case unstable.Table:
			currentTool, currentParam = -1, -1
		case unstable.ArrayTable:
			line := nodeLine(&parser, node)
			switch nodeKey(node) {
			case "tool":
				idx.tools = append(idx.tools, toolLines{
					header: line,
					keys:   make(map[string]int),
				})
				currentTool = len(idx.tools) - 1
				currentParam = -1
			case "tool.param":
				if currentTool >= 0 {
					tool := &idx.tools[currentTool]
					tool.params = append(tool.params, paramLines{
						header: line,
						keys:   make(map[string]int),
					})
					currentParam = len(tool.params) - 1
				} else {
					currentTool, currentParam = -1, -1
				}
			default:
				currentTool, currentParam = -1, -1
			}
		case unstable.KeyValue:
			line := nodeLine(&parser, node)
			key := nodeKey(node)
			switch {
			case currentTool >= 0 && currentParam >= 0:
				idx.tools[currentTool].params[currentParam].keys[key] = line
			case currentTool >= 0:
				idx.tools[currentTool].keys[key] = line
			case key == "schema_version":
				idx.schemaVersion = line
			}
		}
	}

	return idx
}

func nodeLine(parser *unstable.Parser, node *unstable.Node) int {
	if node.Raw.Length > 0 {
		return parser.Shape(node.Raw).Start.Line
	}
	iterator := node.Key()
	for iterator.Next() {
		key := iterator.Node()
		if key.Raw.Length > 0 {
			return parser.Shape(key.Raw).Start.Line
		}
	}
	return 0
}

func nodeKey(node *unstable.Node) string {
	iterator := node.Key()
	var parts []string
	for iterator.Next() {
		parts = append(parts, string(iterator.Node().Data))
	}
	return strings.Join(parts, ".")
}

func (idx *sourceIndex) schemaVersionLine() int {
	if idx != nil && idx.schemaVersion > 0 {
		return idx.schemaVersion
	}
	return 1
}

func (idx *sourceIndex) toolLine(i int) int {
	if idx != nil && i >= 0 && i < len(idx.tools) {
		return idx.tools[i].header
	}
	return 0
}

func (idx *sourceIndex) toolFieldLine(i int, field string) int {
	if idx == nil || i < 0 || i >= len(idx.tools) {
		return 0
	}
	tool := idx.tools[i]
	if line := tool.keys[field]; line > 0 {
		return line
	}
	return tool.header
}

func (idx *sourceIndex) paramLine(i, j int) int {
	if idx == nil || i < 0 || i >= len(idx.tools) {
		return 0
	}
	tool := idx.tools[i]
	if j >= 0 && j < len(tool.params) {
		return tool.params[j].header
	}
	return tool.header
}

func (idx *sourceIndex) paramFieldLine(i, j int, field string) int {
	if idx == nil || i < 0 || i >= len(idx.tools) {
		return 0
	}
	tool := idx.tools[i]
	if j < 0 || j >= len(tool.params) {
		return tool.header
	}
	param := tool.params[j]
	if line := param.keys[field]; line > 0 {
		return line
	}
	return param.header
}
