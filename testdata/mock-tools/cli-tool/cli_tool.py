"""Tool CLI mẫu: hỏi tên bằng input() rồi in lời chào."""

import sys


def main():
    name = input("Nhập tên của bạn: ")
    name = name.strip() or "bạn"
    print(f"Xin chào, {name}!")
    sys.stdout.flush()


if __name__ == "__main__":
    main()
