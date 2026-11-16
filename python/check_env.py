import sys
import platform
import pathlib


def main():
    print(f"Python 版本: {sys.version}")
    print(f"Python 解释器路径: {sys.executable}")
    print(f"系统架构: {platform.machine()}")
    print(f"当前工作目录: {pathlib.Path().absolute()}")
    print(f"Python 环境根目录: {sys.prefix}")
    print(f"Python 基础根目录: {sys.base_prefix}")
    print(f"是否为虚拟环境: {sys.prefix != sys.base_prefix}")

    print(["A", "B", "C", "D"][::-1])

if __name__ == "__main__":
    main()
