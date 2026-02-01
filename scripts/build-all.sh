#!/bin/bash

# Key-Box 全平台打包脚本
# 同时打包 macOS 和 Windows 版本

set -e

# 项目根目录
SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
PROJECT_ROOT="$(dirname "$SCRIPT_DIR")"

echo "======================================"
echo "  Key-Box 全平台打包脚本"
echo "======================================"
echo ""

# 检测当前操作系统
OS="$(uname -s)"

case "$OS" in
    Darwin)
        echo "检测到 macOS 系统"
        echo "正在打包 macOS 应用..."
        bash "$SCRIPT_DIR/build-macos.sh"
        echo ""
        echo "注意: Windows 打包需要在 Windows 系统上运行，或使用交叉编译"
        echo "如需打包 Windows 版本，请运行: bash $SCRIPT_DIR/build-windows.sh"
        ;;

    Linux)
        echo "检测到 Linux 系统"
        echo "Linux 系统主要用于交叉编译 Windows 应用"
        echo ""
        echo "正在打包 Windows 应用..."
        bash "$SCRIPT_DIR/build-windows.sh"
        echo ""
        echo "注意: macOS 打包需要在 macOS 系统上运行"
        ;;

    MINGW*|MSYS*|CYGWIN*)
        echo "检测到 Windows 系统"
        echo "正在打包 Windows 应用..."
        cmd.exe /c "$SCRIPT_DIR\\build-windows.bat"
        echo ""
        echo "注意: macOS 打包需要在 macOS 系统上运行"
        ;;

    *)
        echo "未知操作系统: $OS"
        exit 1
        ;;
esac

echo ""
echo "======================================"
echo "  全部打包任务完成！"
echo "======================================"
echo "输出目录: $PROJECT_ROOT/dist"
echo ""
