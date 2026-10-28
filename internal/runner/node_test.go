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

// fakeNpm đặt một npm giả ở đầu PATH; npm-ran.txt ghi khi install,
// npm-build-ran.txt ghi khi chạy script build.
func fakeNpm(t *testing.T) {
	t.Helper()
	dir := t.TempDir()
	if runtime.GOOS == "windows" {
		content := "@echo off\r\necho fake npm %*\r\nif \"%1\"==\"install\" echo installed> npm-ran.txt\r\nif \"%1\"==\"run\" echo built> npm-build-ran.txt\r\nexit /b 0\r\n"
		if err := os.WriteFile(filepath.Join(dir, "npm.cmd"), []byte(content), 0o755); err != nil {
			t.Fatal(err)
		}
	} else {
		content := "#!/bin/sh\nprintf 'fake npm %s\\n' \"$*\"\nif [ \"$1\" = \"install\" ]; then : > npm-ran.txt; fi\nif [ \"$1\" = \"run\" ]; then : > npm-build-ran.txt; fi\n"
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

func TestNodeCommandStripsElectronRunAsNodeForGUI(t *testing.T) {
	fakeNpm(t)
	t.Setenv("ELECTRON_RUN_AS_NODE", "1")
	appDir := t.TempDir()
	app := core.App{
		AppSpec: core.AppSpec{ID: "gui-app", Runtime: "node", Entry: []string{"npm", "start"}, GUI: true},
		Dir:     appDir,
	}

	cmd, err := NodeCommand(context.Background(), app, nil)
	if err != nil {
		t.Fatalf("NodeCommand: %v", err)
	}
	for _, entry := range cmd.Env {
		if key, _, ok := strings.Cut(entry, "="); ok && strings.EqualFold(key, "ELECTRON_RUN_AS_NODE") {
			t.Fatalf("GUI app không được giữ ELECTRON_RUN_AS_NODE trong env: %q", entry)
		}
	}

	app.GUI = false
	cmd, err = NodeCommand(context.Background(), app, nil)
	if err != nil {
		t.Fatalf("NodeCommand: %v", err)
	}
	if cmd.Env != nil {
		t.Fatalf("app không GUI phải kế thừa env của tiến trình mt, Env = %v", cmd.Env)
	}
}

func TestNodeCommandIsolatesNpmCache(t *testing.T) {
	fakeNpm(t)
	cacheRoot := t.TempDir()
	app := core.App{
		AppSpec:  core.AppSpec{ID: "demo", Runtime: "node", Entry: []string{"npm", "start"}},
		Dir:      t.TempDir(),
		CacheDir: cacheRoot,
	}

	cmd, err := NodeCommand(context.Background(), app, nil)
	if err != nil {
		t.Fatalf("NodeCommand: %v", err)
	}
	want := "npm_config_cache=" + filepath.Join(cacheRoot, "npm")
	found := false
	for _, entry := range cmd.Env {
		if entry == want {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("cmd.Env thiếu %q", want)
	}
	if _, err := os.Stat(filepath.Join(cacheRoot, "npm")); err != nil {
		t.Fatalf("thư mục cache npm không được tạo: %v", err)
	}
}

func TestUsesNpx(t *testing.T) {
	if UsesNpx(core.App{AppSpec: core.AppSpec{Runtime: "node", Entry: []string{"npx", "x"}}}) != true {
		t.Fatal("entry npx phải được nhận diện")
	}
	if UsesNpx(core.App{AppSpec: core.AppSpec{Runtime: "node", Entry: []string{"npm", "start"}}}) {
		t.Fatal("entry npm không phải npx")
	}
	if UsesNpx(core.App{AppSpec: core.AppSpec{Runtime: "python", Entry: []string{"npx", "x"}}}) {
		t.Fatal("npx chỉ áp dụng cho runtime node")
	}
	if UsesNpx(core.App{AppSpec: core.AppSpec{Runtime: "node"}}) {
		t.Fatal("entry rỗng không phải npx")
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

func TestInstallNodeRunsBuild(t *testing.T) {
	fakeNpm(t)
	appDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(appDir, "package.json"), []byte(`{"name":"demo"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	app := core.App{
		AppSpec: core.AppSpec{
			ID:      "demo",
			Runtime: "node",
			Entry:   []string{"npm", "start"},
			Build:   []string{"npm", "run", "build"},
		},
		Dir: appDir,
	}

	if err := InstallNode(context.Background(), app, nil); err != nil {
		t.Fatalf("InstallNode: %v", err)
	}
	for _, name := range []string{"npm-ran.txt", "npm-build-ran.txt"} {
		if _, err := os.Stat(filepath.Join(appDir, name)); err != nil {
			t.Fatalf("thiếu marker %s: %v", name, err)
		}
	}
}

func TestInstallNodeBuildWithExistingNodeModules(t *testing.T) {
	fakeNpm(t)
	appDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(appDir, "package.json"), []byte(`{"name":"demo"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(appDir, "node_modules"), 0o700); err != nil {
		t.Fatal(err)
	}
	app := core.App{
		AppSpec: core.AppSpec{
			ID:      "demo",
			Runtime: "node",
			Entry:   []string{"npm", "start"},
			Build:   []string{"npm", "run", "build"},
		},
		Dir: appDir,
	}

	if err := InstallNode(context.Background(), app, nil); err != nil {
		t.Fatalf("InstallNode: %v", err)
	}
	if _, err := os.Stat(filepath.Join(appDir, "npm-ran.txt")); !os.IsNotExist(err) {
		t.Fatalf("npm install không được chạy khi đã có node_modules: %v", err)
	}
	if _, err := os.Stat(filepath.Join(appDir, "npm-build-ran.txt")); err != nil {
		t.Fatalf("build không chạy dù đã có node_modules: %v", err)
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
