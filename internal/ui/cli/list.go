package cli

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"
)

func newListCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "Liệt kê tool",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			tools, err := loadTools()
			if err != nil {
				return err
			}
			for _, tool := range tools {
				_, err := fmt.Fprintf(cmd.OutOrStdout(), "%-12s  %s  [%s]\n", tool.ID, tool.Name, strings.Join(tool.Tags, ", "))
				if err != nil {
					return err
				}
			}
			return nil
		},
	}
}
