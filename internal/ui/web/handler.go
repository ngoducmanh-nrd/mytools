package web

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"mytools/internal/core"
)

func (s *Server) handleIndex(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	_, _ = w.Write(s.indexHTML)
}

func (s *Server) handleStyle(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/css; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	_, _ = w.Write(s.styleCSS)
}

func (s *Server) handleAppJS(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/javascript; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	_, _ = w.Write(s.appJS)
}

type toolJSON struct {
	ID     string      `json:"id"`
	Name   string      `json:"name"`
	Kind   string      `json:"kind"`
	Target string      `json:"target,omitempty"`
	Tags   []string    `json:"tags,omitempty"`
	Params []paramJSON `json:"params"`
}

type paramJSON struct {
	Name     string   `json:"name"`
	Type     string   `json:"type"`
	Required bool     `json:"required"`
	Default  any      `json:"default,omitempty"`
	Choices  []string `json:"choices,omitempty"`
}

func (s *Server) handleTools(w http.ResponseWriter, r *http.Request) {
	tools, err := s.runner.ListTools()
	if err != nil {
		writeAPIError(w, http.StatusInternalServerError, err)
		return
	}

	list := make([]toolJSON, 0, len(tools))
	for _, tool := range tools {
		item := toolJSON{ID: tool.ID, Name: tool.Name, Kind: tool.Kind, Target: tool.Target, Tags: tool.Tags}
		item.Params = make([]paramJSON, 0, len(tool.Params))
		for _, param := range tool.Params {
			item.Params = append(item.Params, paramJSON{
				Name:     param.Name,
				Type:     param.Type,
				Required: param.Required,
				Default:  param.Default,
				Choices:  param.Choices,
			})
		}
		list = append(list, item)
	}
	writeJSON(w, http.StatusOK, list)
}

type runRequest struct {
	ID      string            `json:"id"`
	Params  map[string]string `json:"params"`
	RawArgs string            `json:"raw_args"`
}

func (s *Server) handleRun(w http.ResponseWriter, r *http.Request) {
	var req runRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(&req); err != nil {
		writeAPIError(w, http.StatusBadRequest, fmt.Errorf("JSON không hợp lệ: %w", err))
		return
	}

	runID, err := s.runner.RunTool(r.Context(), req.ID, req.Params, req.RawArgs)
	if err != nil {
		writeAPIError(w, http.StatusBadRequest, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"run_id": runID})
}

type answerRequest struct {
	RunID  string `json:"run_id"`
	Answer string `json:"answer"`
}

func (s *Server) handleAnswer(w http.ResponseWriter, r *http.Request) {
	var req answerRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(&req); err != nil {
		writeAPIError(w, http.StatusBadRequest, fmt.Errorf("JSON không hợp lệ: %w", err))
		return
	}
	if err := s.runner.Answer(req.RunID, req.Answer); err != nil {
		writeAPIError(w, http.StatusBadRequest, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

type installRequest struct {
	// Định dạng cũ: chỉ có nguồn cài đặt.
	Source string `json:"source"`

	// Định dạng form chi tiết: khai báo id/kind/target... để mt tự tạo app.toml.
	ID     string   `json:"id"`
	Name   string   `json:"name"`
	Kind   string   `json:"kind"`
	Target string   `json:"target"`
	Entry  string   `json:"entry"`
	Tags   []string `json:"tags"`
	Venv   *bool    `json:"venv"`
}

// handleAppAdd nhận nguồn cài đặt (định dạng cũ) hoặc payload form chi tiết,
// trả run_id để client theo dõi log cài đặt qua /api/events như một lần chạy
// thường. Với kind "url" chỉ ghi registry nên trả kết quả trực tiếp.
func (s *Server) handleAppAdd(w http.ResponseWriter, r *http.Request) {
	var req installRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(&req); err != nil {
		writeAPIError(w, http.StatusBadRequest, fmt.Errorf("JSON không hợp lệ: %w", err))
		return
	}

	if strings.TrimSpace(req.Source) != "" {
		runID, err := s.runner.InstallTool(r.Context(), req.Source)
		if err != nil {
			writeAPIError(w, http.StatusBadRequest, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{"run_id": runID})
		return
	}

	venv := req.Venv == nil || *req.Venv
	runID, err := s.runner.InstallCustomTool(r.Context(), req.ID, req.Name, req.Kind, req.Target, req.Entry, req.Tags, venv)
	if err != nil {
		writeAPIError(w, http.StatusBadRequest, err)
		return
	}
	if runID == "" {
		writeJSON(w, http.StatusOK, map[string]any{"ok": true, "text": fmt.Sprintf("Đã thêm tool %q.", req.Name)})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"run_id": runID})
}

type manualInstallRequest struct {
	Source string `json:"source"`
	Name   string `json:"name"`
	Entry  string `json:"entry"`
	GUI    bool   `json:"gui"`
}

// handleAppInstallManual nhận thông tin để sinh app.toml cho repo git thiếu
// file cấu hình; trả run_id để theo dõi log cài đặt qua /api/events.
func (s *Server) handleAppInstallManual(w http.ResponseWriter, r *http.Request) {
	var req manualInstallRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(&req); err != nil {
		writeAPIError(w, http.StatusBadRequest, fmt.Errorf("JSON không hợp lệ: %w", err))
		return
	}

	runID, err := s.runner.InstallManualTool(r.Context(), req.Source, req.Name, req.Entry, req.GUI)
	if err != nil {
		writeAPIError(w, http.StatusBadRequest, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"run_id": runID})
}

type removeRequest struct {
	ID string `json:"id"`
}

func (s *Server) handleAppRemove(w http.ResponseWriter, r *http.Request) {
	var req removeRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(&req); err != nil {
		writeAPIError(w, http.StatusBadRequest, fmt.Errorf("JSON không hợp lệ: %w", err))
		return
	}
	if err := s.runner.RemoveTool(req.ID); err != nil {
		writeAPIError(w, http.StatusBadRequest, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

// handleEvents phát SSE: phát lại lịch sử rồi chuyển event mới cho tới khi
// client ngắt kết nối (khi đó chỉ hủy đăng ký, tiến trình vẫn chạy tiếp).
func (s *Server) handleEvents(w http.ResponseWriter, r *http.Request) {
	runID := r.URL.Query().Get("runID")
	if runID == "" {
		writeAPIError(w, http.StatusBadRequest, fmt.Errorf("thiếu runID"))
		return
	}

	ch, unsubscribe, err := s.runner.Subscribe(runID)
	if err != nil {
		writeAPIError(w, http.StatusNotFound, err)
		return
	}
	defer unsubscribe()

	flusher, ok := w.(http.Flusher)
	if !ok {
		writeAPIError(w, http.StatusInternalServerError, fmt.Errorf("máy chủ không hỗ trợ SSE"))
		return
	}

	w.Header().Set("Content-Type", "text/event-stream; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Connection", "keep-alive")
	w.WriteHeader(http.StatusOK)
	flusher.Flush()

	for {
		select {
		case event, ok := <-ch:
			if !ok {
				_, _ = fmt.Fprintf(w, "event: closed\ndata: {}\n\n")
				flusher.Flush()
				return
			}
			writeSSE(w, event)
			flusher.Flush()
		case <-r.Context().Done():
			return
		}
	}
}

func writeSSE(w http.ResponseWriter, event core.Event) {
	data, err := json.Marshal(event)
	if err != nil {
		return
	}
	_, _ = fmt.Fprintf(w, "data: %s\n\n", data)
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

func writeAPIError(w http.ResponseWriter, status int, err error) {
	writeJSON(w, status, map[string]string{"error": err.Error()})
}
