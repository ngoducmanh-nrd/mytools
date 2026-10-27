package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"mytools/internal/appmgr"
)

func newLogsCmd() *cobra.Command {
	var follow bool
	cmd := &cobra.Command{
		Use:   "logs <tool-id>",
		Short: "Xem log của tool",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			mgr, err := newAppManager()
			if err != nil {
				return err
			}
			appID, err := resolveLogApp(mgr, args[0])
			if err != nil {
				return err
			}
			path, err := latestLogPath(filepath.Join(mgr.DataDir(), "logs", appID))
			if err != nil {
				return err
			}
			return printLog(cmd, path, follow)
		},
	}
	cmd.Flags().BoolVarP(&follow, "follow", "f", false, "theo dõi log theo thời gian thực")
	return cmd
}

func resolveLogApp(mgr *appmgr.Manager, id string) (string, error) {
	tools, err := loadTools()
	if err != nil {
		return "", err
	}
	if tool := findTool(tools, id); tool != nil {
		if tool.Kind != "app" || tool.App == "" {
			return "", fmt.Errorf("tool %q không phải app", id)
		}
		if _, err := mgr.LoadApp(tool.App); err != nil {
			return "", err
		}
		return tool.App, nil
	}
	if _, err := mgr.LoadApp(id); err != nil {
		return "", fmt.Errorf("không có tool hay app %q", id)
	}
	return id, nil
}

func latestLogPath(dir string) (string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return "", fmt.Errorf("chưa có log trong %s", dir)
		}
		return "", fmt.Errorf("đọc thư mục log %s: %w", dir, err)
	}

	var newest string
	var newestTime time.Time
	for _, entry := range entries {
		if !entry.Type().IsRegular() || !strings.HasSuffix(entry.Name(), ".log") {
			continue
		}
		info, err := entry.Info()
		if err != nil {
			continue
		}
		path := filepath.Join(dir, entry.Name())
		if newest == "" || info.ModTime().After(newestTime) ||
			(info.ModTime().Equal(newestTime) && entry.Name() > filepath.Base(newest)) {
			newest = path
			newestTime = info.ModTime()
		}
	}
	if newest == "" {
		return "", fmt.Errorf("chưa có file log trong %s", dir)
	}
	return newest, nil
}

func printLog(cmd *cobra.Command, path string, follow bool) error {
	file, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("mở log %s: %w", path, err)
	}
	defer file.Close()

	out := cmd.OutOrStdout()
	if _, err := io.Copy(out, file); err != nil {
		return fmt.Errorf("đọc log %s: %w", path, err)
	}
	if !follow {
		return nil
	}

	base := cmd.Context()
	if base == nil {
		base = context.Background()
	}
	ctx, stop := signal.NotifyContext(base, os.Interrupt)
	defer stop()

	buf := make([]byte, 32*1024)
	for {
		n, readErr := file.Read(buf)
		if n > 0 {
			if _, err := out.Write(buf[:n]); err != nil {
				return fmt.Errorf("ghi log: %w", err)
			}
			continue
		}
		if readErr != nil && !errors.Is(readErr, io.EOF) {
			return fmt.Errorf("đọc log %s: %w", path, readErr)
		}
		select {
		case <-ctx.Done():
			return nil
		case <-time.After(200 * time.Millisecond):
		}
	}
}
