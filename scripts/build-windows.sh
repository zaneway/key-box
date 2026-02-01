#!/bin/bash

# Key-Box Windows 打包脚本 (在 Linux/macOS 上交叉编译)
# 生成 .exe 安装文件

set -e

# 项目根目录
PROJECT_ROOT="$(cd "$(dirname "$0")/.." && pwd)"
APP_NAME="Key-Box"
APP_ID="com.keybox.app"
ICON_FILE="${PROJECT_ROOT}/key-box.png"
BUILD_DIR="${PROJECT_ROOT}/dist/windows"

echo "======================================"
echo "  Key-Box Windows 打包脚本"
echo "======================================"

# 检查 icon 文件是否存在
if [ ! -f "$ICON_FILE" ]; then
    echo "错误: 图标文件不存在: $ICON_FILE"
    exit 1
fi

# 检查 fyne 命令是否可用
if ! command -v fyne &> /dev/null; then
    echo "正在安装 fyne 命令行工具..."
    go install fyne.io/fyne/v2/cmd/fyne@latest
fi

# 清理并创建构建目录
echo "清理构建目录..."
rm -rf "$BUILD_DIR"
mkdir -p "$BUILD_DIR"

# 使用 fyne package 打包 Windows 应用 (需要 Windows 特定的图标文件)
echo "注意: Windows 需要专门的 .ico 图标文件"
echo "正在尝试使用 PNG 图标打包..."

# Fyne 需要的 Windows 工具
# 检查是否安装了 mingw-w64 (用于 ico 编译)
if ! command -v convert &> /dev/null; then
    echo "警告: ImageMagick 未安装，无法将 PNG 转换为 ICO"
    echo "请手动提供 .ico 格式的图标文件"
fi

# 使用 fyne package 打包 Windows 应用
echo "正在打包 Windows 应用..."
cd "$PROJECT_ROOT"

# 尝试使用 PNG 图标（Fyne 会自动转换，但需要特定工具）
GOOS=windows GOARCH=amd64 CGO_ENABLED=1 \
fyne package \
    --target windows \
    --src cmd/gui \
    --name "$APP_NAME" \
    --icon "$ICON_FILE" \
    --app-id "$APP_ID" \
    --tags sqlite_unlock_notify \
    --release || {
    echo "打包失败，请确保已安装必要的工具"
    echo "在 Ubuntu/Debian 上需要: sudo apt-get install gcc-mingw-w64"
    echo "在 macOS 上需要: brew install mingw-w64"
    exit 1
}

# 移动生成的文件
if [ -f "$APP_NAME.exe" ]; then
    mv "$APP_NAME.exe" "$BUILD_DIR/"
    echo "已生成: $BUILD_DIR/$APP_NAME.exe"

    # 创建安装包目录
    INSTALL_DIR="${BUILD_DIR}/installer"
    mkdir -p "$INSTALL_DIR"

    # 复制 exe 到安装目录
    cp "$BUILD_DIR/$APP_NAME.exe" "$INSTALL_DIR/"

    # 创建安装说明文件
    cat > "${INSTALL_DIR}/README.txt" << 'EOF'
=======================================
      Key-Box 安装说明
=======================================

1. 将 Key-Box.exe 复制到您想要安装的目录
   (例如: C:\Program Files\Key-Box)

2. 运行 Key-Box.exe 即可启动应用

3. 如需创建桌面快捷方式:
   - 右键点击 Key-Box.exe
   - 选择"发送到" -> "桌面快捷方式"

4. 如需添加到开始菜单:
   - 将快捷方式复制到:
     C:\Users\[您的用户名]\AppData\Roaming\Microsoft\Windows\Start Menu\Programs

=======================================
EOF

    # 创建压缩安装包
    cd "$BUILD_DIR"
    ZIP_NAME="${APP_NAME}-Windows-Installer.zip"
    zip -r "$ZIP_NAME" installer/ > /dev/null

    echo ""
    echo "======================================"
    echo "  打包完成！"
    echo "======================================"
    echo "EXE 文件: $BUILD_DIR/$APP_NAME.exe"
    echo "安装包:   $BUILD_DIR/$ZIP_NAME"
    echo ""
else
    echo "错误: 未找到生成的 .exe 文件"
    exit 1
fi
