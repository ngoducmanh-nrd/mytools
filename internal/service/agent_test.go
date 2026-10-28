package service

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"mytools/internal/core"
)

func writeAgentToolsFile(t *testing.T, dir string) {
	t.Helper()
	data := `schema_version = 1

[[tool]]
id = "yt-dlp"
name = "Tải video"
kind = "app"
app = "yt-dlp"
target = "yt-dlp.exe"
tags = ["media", "video"]

[[tool]]
id = "movies"
name = "Movie Tracker"
kind = "url"
target = "https://movies.example/"
`
	if err := os.WriteFile(filepath.Join(dir, "tools.toml"), []byte(data), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestAgentChatOK(t *testing.T) {
	var gotPath, gotAuth, gotModel string
	var gotMessages []map[string]string

	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotAuth = r.Header.Get("Authorization")
		var body struct {
			Model    string            `json:"model"`
			Messages []map[string]string `json:"messages"`
			Stream   bool              `json:"stream"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Errorf("decode body: %v", err)
		}
		gotModel = body.Model
		gotMessages = body.Messages
		if body.Stream {
			t.Errorf("stream = true, muốn false")
		}
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"Dùng mt run yt-dlp <url>."}}]}`))
	}))
	defer upstream.Close()

	dir := t.TempDir()
	writeAgentToolsFile(t, dir)
	svc := New(dir)
	defer svc.Close()

	content, err := svc.AgentChat(context.Background(), core.AgentConfig{
		BaseURL: upstream.URL + "/v1",
		APIKey:  "secret-key",
		Model:   "test-model",
	}, []core.AgentMessage{{Role: "user", Content: "Cách tải video?"}})
	if err != nil {
		t.Fatal(err)
	}
	if content != "Dùng mt run yt-dlp <url>." {
		t.Errorf("content = %q", content)
	}
	if gotPath != "/v1/chat/completions" {
		t.Errorf("path = %q", gotPath)
	}
	if gotAuth != "Bearer secret-key" {
		t.Errorf("Authorization = %q", gotAuth)
	}
	if gotModel != "test-model" {
		t.Errorf("model = %q", gotModel)
	}
	if len(gotMessages) != 2 {
		t.Fatalf("số message = %d, muốn 2 (system + user)", len(gotMessages))
	}
	if gotMessages[0]["role"] != "system" {
		t.Errorf("message đầu = %v, muốn system", gotMessages[0])
	}
	sys := gotMessages[0]["content"]
	for _, want := range []string{"yt-dlp", "Tải video", "movies", "Movie Tracker", "mt run <id>"} {
		if !strings.Contains(sys, want) {
			t.Errorf("system prompt thiếu %q:\n%s", want, sys)
		}
	}
	if gotMessages[1]["role"] != "user" || gotMessages[1]["content"] != "Cách tải video?" {
		t.Errorf("message user = %v", gotMessages[1])
	}
}

func TestAgentChatUpstreamError(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"error":{"message":"invalid api key"}}`))
	}))
	defer upstream.Close()

	dir := t.TempDir()
	writeAgentToolsFile(t, dir)
	svc := New(dir)
	defer svc.Close()

	_, err := svc.AgentChat(context.Background(), core.AgentConfig{
		BaseURL: upstream.URL, APIKey: "k", Model: "m",
	}, []core.AgentMessage{{Role: "user", Content: "xin chào"}})
	if err == nil {
		t.Fatal("err = nil, muốn lỗi HTTP 401")
	}
	if !strings.Contains(err.Error(), "HTTP 401") || !strings.Contains(err.Error(), "invalid api key") {
		t.Errorf("err = %v", err)
	}
}

func TestAgentChatBadConfig(t *testing.T) {
	dir := t.TempDir()
	writeAgentToolsFile(t, dir)
	svc := New(dir)
	defer svc.Close()

	msg := []core.AgentMessage{{Role: "user", Content: "hi"}}
	cases := []struct {
		name string
		cfg  core.AgentConfig
		msgs []core.AgentMessage
		want string
	}{
		{name: "thiếu base_url", cfg: core.AgentConfig{Model: "m"}, msgs: msg, want: "base_url"},
		{name: "base_url rác", cfg: core.AgentConfig{BaseURL: "không-phải-url", Model: "m"}, msgs: msg, want: "base_url"},
		{name: "thiếu model", cfg: core.AgentConfig{BaseURL: "http://127.0.0.1:1"}, msgs: msg, want: "model"},
		{name: "thiếu tin nhắn", cfg: core.AgentConfig{BaseURL: "http://127.0.0.1:1", Model: "m"}, msgs: nil, want: "tin nhắn"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := svc.AgentChat(context.Background(), tc.cfg, tc.msgs)
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Errorf("err = %v, muốn chứa %q", err, tc.want)
			}
		})
	}
}

func TestCleanAgentHistory(t *testing.T) {
	got := cleanAgentHistory([]core.AgentMessage{
		{Role: "user", Content: "1"},
		{Role: "system", Content: "bỏ qua role lạ"},
		{Role: "assistant", Content: "  "},
		{Role: "assistant", Content: "2"},
		{Role: "user", Content: "3"},
	})
	if len(got) != 3 {
		t.Fatalf("len = %d, muốn 3: %v", len(got), got)
	}
	if got[0].Content != "1" || got[1].Content != "2" || got[2].Content != "3" {
		t.Errorf("got = %v", got)
	}

	many := make([]core.AgentMessage, 0, 12)
	for i := 0; i < 12; i++ {
		many = append(many, core.AgentMessage{Role: "user", Content: string(rune('a' + i))})
	}
	got = cleanAgentHistory(many)
	if len(got) != agentHistoryLen {
		t.Fatalf("len = %d, muốn %d", len(got), agentHistoryLen)
	}
	if got[0].Content != "e" {
		t.Errorf("tin đầu sau cắt = %q, muốn giữ 8 tin gần nhất", got[0].Content)
	}
}

func TestAgentEndpoint(t *testing.T) {
	cases := []struct {
		raw  string
		want string
		err  bool
	}{
		{raw: "https://api.groq.com/openai/v1", want: "https://api.groq.com/openai/v1/chat/completions"},
		{raw: "http://localhost:11434/v1/", want: "http://localhost:11434/v1/chat/completions"},
		{raw: "", err: true},
		{raw: "ftp://x.example", err: true},
		{raw: "không phải url", err: true},
	}
	for _, tc := range cases {
		got, err := agentEndpoint(tc.raw)
		if tc.err {
			if err == nil {
				t.Errorf("agentEndpoint(%q) = %q, muốn lỗi", tc.raw, got)
			}
			continue
		}
		if err != nil {
			t.Errorf("agentEndpoint(%q): %v", tc.raw, err)
			continue
		}
		if got != tc.want {
			t.Errorf("agentEndpoint(%q) = %q, muốn %q", tc.raw, got, tc.want)
		}
	}
}
