# mt — Hub quản lý tool cá nhân

`mt` gom các tool hay dùng vào một chỗ: mở website, chạy tool dòng lệnh, chạy tool đồ họa, cài và quản lý app từ git. Cùng một lõi, hai giao diện: CLI và web local. Viết bằng Go, đóng gói thành **một file exe duy nhất**, không cần cài runtime.

## Tính năng

- Danh sách tool khai báo bằng `tools.toml` — thêm tool mới chỉ cần sửa file cấu hình, không build lại.
- Chạy tool dòng lệnh với log và tiến trình realtime.
- Quản lý app: tải binary (yt-dlp), clone git + môi trường Python/Node riêng, cập nhật, xóa sạch.
- Theo dõi tool sống (cửa sổ GUI, service nền): `ps`, `stop`, `restart`, `logs`.
- Giao diện web local `mt web` — form tự sinh theo tham số của tool, log/tiến trình đẩy qua SSE, bảo vệ bằng token + kiểm tra Host/Origin (chỉ nghe `127.0.0.1`).
- Chế độ portable: dữ liệu nằm cạnh file exe, tiện mang theo USB.
- `mt self-update`: tự tải bản mới từ GitHub Releases.

## Cài đặt

Tải bản build sẵn tại trang Releases:

**https://github.com/ngoducmanh-nrd/mytools/releases/latest**

| Hệ điều hành | File |
| --- | --- |
| Windows 64-bit | `mt-windows-amd64.exe` |
| Linux 64-bit | `mt-linux-amd64` |
| macOS Intel | `mt-darwin-amd64` |
| macOS Apple Silicon | `mt-darwin-arm64` |

Hoặc build từ mã nguồn (cần Go 1.22+):

```bash
go build -ldflags "-s -w" -trimpath -o mt.exe ./cmd/mt
```

Lần chạy đầu, `mt` tự tạo thư mục dữ liệu `~/.mytools/` gồm `config.toml` và `tools.toml` mẫu.

## Lệnh cơ bản

```bash
mt list                      # danh sách tool
mt open movies               # mở tool web bằng trình duyệt mặc định
mt run dl --url <link>       # chạy tool theo id (ví dụ tải video bằng yt-dlp)
mt web                       # mở giao diện web local (mặc định http://127.0.0.1:7878)
mt app add yt-dlp            # cài app dựng sẵn (hoặc: mt app add <url-git>)
mt app list                  # danh sách app đã cài
mt ps                        # tool đang chạy
mt stop <id>                 # dừng tool đang chạy
mt logs <id> -f              # xem log, -f để theo dõi tiếp
mt doctor                    # kiểm tra Git, Python, ffmpeg
mt self-update               # cập nhật lên bản mới nhất
mt version                   # phiên bản
```

Gõ `mt` không tham số: mặc định in trợ giúp; nếu `default_ui = "web"` trong `config.toml` thì mở web server và trình duyệt.

## Cấu hình

Thư mục dữ liệu: `~/.mytools/`

- `config.toml` — `default_ui`, `web_port`, `python`, `keep_logs_days`, `update_check_url`.
- `tools.toml` — khai báo tool (kind `url`, `process`, `app`, `native`). Tool `app` khai báo `param` để CLI tự sinh cờ `--tên` và web tự sinh form.
- `apps/<id>/` — mỗi app một thư mục riêng (app.toml, state.json, repo, env).
- `logs/<app>/<ngày>.log` — log từng app.
- `run/<id>.json` — PID tool đang chạy.

## Chế độ Portable

Tạo một file rỗng tên `portable.txt` cạnh `mt.exe`:

```
D:\tools\mt\
  mt.exe
  portable.txt     ← thêm file này
```

Khi có `portable.txt`, toàn bộ dữ liệu (`config.toml`, `tools.toml`, `apps/`, `logs/`, `run/`) được tạo ngay trong thư mục chứa exe thay vì `~/.mytools`. Xóa `portable.txt` để quay lại dùng thư mục người dùng.

## Phát triển

```bash
go test ./...                # chạy test toàn bộ
go vet ./...
GOOS=linux go build ./...    # kiểm tra build chéo
```

Phát hành: đẩy tag `v*` (ví dụ `git tag v0.1.0 && git push origin v0.1.0`), GitHub Actions tự build cho Windows/Linux/macOS và tạo Release kèm các file chạy được.
