"""Tool CLI mẫu: hỏi tên bằng input() rồi in lời chào."""

import sys

HELP = """SỬ DỤNG: cli-tool [tùy chọn]

Hỏi tên rồi in lời chào.

  --help           In trợ giúp này
  --greeting <câu> Lời chào tùy biến (mặc định: Xin chào)
  --shout          In hoa lời chào
  --no-color       Không tô màu đầu ra
"""


def main():
    if "--help" in sys.argv[1:] or "-h" in sys.argv[1:]:
        print(HELP)
        return

    name = input("Nhập tên của bạn: ")
    name = name.strip() or "bạn"

    greeting = "Xin chào"
    if "--greeting" in sys.argv[1:]:
        idx = sys.argv.index("--greeting")
        if idx + 1 < len(sys.argv):
            greeting = sys.argv[idx + 1]

    message = f"{greeting}, {name}!"
    if "--shout" in sys.argv[1:]:
        message = message.upper()
    print(message)
    sys.stdout.flush()


if __name__ == "__main__":
    main()
