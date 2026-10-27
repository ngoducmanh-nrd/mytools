package cli

import (
	"fmt"
	"time"

	"github.com/spf13/cobra"
)

func newPsCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "ps",
		Short: "Liệt kê tool đang chạy",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			mgr, err := newAppManager()
			if err != nil {
				return err
			}
			states, err := mgr.ListRunning()
			if err != nil {
				return err
			}

			out := cmd.OutOrStdout()
			if len(states) == 0 {
				_, err := fmt.Fprintln(out, "Không có tool nào đang chạy.")
				return err
			}

			if _, err := fmt.Fprintf(out, "%-16s  %-8s  %-19s  %-10s  %s\n", "ID", "PID", "START", "UPTIME", "CMD"); err != nil {
				return err
			}
			now := time.Now()
			for _, state := range states {
				started, parseErr := time.Parse(time.RFC3339Nano, state.StartTime)
				startText := state.StartTime
				uptime := "?"
				if parseErr == nil {
					startText = started.Local().Format("2006-01-02 15:04:05")
					uptime = formatUptime(now.Sub(started))
				}
				if _, err := fmt.Fprintf(out, "%-16s  %-8d  %-19s  %-10s  %s\n", state.ID, state.PID, startText, uptime, state.Cmd); err != nil {
					return err
				}
			}
			return nil
		},
	}
}

func formatUptime(d time.Duration) string {
	if d < 0 {
		d = 0
	}
	d = d.Truncate(time.Second)
	switch {
	case d < time.Minute:
		return fmt.Sprintf("%ds", int(d.Seconds()))
	case d < time.Hour:
		return fmt.Sprintf("%dm%02ds", int(d.Minutes()), int(d.Seconds())%60)
	case d < 24*time.Hour:
		return fmt.Sprintf("%dh%02dm", int(d.Hours()), int(d.Minutes())%60)
	default:
		return fmt.Sprintf("%dd%02dh", int(d.Hours())/24, int(d.Hours())%24)
	}
}
