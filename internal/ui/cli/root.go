package cli

import (
	"os"
	"path/filepath"

	"github.com/spf13/cobra"

	"mytools/internal/config"
	"mytools/internal/core"
	"mytools/internal/registry"
)

func Execute() error {
	root := NewRootCmd()
	root.SetOut(os.Stdout)
	root.SetErr(os.Stderr)
	return root.Execute()
}

func NewRootCmd() *cobra.Command {
	root := &cobra.Command{
		Use:           "mt",
		Short:         "Hub tool cá nhân",
		SilenceUsage:  true,
		SilenceErrors: true,
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			return config.EnsureInit()
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			return runDefaultUI(cmd)
		},
	}
	root.CompletionOptions.DisableDefaultCmd = true
	root.AddCommand(newVersionCmd(), newListCmd(), newOpenCmd(), newRunCmd(), newAppCmd(), newDoctorCmd(), newPsCmd(), newStopCmd(), newRestartCmd(), newLogsCmd(), newWebCmd())
	return root
}

func loadTools() ([]core.Tool, error) {
	dir, err := config.ResolveDir()
	if err != nil {
		return nil, err
	}
	return registry.Load(filepath.Join(dir, "tools.toml"))
}
