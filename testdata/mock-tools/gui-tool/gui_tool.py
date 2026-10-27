"""Tool GUI mẫu: cửa sổ Tkinter tự đóng sau 2 giây."""

import tkinter as tk


def main():
    root = tk.Tk()
    root.title("Hello GUI")
    label = tk.Label(root, text="Hello GUI", font=("Segoe UI", 20))
    label.pack(padx=40, pady=40)
    root.after(2000, root.destroy)
    root.mainloop()


if __name__ == "__main__":
    main()
