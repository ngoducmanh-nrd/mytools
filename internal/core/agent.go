package core

// AgentMessage là một tin nhắn trong hội thoại với agent mini (role: user|assistant).
type AgentMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// AgentConfig chứa cấu hình kết nối API LLM dạng OpenAI-compatible do người
// dùng nhập trên giao diện. APIKey chỉ được dùng khi gọi API, không ghi vào
// file hay log ở bất kỳ đâu khác.
type AgentConfig struct {
	BaseURL string `json:"base_url"`
	APIKey  string `json:"api_key"`
	Model   string `json:"model"`
}
