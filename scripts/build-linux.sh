#!/bin/bash
# Key-Box Linux 打包脚本
# 用于打包 GUI 和 CLI 版本为 Debian/Ubuntu .deb 包

set -e

# 脚本所在目录
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_DIR="$(dirname "$SCRIPT_DIR")"
BUILD_DIR="$PROJECT_DIR/build"
DIST_DIR="$BUILD_DIR/dist"
VERSION="1.0.0"
APP_NAME="key-box"
DISPLAY_NAME="Key-Box"

# 颜色输出
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
RED='\033[0;31m'
NC='\033[0m' # No Color

echo -e "${GREEN}==================================${NC}"
echo -e "${GREEN}Key-Box Linux 打包脚本${NC}"
echo -e "${GREEN}==================================${NC}"
echo ""

# 检查必要工具
check_tool() {
    if ! command -v $1 &> /dev/null; then
        echo -e "${RED}错误: 未找到 $1，请先安装${NC}"
        return 1
    fi
    return 0
}

# 检查必要工具
echo -e "${YELLOW}[1/7] 检查环境...${NC}"
if ! check_tool go || ! check_tool dpkg-deb; then
    echo -e "${YELLOW}提示: dpkg-deb 未找到，将创建 tar.gz 包${NC}"
    CREATE_DEB=0
else
    CREATE_DEB=1
fi
echo -e "${GREEN}✓ 环境检查通过${NC}"
echo ""

# 清理旧的构建文件
echo -e "${YELLOW}[2/7] 清理旧的构建文件...${NC}"
rm -rf "$BUILD_DIR"
mkdir -p "$DIST_DIR"
echo -e "${GREEN}✓ 清理完成${NC}"
echo ""

# 编译 GUI 版本 (Linux)
echo -e "${YELLOW}[3/7] 编译 Linux GUI 版本...${NC}"
cd "$PROJECT_DIR"
GOOS=linux GOARCH=amd64 go build -ldflags="-s -w" -o "$BUILD_DIR/key-box-gui" ./cmd/gui
chmod +x "$BUILD_DIR/key-box-gui"
echo -e "${GREEN}✓ Linux GUI 编译完成${NC}"
echo ""

# 编译 CLI 版本 (Linux)
echo -e "${YELLOW}[4/7] 编译 Linux CLI 版本...${NC}"
GOOS=linux GOARCH=amd64 go build -ldflags="-s -w" -o "$BUILD_DIR/key-box-cli" ./cmd/client
chmod +x "$BUILD_DIR/key-box-cli"
echo -e "${GREEN}✓ Linux CLI 编译完成${NC}"
echo ""

# 创建 .deb 包
echo -e "${YELLOW}[5/7] 创建 .deb 包...${NC}"
if [ $CREATE_DEB -eq 1 ]; then
    # GUI .deb 包
    DEB_GUI_DIR="$BUILD_DIR/key-box-gui-deb"
    mkdir -p "$DEB_GUI_DIR"

    # 创建 Debian 控制文件
    mkdir -p "$DEB_GUI_DIR/DEBIAN"
    cat > "$DEB_GUI_DIR/DEBIAN/control" << EOF
Package: $APP_NAME-gui
Version: $VERSION
Architecture: amd64
Maintainer: Key-Box <support@keybox.app>
Installed-Size: 10240
Section: utils
Priority: optional
Homepage: https://github.com/keybox/key-box
Description: 本地密码管理器 - GUI 版本
 Key-Box 是一个安全的本地密码管理器，支持加密存储和备份恢复。
 .
 本软件包含图形界面版本，方便用户操作。
Depends: libc6, libgtk-3-0
EOF

    # 创建安装目录结构
    mkdir -p "$DEB_GUI_DIR/opt/$APP_NAME"
    mkdir -p "$DEB_GUI_DIR/usr/share/applications"
    mkdir -p "$DEB_GUI_DIR/usr/local/bin"
    mkdir -p "$DEB_GUI_DIR/usr/share/doc/$APP_NAME-gui"
    mkdir -p "$DEB_GUI_DIR/usr/share/icons/hicolor/512x512/apps"

    # 复制文件
    cp "$BUILD_DIR/key-box-gui" "$DEB_GUI_DIR/opt/$APP_NAME/"
    ln -sf "/opt/$APP_NAME/key-box-gui" "$DEB_GUI_DIR/usr/local/bin/key-box-gui"

    # 复制图标
    if [ -f "$PROJECT_DIR/key-box.png" ]; then
        cp "$PROJECT_DIR/key-box.png" "$DEB_GUI_DIR/opt/$APP_NAME/"
        cp "$PROJECT_DIR/key-box.png" "$DEB_GUI_DIR/usr/share/icons/hicolor/512x512/apps/key-box.png"
        ICON_PATH="/opt/$APP_NAME/key-box.png"
    else
        ICON_PATH="security-high"
    fi

    # 创建桌面图标
    cat > "$DEB_GUI_DIR/usr/share/applications/key-box.desktop" << DESKTOP
[Desktop Entry]
Name=Key-Box
Comment=本地密码管理器
Exec=/opt/$APP_NAME/key-box-gui
Icon=$ICON_PATH
Terminal=false
Type=Application
Categories=Utility;Security;
StartupNotify=true
DESKTOP

    # 复制文档
    cp "$PROJECT_DIR/README.md" "$DEB_GUI_DIR/usr/share/doc/$APP_NAME-gui/" 2>/dev/null || true

    # 计算 MD5
    cd "$DEB_GUI_DIR"
    find . -type f ! -path "./DEBIAN/*" -exec md5sum {} + > DEBIAN/md5sums 2>/dev/null || true

    # 构建 .deb
    cd "$BUILD_DIR"
    dpkg-deb --build "$DEB_GUI_DIR" "$DIST_DIR/key-box-gui-${VERSION}-amd64.deb"
    echo -e "${GREEN}✓ GUI .deb 包创建完成${NC}"

    # CLI .deb 包
    DEB_CLI_DIR="$BUILD_DIR/key-box-cli-deb"
    mkdir -p "$DEB_CLI_DIR"

    # 创建 Debian 控制文件
    mkdir -p "$DEB_CLI_DIR/DEBIAN"
    cat > "$DEB_CLI_DIR/DEBIAN/control" << EOF
Package: $APP_NAME-cli
Version: $VERSION
Architecture: amd64
Maintainer: Key-Box <support@keybox.app>
Installed-Size: 5120
Section: utils
Priority: optional
Homepage: https://github.com/keybox/key-box
Description: 本地密码管理器 - 命令行版本
 Key-Box 是一个安全的本地密码管理器，支持加密存储和备份恢复。
 .
 本软件包含命令行版本，适合自动化脚本和高级用户。
Depends: libc6
EOF

    # 创建安装目录结构
    mkdir -p "$DEB_CLI_DIR/usr/local/bin"
    mkdir -p "$DEB_CLI_DIR/usr/share/doc/$APP_NAME-cli"

    # 复制文件
    cp "$BUILD_DIR/key-box-cli" "$DEB_CLI_DIR/usr/local/bin/"
    ln -sf "/usr/local/bin/key-box-cli" "$DEB_CLI_DIR/usr/local/bin/key-box"

    # 复制文档
    cp "$PROJECT_DIR/README.md" "$DEB_CLI_DIR/usr/share/doc/$APP_NAME-cli/" 2>/dev/null || true

    # 计算 MD5
    cd "$DEB_CLI_DIR"
    find . -type f ! -path "./DEBIAN/*" -exec md5sum {} + > DEBIAN/md5sums 2>/dev/null || true

    # 构建 .deb
    cd "$BUILD_DIR"
    dpkg-deb --build "$DEB_CLI_DIR" "$DIST_DIR/key-box-cli-${VERSION}-amd64.deb"
    echo -e "${GREEN}✓ CLI .deb 包创建完成${NC}"
else
    # 创建 tar.gz 包
    echo -e "${YELLOW}创建 tar.gz 包...${NC}"

    # GUI tar.gz
    GUI_DIR="$BUILD_DIR/key-box-gui"
    mkdir -p "$GUI_DIR"
    cp "$BUILD_DIR/key-box-gui" "$GUI_DIR/"
    cp "$PROJECT_DIR/README.md" "$GUI_DIR/" 2>/dev/null || true
    if [ -f "$PROJECT_DIR/key-box.png" ]; then
        cp "$PROJECT_DIR/key-box.png" "$GUI_DIR/"
    fi

    cat > "$GUI_DIR/install.sh" << 'EOF'
#!/bin/bash
# Key-Box GUI 安装脚本

set -e

INSTALL_DIR="/opt/key-box"
BIN_DIR="/usr/local/bin"

echo "=================================="
echo "Key-Box GUI 安装程序"
echo "=================================="
echo ""
echo "此脚本将把 Key-Box GUI 安装到 $INSTALL_DIR"
echo "需要管理员权限"
echo ""

# 检查是否已安装
if [ -f "$INSTALL_DIR/key-box-gui" ] || [ -f "$BIN_DIR/key-box-gui" ]; then
    read -p "Key-Box GUI 已存在，是否覆盖？(y/N): " -n 1 -r
    echo
    if [[ ! $REPLY =~ ^[Yy]$ ]]; then
        echo "安装已取消"
        exit 0
    fi
fi

# 安装
sudo mkdir -p "$INSTALL_DIR"
sudo cp key-box-gui "$INSTALL_DIR/"
if [ -f "key-box.png" ]; then
    sudo cp key-box.png "$INSTALL_DIR/"
    sudo mkdir -p "/usr/share/icons/hicolor/512x512/apps"
    sudo cp key-box.png "/usr/share/icons/hicolor/512x512/apps/key-box.png"
    ICON_PATH="/opt/key-box/key-box.png"
else
    ICON_PATH="security-high"
fi
sudo chmod +x "$INSTALL_DIR/key-box-gui"

# 创建符号链接
sudo ln -sf "$INSTALL_DIR/key-box-gui" "$BIN_DIR/key-box-gui"

# 创建桌面图标
if [ -d "/usr/share/applications" ]; then
    sudo tee /usr/share/applications/key-box.desktop > /dev/null << DESKTOP
[Desktop Entry]
Name=Key-Box
Comment=本地密码管理器
Exec=$BIN_DIR/key-box-gui
Icon=$ICON_PATH
Terminal=false
Type=Application
Categories=Utility;Security;
DESKTOP
    echo "已创建桌面图标"
fi

echo ""
echo "=================================="
echo "安装完成！"
echo "=================================="
echo ""
echo "运行方式: $BIN_DIR/key-box-gui"
echo ""
echo "快捷启动: alias keybox='$BIN_DIR/key-box-gui'"
echo ""
echo "注意：首次运行前需要设置环境变量 SEC_APP_SALT"
echo "  export SEC_APP_SALT=\"your_salt_here\""
echo ""
EOF
    chmod +x "$GUI_DIR/install.sh"

    cd "$BUILD_DIR"
    tar -czf "$DIST_DIR/key-box-gui-${VERSION}-amd64.tar.gz" key-box-gui
    echo -e "${GREEN}✓ GUI tar.gz 包创建完成${NC}"

    # CLI tar.gz
    CLI_DIR="$BUILD_DIR/key-box-cli"
    mkdir -p "$CLI_DIR"
    cp "$BUILD_DIR/key-box-cli" "$CLI_DIR/"
    cp "$PROJECT_DIR/README.md" "$CLI_DIR/" 2>/dev/null || true

    cat > "$CLI_DIR/install.sh" << 'EOF'
#!/bin/bash
# Key-Box CLI 安装脚本

set -e

BIN_DIR="/usr/local/bin"

echo "=================================="
echo "Key-Box CLI 安装程序"
echo "=================================="
echo ""
echo "此脚本将把 Key-Box CLI 安装到 $BIN_DIR"
echo "需要管理员权限"
echo ""

# 检查是否已安装
if [ -f "$BIN_DIR/key-box-cli" ] || [ -f "$BIN_DIR/key-box" ]; then
    read -p "Key-Box CLI 已存在，是否覆盖？(y/N): " -n 1 -r
    echo
    if [[ ! $REPLY =~ ^[Yy]$ ]]; then
        echo "安装已取消"
        exit 0
    fi
fi

# 安装
sudo cp key-box-cli "$BIN_DIR/"
sudo chmod +x "$BIN_DIR/key-box-cli"
sudo ln -sf "$BIN_DIR/key-box-cli" "$BIN_DIR/key-box"

echo ""
echo "=================================="
echo "安装完成！"
echo "=================================="
echo ""
echo "运行方式: key-box-cli 或 key-box"
echo ""
echo "注意：首次运行前需要设置环境变量 SEC_APP_SALT"
echo "  export SEC_APP_SALT=\"your_salt_here\""
echo ""
EOF
    chmod +x "$CLI_DIR/install.sh"

    cd "$BUILD_DIR"
    tar -czf "$DIST_DIR/key-box-cli-${VERSION}-amd64.tar.gz" key-box-cli
    echo -e "${GREEN}✓ CLI tar.gz 包创建完成${NC}"
fi
echo ""

# 显示结果
echo -e "${GREEN}==================================${NC}"
echo -e "${GREEN}打包完成！${NC}"
echo -e "${GREEN}==================================${NC}"
echo ""
echo "输出文件:"
if [ $CREATE_DEB -eq 1 ]; then
    echo "  - $DIST_DIR/key-box-gui-${VERSION}-amd64.deb  (GUI 安装包)"
    echo "  - $DIST_DIR/key-box-cli-${VERSION}-amd64.deb  (CLI 安装包)"
else
    echo "  - $DIST_DIR/key-box-gui-${VERSION}-amd64.tar.gz  (GUI 安装包)"
    echo "  - $DIST_DIR/key-box-cli-${VERSION}-amd64.tar.gz  (CLI 安装包)"
fi
echo ""
echo "安装方式:"
if [ $CREATE_DEB -eq 1 ]; then
    echo ""
    echo "Debian/Ubuntu 系统:"
    echo "  sudo dpkg -i key-box-gui-${VERSION}-amd64.deb"
    echo "  sudo dpkg -i key-box-cli-${VERSION}-amd64.deb"
else
    echo ""
    echo "其他 Linux 系统:"
    echo "  1. 解压 tar.gz 文件"
    echo "  2. 运行 ./install.sh"
    echo "  3. 按提示完成安装"
fi
echo ""
