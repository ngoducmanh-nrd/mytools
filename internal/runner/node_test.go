package runner

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"mytools/internal/core"
)

// fakeNpm đặt một npm giả ở đầu PATH; npm-ran.txt được ghi vào thư mục làm việc.
func fakeNpm(t *testing.T) {
	t.Helper()
	dir := t.TempDir()
	if runtime.GOOS == "windows" {
		content := "@echo off\r\necho fake npm install\r\necho installed> npm-ran.txt\r\nexit /b 0\r\n"
		if err := os.WriteFile(filepath.Join(dir, "npm.cmd"), []byte(content), 0o755); err != nil {
			t.Fatal(err)
		}
	} else {
		content := "#!/bin/sh\nprintf 'fake npm install\\n'\n: > npm-ran.txt\n"
		if err := os.WriteFile(filepath.Join(dir, "npm"), []byte(content), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
}

func TestNodeCommandResolvesNpm(t *testing.T) {
	fakeNpm(t)
	appDir := t.TempDir()
	app := core.App{
		AppSpec: core.AppSpec{ID: "demo", Runtime: "node", Entry: []string{"npm", "start"}},
		Dir:     appDir,
	}

	cmd, err := NodeCommand(context.Background(), app, []string{"--dev"})
	if err != nil {
		t.Fatalf("NodeCommand: %v", err)
	}
	if cmd.Dir != appDir {
		t.Fatalf("Dir = %q, muốn %q", cmd.Dir, appDir)
	}
	base := strings.ToLower(filepath.Base(cmd.Path))
	if runtime.GOOS == "windows" {
		if base != "npm.cmd" {
			t.Fatalf("Path = %q, muốn npm.cmd", cmd.Path)
		}
	} else if base != "npm" {
		t.Fatalf("Path = %q, muốn npm", cmd.Path)
	}
	wantArgs := []string{cmd.Path, "start", "--dev"}
	if len(cmd.Args) != len(wantArgs) {
		t.Fatalf("Args = %v, muốn %v", cmd.Args, wantArgs)
	}
	for i := range wantArgs {
		if cmd.Args[i] != wantArgs[i] {
			t.Fatalf("Args = %v, muốn %v", cmd.Args, wantArgs)
		}
	}
}

func TestNodeCommandRejectsInvalidApp(t *testing.T) {
	cases := []core.App{
		{AppSpec: core.AppSpec{ID: "demo", Runtime: "python", Entry: []string{"python", "main.py"}}, Dir: t.TempDir()},
		{AppSpec: core.AppSpec{ID: "demo", Runtime: "node"}, Dir: t.TempDir()},
		{AppSpec: core.AppSpec{ID: "demo", Runtime: "node", Entry: []string{"npm", "start"}}, Dir: "tuong-doi"},
	}
	for _, app := range cases {
		if _, err := NodeCommand(context.Background(), app, nil); err == nil {
			t.Fatalf("NodeCommand(%+v) phải lỗi", app.AppSpec)
		}
	}
}

func TestInstallNodeRunsNpmInstall(t *testing.T) {
	fakeNpm(t)
	appDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(appDir, "package.json"), []byte(`{"name":"demo"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	app := core.App{
		AppSpec: core.AppSpec{ID: "demo", Runtime: "node", Entry: []string{"npm", "start"}},
		Dir:     appDir,
	}

	if err := InstallNode(context.Background(), app, nil); err != nil {
		t.Fatalf("InstallNode: %v", err)
	}
	if _, err := os.Stat(filepath.Join(appDir, "npm-ran.txt")); err != nil {
		t.Fatalf("npm install không chạy trong thư mục app: %v", err)
	}
}

func TestInstallNodeSkipsWhenNodeModulesExists(t *testing.T) {
	fakeNpm(t)
	appDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(appDir, "package.json"), []byte(`{"name":"demo"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(appDir, "node_modules"), 0o700); err != nil {
		t.Fatal(err)
	}
	app := core.App{
		AppSpec: core.AppSpec{ID: "demo", Runtime: "node", Entry: []string{"npm", "start"}},
		Dir:     appDir,
	}

	if err := InstallNode(context.Background(), app, nil); err != nil {
		t.Fatalf("InstallNode: %v", err)
	}
	if _, err := os.Stat(filepath.Join(appDir, "npm-ran.txt")); !os.IsNotExist(err) {
		t.Fatalf("npm install không được chạy khi đã có node_modules: %v", err)
	}
}

func TestInstallNodeWithoutPackageJSONSkips(t *testing.T) {
	fakeNpm(t)
	appDir := t.TempDir()
	app := core.App{
		AppSpec: core.AppSpec{ID: "demo", Runtime: "node", Entry: []string{"node", "server.js"}},
		Dir:     appDir,
	}

	if err := InstallNode(context.Background(), app, nil); err != nil {
		t.Fatalf("InstallNode: %v", err)
	}
	if _, err := os.Stat(filepath.Join(appDir, "npm-ran.txt")); !os.IsNotExist(err) {
		t.Fatalf("không có package.json thì phải bỏ qua npm install: %v", err)
	}
}
