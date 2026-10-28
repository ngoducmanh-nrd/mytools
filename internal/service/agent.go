package service

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"mytools/internal/core"
)

const (
	agentTimeout     = 2 * time.Minute
	maxAgentReply    = 1 << 20
	agentHistoryLen  = 8
	agentErrorSnippet = 300
)

// agentSystemPrompt mô tả ngắn gọn mt + danh sách công cụ thật để agent trả
// lời đúng ngay từ tin đầu, không tốn token giải thích lại.
const agentSystemPrompt = `Bạn là trợ lý mini của mt (mytools) — hub công cụ cá nhân chạy local trên máy người dùng (Windows/Linux). Trả lời ngắn gọn, đúng trọng tâm, bằng tiếng Việt.

Công cụ đang có (id | tên | loại | tham số):
%s

Cách dùng:
- CLI: "mt run <id> [đối số]" (chạy tool), "mt open <id>" (tool dạng url), "mt list", "mt ps", "mt stop <id>", "mt logs <id>".
- Web UI: chọn tool ở cột giữa, điền tham số hoặc Raw Args rồi bấm Chạy ngay.

Chỉ gợi ý công cụ có trong danh sách trên; nếu thiếu thông tin thì hỏi lại người dùng.`

// AgentChat gửi hội thoại tới API LLM dạng OpenAI-compatible do người dùng
// cấu hình (Groq, OpenRouter, Ollama local...) và trả về câu trả lời.
func (s *Service) AgentChat(ctx context.Context, cfg core.AgentConfig, messages []core.AgentMessage) (string, error) {
	endpoint, err := agentEndpoint(cfg.BaseURL)
	if err != nil {
		return "", err
	}
	model := strings.TrimSpace(cfg.Model)
	if model == "" {
		return "", errors.New("thiếu model")
	}

	history := cleanAgentHistory(messages)
	if len(history) == 0 {
		return "", errors.New("thiếu tin nhắn")
	}

	tools, err := s.ListTools()
	if err != nil {
		return "", fmt.Errorf("đọc danh sách công cụ: %w", err)
	}

	all := make([]map[string]string, 0, len(history)+1)
	all = append(all, map[string]string{
		"role": "system", "content": fmt.Sprintf(agentSystemPrompt, agentToolList(tools)),
	})
	for _, msg := range history {
		all = append(all, map[string]string{"role": msg.Role, "content": msg.Content})
	}

	payload, err := json.Marshal(map[string]any{
		"model":    model,
		"messages": all,
		"stream":   false,
	})
	if err != nil {
		return "", fmt.Errorf("mã hóa yêu cầu agent: %w", err)
	}

	request, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(payload))
	if err != nil {
		return "", fmt.Errorf("tạo yêu cầu agent: %w", err)
	}
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("User-Agent", "mt/0.1")
	if key := strings.TrimSpace(cfg.APIKey); key != "" {
		request.Header.Set("Authorization", "Bearer "+key)
	}

	response, err := (&http.Client{Timeout: agentTimeout}).Do(request)
	if err != nil {
		return "", fmt.Errorf("gọi API agent: %w", err)
	}
	defer response.Body.Close()

	body, err := io.ReadAll(io.LimitReader(response.Body, maxAgentReply+1))
	if err != nil {
		return "", fmt.Errorf("đọc phản hồi agent: %w", err)
	}
	if len(body) > maxAgentReply {
		return "", errors.New("phản hồi agent quá lớn (hơn 1 MB)")
	}

	if response.StatusCode != http.StatusOK {
		return "", fmt.Errorf("API agent trả HTTP %d: %s", response.StatusCode, snippet(body))
	}

	var parsed struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
		Error struct {
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.Unmarshal(body, &parsed); err != nil {
		return "", fmt.Errorf("phản hồi agent không phải JSON: %w", err)
	}
	if parsed.Error.Message != "" {
		return "", fmt.Errorf("API agent báo lỗi: %s", parsed.Error.Message)
	}
	if len(parsed.Choices) == 0 || strings.TrimSpace(parsed.Choices[0].Message.Content) == "" {
		return "", errors.New("phản hồi agent không có nội dung")
	}
	return parsed.Choices[0].Message.Content, nil
}

// agentEndpoint kiểm tra base_url người dùng nhập rồi nối chuẩn
// /chat/completions, phù hợp mọi API dạng OpenAI-compatible.
func agentEndpoint(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", errors.New("thiếu base_url")
	}
	parsed, err := url.Parse(raw)
	if err != nil || parsed.Host == "" || (parsed.Scheme != "http" && parsed.Scheme != "https") {
		return "", fmt.Errorf("base_url %q không hợp lệ (cần địa chỉ http(s)://host)", raw)
	}
	return strings.TrimRight(raw, "/") + "/chat/completions", nil
}

// cleanAgentHistory chỉ giữ các tin nhắn user/assistant có nội dung, giới hạn
// số tin gần nhất để không tốn token cho hội thoại dài.
func cleanAgentHistory(messages []core.AgentMessage) []core.AgentMessage {
	kept := make([]core.AgentMessage, 0, len(messages))
	for _, msg := range messages {
		if (msg.Role == "user" || msg.Role == "assistant") && strings.TrimSpace(msg.Content) != "" {
			kept = append(kept, msg)
		}
	}
	if len(kept) > agentHistoryLen {
		kept = kept[len(kept)-agentHistoryLen:]
	}
	return kept
}

func agentToolList(tools []core.Tool) string {
	var b strings.Builder
	for _, tool := range tools {
		params := "không khai báo tham số"
		if len(tool.Params) > 0 {
			names := make([]string, 0, len(tool.Params))
			for _, p := range tool.Params {
				names = append(names, p.Name)
			}
			params = strings.Join(names, ", ")
		}
		kind := tool.Kind
		if kind == "" {
			kind = "tool"
		}
		fmt.Fprintf(&b, "- %s | %s | %s | %s\n", tool.ID, tool.Name, kind, params)
	}
	return strings.TrimRight(b.String(), "\n")
}

// snippet cắt ngắn nội dung lỗi từ upstream để không kéo theo cả trang HTML
// hay nội dung nhạy cảm vào thông báo lỗi.
func snippet(body []byte) string {
	text := strings.TrimSpace(string(body))
	if len(text) > agentErrorSnippet {
		return text[:agentErrorSnippet] + "..."
	}
	return text
}
