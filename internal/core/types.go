package core

import (
	"context"
	"errors"
	"os/exec"
	"time"
)

type Event struct {
	RunID string         `json:"run_id"`
	Kind  string         `json:"kind"`
	Text  string         `json:"text"`
	Pct   float64        `json:"pct,omitempty"`
	Data  map[string]any `json:"data,omitempty"`
}

type Param struct {
	Name     string   `toml:"name"`
	Type     string   `toml:"type"`
	Required bool     `toml:"required"`
	Default  any      `toml:"default"`
	Choices  []string `toml:"choices"`
}

type Tool struct {
	ID        string   `toml:"id"`
	Name      string   `toml:"name"`
	Kind      string   `toml:"kind"`
	Target    string   `toml:"target"`
	App       string   `toml:"app"`
	Command   string   `toml:"command"`
	Args      []string `toml:"args"`
	Tags      []string `toml:"tags"`
	Params    []Param  `toml:"param"`
	Lifecycle string   `toml:"lifecycle"`
}

type AppSpec struct {
	SchemaVersion int      `toml:"schema_version"`
	ID            string   `toml:"id"`
	Runtime       string   `toml:"runtime"`
	Download      string   `toml:"download"`
	Source        string   `toml:"source"`
	Ref           string   `toml:"ref"`
	Python        string   `toml:"python"`
	Requirements  string   `toml:"requirements"`
	Entry         []string `toml:"entry"`
	Update        []string `toml:"update"`
	GUI           bool     `toml:"gui"`
}

type AppState struct {
	URL              string    `json:"url"`
	Sha256           string    `json:"sha256"`
	DownloadedAt     time.Time `json:"downloaded_at"`
	Version          string    `json:"version"`
	TrustedCommit    string    `json:"trusted_commit"`
	RequirementsHash string    `json:"requirements_hash,omitempty"`
}

type App struct {
	AppSpec
	AppState
	Dir string `json:"-" toml:"-"`
}

type Check struct {
	Name    string
	Ok      bool
	Message string
	Hint    string
}

type Service interface {
	ListTools(ctx context.Context) ([]Tool, error)
	RunTool(ctx context.Context, id string, params map[string]string) (runID string, events <-chan Event, err error)
	Answer(runID string, answer string) error
	Cancel(runID string) error
	ListApps(ctx context.Context) ([]App, error)
	AddApp(ctx context.Context, spec AppSpec) (<-chan Event, error)
	UpdateApp(ctx context.Context, id string) (<-chan Event, error)
	RemoveApp(ctx context.Context, id string) (<-chan Event, error)
	Doctor(ctx context.Context) []Check
}

type Runtime interface {
	Name() string
	Install(ctx context.Context, app App, out chan<- Event) error
	Command(app App, args []string) (*exec.Cmd, error)
	Remove(app App) error
}

var ErrAppConfigMissing = errors.New("repo không có app.toml")
