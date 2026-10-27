package cli

import (
	"bytes"
	"context"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"mytools/internal/appmgr"
	"mytools/internal/core"
)

func TestConfirmGitRepo(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("không có git trong PATH")
	}

	dataDir := t.TempDir()
	appDir := filepath.Join(dataDir, "apps", "demo")
	if err := os.MkdirAll(appDir, 0o700); err != nil {
		t.Fatal(err)
	}
	spec := `schema_version = 1
id = "demo"
runtime = "binary"
download = "https://example.com/tool.exe"
entry = ["tool.exe"]
`
	if err := os.WriteFile(filepath.Join(appDir, "app.toml"), []byte(spec), 0o644); err != nil {
		t.Fatal(err)
	}
	gitIn(t, appDir, "init", "--quiet")
	gitIn(t, appDir, "add", ".")
	gitIn(t, appDir, "-c", "user.email=test@example.com", "-c", "user.name=Test", "commit", "--quiet", "-m", "init")

	mgr := appmgr.New(dataDir)
	head, err := mgr.HeadCommit(context.Background(), "demo")
	if err != nil {
		t.Fatalf("HeadCommit: %v", err)
	}
	app := &core.App{
		AppSpec: core.AppSpec{ID: "demo", Runtime: "binary", Source: "local", Entry: []string{"tool.exe"}},
		Dir:     appDir,
	}

	in := strings.NewReader("y\npayload\n")
	var out bytes.Buffer
	ok, err := confirmGitRepo(context.Background(), in, &out, mgr, app)
	if err != nil {
		t.Fatalf("confirmGitRepo: %v", err)
	}
	if !ok {
		t.Fatal("nhập y phải chạy tiếp")
	}
	if !strings.Contains(out.String(), "Repo lạ, chạy không? (y/N)") {
		t.Fatalf("thiếu câu hỏi: %q", out.String())
	}
	rest, err := io.ReadAll(in)
	if err != nil {
		t.Fatal(err)
	}
	if string(rest) != "payload\n" {
		t.Fatalf("phần stdin còn lại = %q", rest)
	}
	loaded, err := mgr.LoadApp("demo")
	if err != nil {
		t.Fatal(err)
	}
	if loaded.TrustedCommit != head {
		t.Fatalf("TrustedCommit = %q, muốn %q", loaded.TrustedCommit, head)
	}

	var out2 bytes.Buffer
	ok, err = confirmGitRepo(context.Background(), strings.NewReader(""), &out2, mgr, app)
	if err != nil {
		t.Fatalf("confirmGitRepo lần hai: %v", err)
	}
	if !ok {
		t.Fatal("app đã tin phải chạy không hỏi")
	}
	if strings.Contains(out2.String(), "Repo lạ") {
		t.Fatalf("hỏi lại lần hai: %q", out2.String())
	}
}

func TestConfirmGitRepoDeclined(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("không có git trong PATH")
	}

	dataDir := t.TempDir()
	appDir := filepath.Join(dataDir, "apps", "demo")
	if err := os.MkdirAll(appDir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(appDir, "keep.txt"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	gitIn(t, appDir, "init", "--quiet")
	gitIn(t, appDir, "add", ".")
	gitIn(t, appDir, "-c", "user.email=test@example.com", "-c", "user.name=Test", "commit", "--quiet", "-m", "init")

	app := &core.App{AppSpec: core.AppSpec{ID: "demo", Source: "local"}, Dir: appDir}
	var out bytes.Buffer
	ok, err := confirmGitRepo(context.Background(), strings.NewReader("n\n"), &out, appmgr.New(dataDir), app)
	if err != nil {
		t.Fatalf("confirmGitRepo: %v", err)
	}
	if ok {
		t.Fatal("nhập n không được chạy")
	}
	if app.TrustedCommit != "" {
		t.Fatalf("TrustedCommit = %q", app.TrustedCommit)
	}
}

func gitIn(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
	cmd.Env = append(os.Environ(), "GIT_TERMINAL_PROMPT=0")
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, output)
	}
	return string(output)
}
