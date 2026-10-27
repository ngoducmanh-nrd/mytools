package web

import (
	"context"
	"crypto/rand"
	"embed"
	"encoding/hex"
	"errors"
	"fmt"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"

	"mytools/internal/core"
)

//go:embed static
var staticFS embed.FS

// ToolRunner là phần service mà web cần; tách interface để test bằng giả lập.
type ToolRunner interface {
	ListTools() ([]core.Tool, error)
	RunTool(ctx context.Context, id string, params map[string]string) (string, error)
	Subscribe(runID string) (<-chan core.Event, func(), error)
	Answer(runID, answer string) error
}

// Server chỉ lắng nghe 127.0.0.1 và sinh token ngẫu nhiên mỗi lần khởi động;
// token được nhúng vào HTML trả về và bắt buộc cho mọi request POST.
type Server struct {
	port      int
	token     string
	runner    ToolRunner
	ln        net.Listener
	indexHTML []byte
	styleCSS  []byte
	appJS     []byte
}

func New(port int, runner ToolRunner) (*Server, error) {
	if runner == nil {
		return nil, errors.New("thiếu runner cho web server")
	}
	if port < 0 || port > 65535 {
		return nil, fmt.Errorf("port %d không hợp lệ (0-65535)", port)
	}

	token, err := randomToken()
	if err != nil {
		return nil, err
	}

	ln, err := net.Listen("tcp", net.JoinHostPort("127.0.0.1", strconv.Itoa(port)))
	if err != nil {
		return nil, fmt.Errorf("lắng nghe 127.0.0.1:%d: %w", port, err)
	}

	s := &Server{port: ln.Addr().(*net.TCPAddr).Port, token: token, runner: runner, ln: ln}

	index, err := staticFS.ReadFile("static/index.html")
	if err != nil {
		_ = ln.Close()
		return nil, fmt.Errorf("đọc static/index.html: %w", err)
	}
	s.indexHTML = []byte(strings.ReplaceAll(string(index), "__MT_TOKEN__", token))
	if s.styleCSS, err = staticFS.ReadFile("static/style.css"); err != nil {
		_ = ln.Close()
		return nil, fmt.Errorf("đọc static/style.css: %w", err)
	}
	if s.appJS, err = staticFS.ReadFile("static/app.js"); err != nil {
		_ = ln.Close()
		return nil, fmt.Errorf("đọc static/app.js: %w", err)
	}
	return s, nil
}

func (s *Server) Port() int {
	return s.port
}

func (s *Server) Token() string {
	return s.token
}

func (s *Server) Listener() net.Listener {
	return s.ln
}

// URL trả về địa chỉ kèm token để tiện mở bằng curl hoặc chia sẻ tạm thời.
func (s *Server) URL() string {
	return fmt.Sprintf("http://127.0.0.1:%d/?token=%s", s.port, s.token)
}

// Serve phục vụ cho tới khi ctx hủy (đóng sạch) hoặc lỗi thật sự.
func (s *Server) Serve(ctx context.Context) error {
	if ctx == nil {
		ctx = context.Background()
	}
	httpServer := &http.Server{Handler: s.Handler()}

	errCh := make(chan error, 1)
	go func() {
		errCh <- httpServer.Serve(s.ln)
	}()

	select {
	case err := <-errCh:
		return err
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		if err := httpServer.Shutdown(shutdownCtx); err != nil {
			_ = httpServer.Close()
		}
		err := <-errCh
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return err
	}
}

func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/", s.handleIndex)
	mux.HandleFunc("/style.css", s.handleStyle)
	mux.HandleFunc("/app.js", s.handleAppJS)
	mux.HandleFunc("/api/tools", s.handleTools)
	mux.HandleFunc("/api/run", s.handleRun)
	mux.HandleFunc("/api/events", s.handleEvents)
	mux.HandleFunc("/api/answer", s.handleAnswer)
	return s.security(mux)
}

// security chặn: Host không phải 127.0.0.1/localhost đúng port, Origin không
// khớp Host, POST thiếu hoặc sai X-MT-Token.
func (s *Server) security(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !s.validHost(r.Host) {
			http.Error(w, "từ chối: host không hợp lệ", http.StatusForbidden)
			return
		}
		if origin := r.Header.Get("Origin"); origin != "" && origin != "http://"+r.Host {
			http.Error(w, "từ chối: origin không khớp", http.StatusForbidden)
			return
		}
		if r.Method == http.MethodPost && r.Header.Get("X-MT-Token") != s.token {
			http.Error(w, "từ chối: thiếu hoặc sai token", http.StatusForbidden)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func (s *Server) validHost(host string) bool {
	hostname, port, err := net.SplitHostPort(host)
	if err != nil {
		return false
	}
	if hostname != "127.0.0.1" && hostname != "localhost" {
		return false
	}
	return port == strconv.Itoa(s.port)
}

func randomToken() (string, error) {
	var buf [16]byte
	if _, err := rand.Read(buf[:]); err != nil {
		return "", fmt.Errorf("tạo token bảo mật: %w", err)
	}
	return hex.EncodeToString(buf[:]), nil
}
