package cli

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestCompareVersions(t *testing.T) {
	cases := []struct {
		a, b string
		want int
		err  bool
	}{
		{a: "0.1.0", b: "0.1.0", want: 0},
		{a: "v0.1.0", b: "0.1.0", want: 0},
		{a: "0.2.0", b: "0.1.0", want: 1},
		{a: "v0.1.9", b: "v0.1.10", want: -1},
		{a: "1.0", b: "1.0.0", want: 0},
		{a: "v2.0.0-beta.1", b: "v1.9.9", want: 1},
		{a: "abc", b: "1.0", err: true},
		{a: "", b: "1.0", err: true},
	}
	for _, tc := range cases {
		got, err := compareVersions(tc.a, tc.b)
		if tc.err {
			if err == nil {
				t.Errorf("compareVersions(%q, %q) = %d, muốn lỗi", tc.a, tc.b, got)
			}
			continue
		}
		if err != nil {
			t.Errorf("compareVersions(%q, %q): %v", tc.a, tc.b, err)
			continue
		}
		if got != tc.want {
			t.Errorf("compareVersions(%q, %q) = %d, muốn %d", tc.a, tc.b, got, tc.want)
		}
	}
}

func TestAssetName(t *testing.T) {
	cases := []struct{ goos, goarch, want string }{
		{"windows", "amd64", "mt-windows-amd64.exe"},
		{"linux", "amd64", "mt-linux-amd64"},
		{"darwin", "arm64", "mt-darwin-arm64"},
	}
	for _, tc := range cases {
		if got := assetName(tc.goos, tc.goarch); got != tc.want {
			t.Errorf("assetName(%q, %q) = %q, muốn %q", tc.goos, tc.goarch, got, tc.want)
		}
	}
}

func TestFetchLatestRelease(t *testing.T) {
	t.Run("ok", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Header.Get("User-Agent") == "" {
				t.Errorf("thiếu User-Agent")
			}
			_, _ = w.Write([]byte(`{"tag_name":"v0.2.0","assets":[{"name":"mt-windows-amd64.exe","browser_download_url":"http://example.test/mt.exe"}]}`))
		}))
		defer server.Close()

		release, err := fetchLatestRelease(context.Background(), server.Client(), server.URL)
		if err != nil {
			t.Fatal(err)
		}
		if release.TagName != "v0.2.0" {
			t.Errorf("TagName = %q", release.TagName)
		}
		asset, err := findAsset(release, "mt-windows-amd64.exe")
		if err != nil {
			t.Fatal(err)
		}
		if asset.BrowserDownloadURL != "http://example.test/mt.exe" {
			t.Errorf("BrowserDownloadURL = %q", asset.BrowserDownloadURL)
		}
	})

	t.Run("chưa có release", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			http.NotFound(w, r)
		}))
		defer server.Close()

		_, err := fetchLatestRelease(context.Background(), server.Client(), server.URL)
		if !errors.Is(err, errNoRelease) {
			t.Fatalf("err = %v, muốn errNoRelease", err)
		}
	})

	t.Run("lỗi máy chủ", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			http.Error(w, "boom", http.StatusInternalServerError)
		}))
		defer server.Close()

		_, err := fetchLatestRelease(context.Background(), server.Client(), server.URL)
		if err == nil || !strings.Contains(err.Error(), "HTTP 500") {
			t.Fatalf("err = %v, muốn lỗi HTTP 500", err)
		}
	})
}

func TestFindAssetMissing(t *testing.T) {
	release := &releaseInfo{TagName: "v0.2.0", Assets: []releaseAsset{{Name: "mt-linux-amd64", BrowserDownloadURL: "http://example.test/mt"}}}
	_, err := findAsset(release, "mt-windows-amd64.exe")
	if err == nil || !strings.Contains(err.Error(), "mt-windows-amd64.exe") {
		t.Fatalf("err = %v, muốn lỗi thiếu asset", err)
	}
}

func TestBuildUpdateBatch(t *testing.T) {
	script := buildUpdateBatch(4242, `C:\tools\mt\mt.exe`, `C:\tools\mt\mt.exe.new`)

	checks := []string{
		`tasklist /FI "PID eq 4242" /NH`,
		`find "4242"`,
		`goto wait`,
		`del "C:\tools\mt\mt.exe"`,
		`move /Y "C:\tools\mt\mt.exe.new" "C:\tools\mt\mt.exe"`,
		`start "" /D "C:\tools\mt" "C:\tools\mt\mt.exe"`,
		`chcp 65001`,
	}
	for _, want := range checks {
		if !strings.Contains(script, want) {
			t.Errorf("script thiếu %q:\n%s", want, script)
		}
	}
	if strings.Index(script, ":wait") > strings.Index(script, `del "C:\tools\mt\mt.exe"`) {
		t.Errorf("nhãn :wait phải đứng trước lệnh xóa exe:\n%s", script)
	}
}
