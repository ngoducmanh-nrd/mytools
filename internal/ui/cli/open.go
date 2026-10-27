package cli

import (
	"fmt"

	"github.com/spf13/cobra"

	"mytools/internal/runner"
)

func newOpenCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "open <id>",
		Short: "Mở tool",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			tools, err := loadTools()
			if err != nil {
				return err
			}

			id := args[0]
			for _, tool := range tools {
				if tool.ID != id {
					continue
				}
				if tool.Kind != "url" {
					return fmt.Errorf("tool %q có kind %q; mt open chỉ hỗ trợ kind=url", tool.ID, tool.Kind)
				}
				return runner.Open(cmd.Context(), tool.Target)
			}

			return fmt.Errorf("không tìm thấy tool %q", id)
		},
	}
}
