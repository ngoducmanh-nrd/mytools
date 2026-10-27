package cli

import (
	"errors"
	"fmt"

	"github.com/spf13/cobra"

	"mytools/internal/appmgr"
)

func newStopCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "stop <tool-id>",
		Short: "Dừng tool đang chạy",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			mgr, err := newAppManager()
			if err != nil {
				return err
			}
			return stopRun(cmd, mgr, args[0], true)
		},
	}
}

func stopRun(cmd *cobra.Command, mgr *appmgr.Manager, id string, announceNotRunning bool) error {
	out := cmd.OutOrStdout()
	state, err := mgr.LoadRunState(id)
	if err != nil {
		if errors.Is(err, appmgr.ErrNotRunning) {
			if !announceNotRunning {
				return nil
			}
			_, writeErr := fmt.Fprintf(out, "%q không chạy.\n", id)
			return writeErr
		}
		if removeErr := mgr.RemoveRunState(id); removeErr != nil {
			return err
		}
		if !announceNotRunning {
			return nil
		}
		_, writeErr := fmt.Fprintf(out, "%q không chạy (đã dọn trạng thái hỏng).\n", id)
		return writeErr
	}

	if !appmgr.IsProcessAlive(state.PID, state.StartTime) {
		if err := mgr.RemoveRunState(id); err != nil {
			return err
		}
		if !announceNotRunning {
			return nil
		}
		_, writeErr := fmt.Fprintf(out, "%q không chạy (đã dọn trạng thái cũ).\n", id)
		return writeErr
	}

	if err := appmgr.StopProcess(*state); err != nil {
		if !appmgr.IsProcessAlive(state.PID, state.StartTime) {
			if removeErr := mgr.RemoveRunState(id); removeErr != nil {
				return removeErr
			}
			if !announceNotRunning {
				return nil
			}
			_, writeErr := fmt.Fprintf(out, "%q không chạy (đã dọn trạng thái cũ).\n", id)
			return writeErr
		}
		return fmt.Errorf("dừng %q: %w", id, err)
	}
	if err := mgr.RemoveRunState(id); err != nil {
		return err
	}
	_, err = fmt.Fprintf(out, "Đã dừng %q (PID %d).\n", id, state.PID)
	return err
}
