package cli

import (
	"context"
	"fmt"
	"os/exec"
	"strings"

	"github.com/spf13/cobra"

	"mytools/internal/core"
)

type doctorCandidate struct {
	command string
	args    []string
}

func newDoctorCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "doctor",
		Short: "Kiểm tra môi trường",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			out := cmd.OutOrStdout()
			for _, check := range doctorChecks(cmd.Context()) {
				status := "FAIL"
				if check.Ok {
					status = "PASS"
				}
				if _, err := fmt.Fprintf(out, "[%s] %-8s %s\n", status, check.Name, check.Message); err != nil {
					return err
				}
				if !check.Ok && check.Hint != "" {
					if _, err := fmt.Fprintf(out, "       Gợi ý: %s\n", check.Hint); err != nil {
						return err
					}
				}
			}
			return nil
		},
	}
}

func doctorChecks(ctx context.Context) []core.Check {
	return []core.Check{
		checkCommand(ctx, "Git", []doctorCandidate{
			{command: "git", args: []string{"--version"}},
		}, "Cài Git tại https://git-scm.com/downloads rồi mở terminal mới."),
		checkCommand(ctx, "Python", []doctorCandidate{
			{command: "py", args: []string{"-3.11", "--version"}},
			{command: "py", args: []string{"-3", "--version"}},
			{command: "python3", args: []string{"--version"}},
			{command: "python", args: []string{"--version"}},
		}, "Cài Python 3.11+ tại https://www.python.org/downloads/ và chọn \"Add python.exe to PATH\"."),
	}
}

func checkCommand(ctx context.Context, name string, candidates []doctorCandidate, hint string) core.Check {
	for _, candidate := range candidates {
		path, err := exec.LookPath(candidate.command)
		if err != nil {
			continue
		}
		out, err := exec.CommandContext(ctx, path, candidate.args...).CombinedOutput()
		if err != nil {
			continue
		}
		version := firstLine(string(out))
		if version == "" {
			version = path
		}
		label := candidate.command
		if len(candidate.args) > 1 {
			label = strings.TrimSpace(candidate.command + " " + strings.Join(candidate.args[:len(candidate.args)-1], " "))
		}
		return core.Check{
			Name:    name,
			Ok:      true,
			Message: fmt.Sprintf("%s (%s)", version, label),
		}
	}
	return core.Check{
		Name:    name,
		Ok:      false,
		Message: "không tìm thấy trong PATH",
		Hint:    hint,
	}
}

func firstLine(text string) string {
	for _, line := range strings.Split(text, "\n") {
		line = strings.TrimSpace(strings.TrimSuffix(line, "\r"))
		if line != "" {
			return line
		}
	}
	return ""
}
