# mytools (mt) — Agent Instructions

Go 1.22. Hướng dẫn người dùng: `README.md` (tiếng Việt).

## Lệnh (chạy tại root repo, bash)

```bash
go test ./...
go vet ./...
go build -trimpath -ldflags "-s -w" -o mt.exe ./cmd/mt
GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build ./...
```

CI `.github/workflows/release.yml` chỉ build asset khi push tag `v*`, không chạy test/vet — chạy tay trước khi phát hành.

## Phát hành

- Tăng `Version` trong `internal/ui/cli/version.go` cho khớp tag sắp push; self-update so sánh hằng số này với tag GitHub mới nhất và workflow không inject version.
- Push tag `v*` để workflow build Windows/Linux/macOS (`CGO_ENABLED=0`) rồi tạo GitHub Release.
- Không push, tag hay tạo release khi chưa được người dùng đồng ý rõ ràng.

## Ranh giới

- `internal/ui/web/server.go` chỉ nghe `127.0.0.1`, sinh token mỗi lần khởi động và chặn Host/Origin/`X-MT-Token` trong `Server.security`; mọi route POST/DELETE phải đi qua middleware này.
- Runtime app tách biệt: venv Python tại `<app>/env`, cache pip/npm chuyển vào `app.CacheDir`; `requirements` phải nằm trong thư mục app.
- App Electron GUI phải bỏ biến `ELECTRON_RUN_AS_NODE` (`runner.NodeCommand`); tool dùng `npx` phải qua xác nhận tin cậy (`runner.UsesNpx`). Không bỏ hai cơ chế này khi sửa install/run Node.
- Self-update trên Windows chạy PowerShell ẩn và `Stop-Process -Name mt -Force` (diệt mọi tiến trình mt) trước khi thay exe; thay đổi phần này là thao tác phá huỷ.
- Code theo build tag có cặp đôi: `internal/appmgr/process_{windows,unix}.go`, `internal/runner/process_{windows,unix}.go`, `internal/ui/web/browser_{windows,unix}.go` — sửa một bên phải sửa bên còn lại.

## Schema tools.toml

- `schema_version = 1` được enforce cho `config.toml` và `tools.toml`; id tool khớp `^[a-z][a-z0-9_-]{0,31}$` và không nằm trong `registry.ForbiddenIDs`.
- Khi đổi validate/ghi file, cập nhật đồng bộ `internal/registry/loader.go`, `write.go`, test và fixture `testdata/tools_*.toml`.

## Quy ước

- Text hướng người dùng (CLI help, lỗi, log, README) bằng tiếng Việt.
- Test phải hermetic: dùng `t.TempDir()`, không ghi vào `~/.mytools` hay `downloads/` của repo.
