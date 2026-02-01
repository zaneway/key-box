#!/bin/bash

# Key-Box macOS 打包脚本
# 生成 .app 应用并打包成 .dmg 安装文件
# 支持 Intel (amd64) 和 Apple Silicon (arm64) 芯片

set -e

# 项目根目录
PROJECT_ROOT="$(cd "$(dirname "$0")/.." && pwd)"
APP_NAME="Key-Box"
APP_ID="com.keybox.app"
ICON_FILE="${PROJECT_ROOT}/key-box.png"
BUILD_DIR="${PROJECT_ROOT}/dist/macos"
DMG_TEMP_DIR="${BUILD_DIR}/dmg-temp"

echo "======================================"
echo "  Key-Box macOS 打包脚本"
echo "======================================"

# 检查 icon 文件是否存在
ICON_ABS_PATH="${PROJECT_ROOT}/key-box.png"
if [ ! -f "$ICON_ABS_PATH" ]; then
    echo "错误: 图标文件不存在: $ICON_ABS_PATH"
    exit 1
fi

# 检查 fyne 命令是否可用
if ! command -v fyne &> /dev/null; then
    echo "正在安装 fyne 命令行工具..."
    go install fyne.io/fyne/v2/cmd/fyne@latest
fi

# 检测当前系统架构
ARCH_NAME="$(uname -m)"
case "$ARCH_NAME" in
    arm64|aarch64)
        CURRENT_ARCH="arm64"
        CURRENT_ARCH_DESC="Apple Silicon (M1/M2/M3)"
        ;;
    x86_64|amd64|i386)
        CURRENT_ARCH="amd64"
        CURRENT_ARCH_DESC="Intel"
        ;;
    *)
        echo "警告: 未知架构 $ARCH_NAME"
        CURRENT_ARCH="amd64"
        CURRENT_ARCH_DESC="Unknown"
        ;;
esac

echo "检测到当前系统: $CURRENT_ARCH_DESC"

# 询问用户要构建的架构
echo ""
echo "请选择要构建的架构:"
echo "  1) Universal (推荐，同时支持 Intel 和 Apple Silicon)"
echo "  2) Apple Silicon (M1/M2/M3) - arm64"
echo "  3) Intel (x86_64) - amd64"
echo -n "请输入选项 [1-3，默认=1]: "
read -r ARCH_CHOICE
echo ""

case "$ARCH_CHOICE" in
    2)
        BUILD_ARCH="arm64"
        ARCH_SUFFIX="arm64"
        ;;
    3)
        BUILD_ARCH="amd64"
        ARCH_SUFFIX="x64"
        ;;
    *)
        BUILD_ARCH="universal"
        ARCH_SUFFIX="universal"
        ;;
esac

# 清理并创建构建目录
echo "清理构建目录..."
rm -rf "$BUILD_DIR"
mkdir -p "$BUILD_DIR"
mkdir -p "$DMG_TEMP_DIR"

# 构建 macOS 应用
echo "正在打包 macOS 应用 (${BUILD_ARCH})..."
cd "$PROJECT_ROOT"

case "$BUILD_ARCH" in
    universal)
        # Universal 需要分别构建两个架构，然后使用 lipo 合并
        echo "构建 Universal 二进制文件..."

        # 临时目录
        TEMP_DIR="${BUILD_DIR}/temp"
        mkdir -p "$TEMP_DIR"

        # 构建 arm64 版本
        echo "  - 构建 Apple Silicon 版本..."
        GOOS=darwin GOARCH=arm64 CGO_ENABLED=1 \
        fyne package \
            --target darwin \
            --source-dir cmd/gui \
            --name "$APP_NAME" \
            --icon "$ICON_FILE" \
            --app-id "$APP_ID" \
            --tags sqlite_unlock_notify \
            --release

        # 重命名为 arm64.app
        mv "${APP_NAME}.app" "${TEMP_DIR}/${APP_NAME}-arm64.app"

        # 构建 amd64 版本
        echo "  - 构建 Intel 版本..."
        GOOS=darwin GOARCH=amd64 CGO_ENABLED=1 \
        fyne package \
            --target darwin \
            --source-dir cmd/gui \
            --name "$APP_NAME" \
            --icon "$ICON_FILE" \
            --app-id "$APP_ID" \
            --tags sqlite_unlock_notify \
            --release

        # 重命名为 amd64.app
        mv "${APP_NAME}.app" "${TEMP_DIR}/${APP_NAME}-amd64.app"

        # 合并为 Universal 二进制
        echo "  - 合并为 Universal 二进制..."
        APP_TEMP="${TEMP_DIR}/${APP_NAME}-universal.app"
        cp -R "${TEMP_DIR}/${APP_NAME}-arm64.app" "$APP_TEMP"

        ARM64_BINARY="${TEMP_DIR}/${APP_NAME}-arm64.app/Contents/MacOS/gui"
        AMD64_BINARY="${TEMP_DIR}/${APP_NAME}-amd64.app/Contents/MacOS/gui"
        UNIVERSAL_BINARY="${APP_TEMP}/Contents/MacOS/gui"

        if [ -f "$ARM64_BINARY" ] && [ -f "$AMD64_BINARY" ]; then
            lipo -create "$ARM64_BINARY" "$AMD64_BINARY" -output "$UNIVERSAL_BINARY"
            mv "$APP_TEMP" "${APP_NAME}.app"
            rm -rf "${TEMP_DIR}/${APP_NAME}-arm64.app"
            rm -rf "${TEMP_DIR}/${APP_NAME}-amd64.app"
            rmdir "$TEMP_DIR" 2>/dev/null || true
            echo "  ✓ Universal 二进制创建成功"
        else
            echo "警告: 二进制文件缺失，使用 arm64 版本"
            mv "${TEMP_DIR}/${APP_NAME}-arm64.app" "${APP_NAME}.app"
            rm -rf "${TEMP_DIR}"
        fi
        ;;
    arm64|amd64)
        GOOS=darwin GOARCH="$BUILD_ARCH" CGO_ENABLED=1 \
        fyne package \
            --target darwin \
            --source-dir cmd/gui \
            --name "$APP_NAME" \
            --icon "$ICON_FILE" \
            --app-id "$APP_ID" \
            --tags sqlite_unlock_notify \
            --release
        ;;
esac

# 移动生成的 .app 到构建目录
if [ -d "${APP_NAME}.app" ]; then
    mv "${APP_NAME}.app" "$BUILD_DIR/"
    echo "已生成: $BUILD_DIR/$APP_NAME.app"

    # 显示架构信息（可执行文件名是 gui，不是 Key-Box）
    BINARY_ARCH=$(file "$BUILD_DIR/$APP_NAME.app/Contents/MacOS/gui" 2>/dev/null | grep -o 'arm64\|x86_64' | head -1)
    echo "二进制架构: $BINARY_ARCH"
else
    echo "错误: 未找到生成的 .app 目录"
    echo "当前目录文件:"
    ls -la
    exit 1
fi

# 创建 .dmg 文件
echo "正在创建 .dmg 安装文件..."

DMG_NAME="${APP_NAME}-Installer-${ARCH_SUFFIX}"
DMG_PATH="${BUILD_DIR}/${DMG_NAME}.dmg"

# 方法 1: 使用 create-dmg 工具（推荐，支持自定义布局）
if command -v create-dmg &> /dev/null; then
    echo "使用 create-dmg 创建 DMG..."
    create-dmg \
        --volname "$APP_NAME" \
        --window-pos 200 200 \
        --window-size 800 400 \
        --app-drop-link 600 200 \
        --icon-size 128 \
        "$DMG_PATH" \
        "$BUILD_DIR/$APP_NAME.app"
    echo "  ✓ DMG 创建成功（带 Applications 快捷方式）"
else
    # 方法 2: 使用 hdiutil 手动创建
    echo "create-dmg 未安装，使用 hdiutil 创建 DMG..."

    # 复制应用到临时目录
    cp -R "$BUILD_DIR/$APP_NAME.app" "$DMG_TEMP_DIR/"

    # 创建 Applications 文件夹链接
    ln -s /Applications "$DMG_TEMP_DIR/Applications"

    # 创建自定义背景（可选）
    # echo "创建 DMG 背景..."

    # 创建 dmg
    hdiutil create -volname "$APP_NAME" \
        -srcfolder "$DMG_TEMP_DIR" \
        -ov \
        -format UDZO \
        "$DMG_PATH"

    # 清理临时目录
    rm -rf "$DMG_TEMP_DIR"
    echo "  ✓ DMG 创建成功"
fi

# 验证生成的文件
if [ -f "$DMG_PATH" ]; then
    DMG_SIZE=$(ls -lh "$DMG_PATH" | awk '{print $5}')
    echo ""
    echo "======================================"
    echo "  打包完成！"
    echo "======================================"
    echo "DMG 文件: $DMG_PATH"
    echo "文件大小: $DMG_SIZE"
    echo "架构: ${BUILD_ARCH}"
    echo ""
    echo "安装说明:"
    echo "1. 双击打开 ${DMG_NAME}.dmg"
    echo "2. 将 ${APP_NAME}.app 拖拽到 Applications 文件夹"
    echo ""
    echo "提示: 如需安装 create-dmg 工具以获得更好的 DMG 效果:"
    echo "  brew install create-dmg"
    echo ""
else
    echo "错误: DMG 创建失败"
    exit 1
fi
