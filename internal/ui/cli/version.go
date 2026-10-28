package cli

import (
	"fmt"

	"github.com/spf13/cobra"
)

// Version là phiên bản hiện tại của mt, so với tag trên GitHub Releases khi self-update.
const Version = "0.3.2"

func newVersionCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "In phiên bản",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			_, err := fmt.Fprintln(cmd.OutOrStdout(), "mt "+Version)
			return err
		},
	}
}
