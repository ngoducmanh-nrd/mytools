package cli

import (
	"fmt"

	"github.com/spf13/cobra"

	"mytools/internal/appmgr"
	"mytools/internal/core"
	"mytools/internal/registry"
)

func newRestartCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "restart <tool-id>",
		Short: "Khởi động lại tool",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			mgr, err := newAppManager()
			if err != nil {
				return err
			}
			target, err := resolveRunTarget(mgr, args[0])
			if err != nil {
				return err
			}
			if !isDetached(target.lifecycle, target.app) {
				return fmt.Errorf("%q không phải tool sống (lifecycle window/service); dùng \"mt run %s\"", args[0], args[0])
			}
			return runApp(cmd, mgr, target.app, target.args, target.id, target.lifecycle, true)
		},
	}
}

type runTarget struct {
	id        string
	app       *core.App
	args      []string
	lifecycle string
}

func resolveRunTarget(mgr *appmgr.Manager, id string) (*runTarget, error) {
	tools, err := loadTools()
	if err != nil {
		return nil, err
	}
	if tool := findTool(tools, id); tool != nil {
		if tool.Kind != "app" {
			return nil, fmt.Errorf("tool %q có kind %q: chỉ hỗ trợ kind \"app\"", id, tool.Kind)
		}
		if tool.App == "" {
			return nil, fmt.Errorf("tool %q thiếu app", id)
		}
		app, err := mgr.LoadApp(tool.App)
		if err != nil {
			return nil, err
		}
		registry.ApplyLifecycle(tool, app)
		return &runTarget{
			id:        tool.ID,
			app:       app,
			args:      append([]string{}, tool.Args...),
			lifecycle: tool.Lifecycle,
		}, nil
	}

	app, err := mgr.LoadApp(id)
	if err != nil {
		return nil, fmt.Errorf("không có tool hay app %q", id)
	}
	return &runTarget{id: id, app: app}, nil
}
