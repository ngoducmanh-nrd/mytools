package appmgr

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/pelletier/go-toml/v2"

	"mytools/internal/core"
)

const (
	appsDirName = "apps"
	appSpecName = "app.toml"
	stateName   = "state.json"
)

var appIDPattern = regexp.MustCompile(`^[a-z][a-z0-9_-]{0,31}$`)

type Manager struct {
	dataDir string
}

func New(dataDir string) *Manager {
	return &Manager{dataDir: dataDir}
}

func BuiltinSpec(id string) (core.AppSpec, bool) {
	switch id {
	case "yt-dlp":
		return core.AppSpec{
			SchemaVersion: 1,
			ID:            "yt-dlp",
			Runtime:       "binary",
			Download:      "https://github.com/yt-dlp/yt-dlp/releases/latest/download/yt-dlp.exe",
			Entry:         []string{"yt-dlp.exe"},
			Update:        []string{"yt-dlp.exe", "-U"},
		}, true
	default:
		return core.AppSpec{}, false
	}
}

func (m *Manager) List() ([]core.App, error) {
	appsDir, err := m.appsDir()
	if err != nil {
		return nil, err
	}

	entries, err := os.ReadDir(appsDir)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil, nil
		}
		return nil, fmt.Errorf("đọc thư mục app %s: %w", appsDir, err)
	}

	apps := make([]core.App, 0, len(entries))
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		app, err := m.LoadApp(entry.Name())
		if err != nil {
			return nil, err
		}
		apps = append(apps, *app)
	}
	return apps, nil
}

func (m *Manager) LoadApp(id string) (*core.App, error) {
	dir, err := m.existingAppDir(id)
	if err != nil {
		return nil, err
	}

	spec, err := loadAppSpec(filepath.Join(dir, appSpecName))
	if err != nil {
		return nil, fmt.Errorf("app %q: %w", id, err)
	}
	if spec.ID != id {
		return nil, fmt.Errorf("app %q: app.toml khai báo id %q", id, spec.ID)
	}

	state, err := loadState(filepath.Join(dir, stateName))
	if err != nil {
		return nil, fmt.Errorf("app %q: %w", id, err)
	}

	return &core.App{AppSpec: *spec, AppState: state, Dir: dir}, nil
}

func (m *Manager) Add(spec core.AppSpec) (*core.App, error) {
	if err := validateSpec(spec); err != nil {
		return nil, err
	}
	if spec.Runtime != "binary" {
		return nil, fmt.Errorf("runtime %q chưa hỗ trợ ở P2 pass 1 (chỉ nhận %q)", spec.Runtime, "binary")
	}

	dir, err := m.appDir(spec.ID)
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(filepath.Dir(dir), 0o700); err != nil {
		return nil, fmt.Errorf("tạo thư mục app: %w", err)
	}
	if err := os.Mkdir(dir, 0o700); err != nil {
		if errors.Is(err, fs.ErrExist) {
			return nil, fmt.Errorf("app %q đã tồn tại", spec.ID)
		}
		return nil, fmt.Errorf("tạo thư mục app %q: %w", spec.ID, err)
	}

	if err := writeAppSpec(dir, spec); err != nil {
		if cleanupErr := os.RemoveAll(dir); cleanupErr != nil {
			return nil, fmt.Errorf("%w (dọn thư mục lỗi: %v)", err, cleanupErr)
		}
		return nil, err
	}

	return &core.App{AppSpec: spec, Dir: dir}, nil
}

func (m *Manager) SaveState(id string, state core.AppState) error {
	dir, err := m.existingAppDir(id)
	if err != nil {
		return err
	}

	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return fmt.Errorf("mã hóa state của app %q: %w", id, err)
	}
	data = append(data, '\n')

	path := filepath.Join(dir, stateName)
	if err := writeFileAtomic(path, data, 0o644); err != nil {
		return fmt.Errorf("ghi %s: %w", path, err)
	}
	return nil
}

func (m *Manager) Remove(id string) error {
	dir, err := m.existingAppDir(id)
	if err != nil {
		return err
	}
	if err := os.RemoveAll(dir); err != nil {
		return fmt.Errorf("xóa app %q: %w", id, err)
	}
	return nil
}

func (m *Manager) appsDir() (string, error) {
	if strings.TrimSpace(m.dataDir) == "" {
		return "", errors.New("thư mục dữ liệu app rỗng")
	}
	abs, err := filepath.Abs(m.dataDir)
	if err != nil {
		return "", fmt.Errorf("xác định đường dẫn dữ liệu: %w", err)
	}
	return filepath.Join(abs, appsDirName), nil
}

func (m *Manager) appDir(id string) (string, error) {
	if err := validateID(id); err != nil {
		return "", err
	}

	appsDir, err := m.appsDir()
	if err != nil {
		return "", err
	}
	dir := filepath.Join(appsDir, id)
	absDir, err := filepath.Abs(dir)
	if err != nil {
		return "", fmt.Errorf("xác định đường dẫn app %q: %w", id, err)
	}
	if filepath.Dir(absDir) != appsDir {
		return "", fmt.Errorf("từ chối đường dẫn app %q nằm ngoài %s", id, appsDir)
	}
	return absDir, nil
}

func (m *Manager) existingAppDir(id string) (string, error) {
	dir, err := m.appDir(id)
	if err != nil {
		return "", err
	}

	info, err := os.Lstat(dir)
	switch {
	case errors.Is(err, fs.ErrNotExist):
		return "", fmt.Errorf("app %q chưa cài", id)
	case err != nil:
		return "", fmt.Errorf("kiểm tra app %q: %w", id, err)
	case info.Mode()&os.ModeSymlink != 0:
		return "", fmt.Errorf("từ chối dùng app %q: đường dẫn là liên kết", id)
	case !info.IsDir():
		return "", fmt.Errorf("từ chối dùng app %q: không phải thư mục", id)
	}
	return dir, nil
}

func validateID(id string) error {
	if id == "" {
		return errors.New("id app không được để trống")
	}
	if !appIDPattern.MatchString(id) {
		return fmt.Errorf("id app %q không hợp lệ (chỉ nhận a-z, 0-9, \"-\", \"_\"; bắt đầu bằng chữ cái, tối đa 32 ký tự)", id)
	}
	return nil
}

func validateSpec(spec core.AppSpec) error {
	if spec.SchemaVersion != 1 {
		return fmt.Errorf("schema_version: phải bằng 1 (đang là %d)", spec.SchemaVersion)
	}
	if err := validateID(spec.ID); err != nil {
		return fmt.Errorf("id: %w", err)
	}
	if spec.Runtime == "" {
		return errors.New(`runtime: thiếu trường bắt buộc "runtime"`)
	}

	switch spec.Runtime {
	case "binary":
		if spec.Download == "" {
			return errors.New(`download: thiếu trường bắt buộc "download"`)
		}
		if err := validateDownloadURL(spec.Download); err != nil {
			return fmt.Errorf("download: %w", err)
		}
		if len(spec.Entry) == 0 {
			return errors.New(`entry: thiếu trường bắt buộc "entry"`)
		}
		if err := validateEntryFile(spec.Entry[0]); err != nil {
			return fmt.Errorf("entry[0]: %w", err)
		}
	case "python":
		if spec.Source == "" {
			return errors.New(`source: thiếu trường bắt buộc "source"`)
		}
		if len(spec.Entry) == 0 {
			return errors.New(`entry: thiếu trường bắt buộc "entry"`)
		}
	default:
		return fmt.Errorf("runtime %q không hợp lệ (chỉ nhận binary|python)", spec.Runtime)
	}
	return nil
}

func validateDownloadURL(rawURL string) error {
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return fmt.Errorf("URL không hợp lệ: %w", err)
	}
	if (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" {
		return errors.New("phải là URL http/https có host")
	}
	return nil
}

func validateEntryFile(name string) error {
	if name == "" || name == "." || name == ".." || filepath.Base(name) != name || strings.ContainsAny(name, `/\`) {
		return fmt.Errorf("%q phải là tên file trong thư mục app, không chứa đường dẫn", name)
	}
	return nil
}

type appTOML struct {
	SchemaVersion int      `toml:"schema_version"`
	ID            string   `toml:"id"`
	Runtime       string   `toml:"runtime"`
	Download      string   `toml:"download,omitempty"`
	Source        string   `toml:"source,omitempty"`
	Ref           string   `toml:"ref,omitempty"`
	Python        string   `toml:"python,omitempty"`
	Requirements  string   `toml:"requirements,omitempty"`
	Entry         []string `toml:"entry,omitempty"`
	Update        []string `toml:"update,omitempty"`
	GUI           bool     `toml:"gui,omitempty"`
}

func writeAppSpec(dir string, spec core.AppSpec) error {
	file := appTOML{
		SchemaVersion: spec.SchemaVersion,
		ID:            spec.ID,
		Runtime:       spec.Runtime,
		Download:      spec.Download,
		Source:        spec.Source,
		Ref:           spec.Ref,
		Python:        spec.Python,
		Requirements:  spec.Requirements,
		Entry:         spec.Entry,
		Update:        spec.Update,
		GUI:           spec.GUI,
	}
	data, err := toml.Marshal(file)
	if err != nil {
		return fmt.Errorf("mã hóa app.toml: %w", err)
	}
	if len(data) == 0 || data[len(data)-1] != '\n' {
		data = append(data, '\n')
	}

	path := filepath.Join(dir, appSpecName)
	if err := writeFileAtomic(path, data, 0o644); err != nil {
		return fmt.Errorf("ghi %s: %w", path, err)
	}
	return nil
}

func loadAppSpec(path string) (*core.AppSpec, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil, fmt.Errorf("%s: %w", filepath.Base(path), core.ErrAppConfigMissing)
		}
		return nil, fmt.Errorf("đọc %s: %w", path, err)
	}

	var spec core.AppSpec
	if err := toml.Unmarshal(data, &spec); err != nil {
		return nil, appDecodeError(filepath.Base(path), err)
	}
	if err := validateSpec(spec); err != nil {
		return nil, fmt.Errorf("%s: %w", filepath.Base(path), err)
	}
	return &spec, nil
}

func appDecodeError(file string, err error) error {
	var decodeErr *toml.DecodeError
	if !errors.As(err, &decodeErr) {
		return fmt.Errorf("%s: %w", file, err)
	}

	line, _ := decodeErr.Position()
	field := strings.Join(decodeErr.Key(), ".")
	if field == "" {
		field = "toml"
	}
	return fmt.Errorf("%s:%d: %s: %s", file, line, field, strings.TrimPrefix(decodeErr.Error(), "toml: "))
}

func loadState(path string) (core.AppState, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return core.AppState{}, nil
		}
		return core.AppState{}, fmt.Errorf("đọc %s: %w", path, err)
	}

	var state core.AppState
	if err := json.Unmarshal(data, &state); err != nil {
		return core.AppState{}, fmt.Errorf("%s: JSON không hợp lệ: %w", filepath.Base(path), err)
	}
	return state, nil
}

func writeFileAtomic(path string, data []byte, perm fs.FileMode) error {
	tmpPath := path + ".tmp"
	file, err := os.OpenFile(tmpPath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, perm)
	if err != nil {
		return err
	}

	success := false
	closed := false
	defer func() {
		if success {
			return
		}
		if !closed {
			_ = file.Close()
		}
		_ = os.Remove(tmpPath)
	}()

	if _, err := file.Write(data); err != nil {
		return err
	}
	if err := file.Sync(); err != nil {
		return err
	}
	if err := file.Close(); err != nil {
		return err
	}
	closed = true

	if err := os.Rename(tmpPath, path); err != nil {
		return err
	}
	success = true
	return nil
}
