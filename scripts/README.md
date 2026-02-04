# Key-Box 构建脚本

本目录包含 Key-Box 的打包和构建脚本。

## 脚本说明

| 脚本文件 | 说明 | 适用平台 | 输出格式 |
|---------|------|---------|----------|
| `build-all.sh` | 全平台打包脚本（菜单选择） | macOS/Linux | - |
| `build-macos.sh` | macOS 打包脚本 | macOS | .dmg, .zip |
| `build-linux.sh` | Linux 打包脚本 | Linux | .deb, .tar.gz |
| `build-windows.bat` | Windows 打包脚本 | Windows | .exe, .zip |

## 使用方法

### 快速编译（不打包）

```bash
./scripts/build-all.sh
# 选择 5) 仅快速编译
```

### macOS 打包

生成 `.dmg` 安装包（推荐）：

```bash
./scripts/build-macos.sh
```

**输出文件**：
- `Key-Box-{version}.dmg` - macOS 安装包（双击安装，拖拽到 Applications）
- `key-box-cli-macos-{version}.zip` - CLI 独立包

**安装方式**：
1. 双击 `.dmg` 文件
2. 将 `Key-Box.app` 拖拽到 Applications 文件夹
3. 从启动台或 Launchpad 启动

### Linux 打包

生成 `.deb` 安装包（Debian/Ubuntu）：

```bash
./scripts/build-linux.sh
```

**输出文件**（如果有 dpkg-deb）：
- `key-box-gui-{version}-amd64.deb` - GUI 安装包
- `key-box-cli-{version}-amd64.deb` - CLI 安装包

**或输出文件**（如果没有 dpkg-deb）：
- `key-box-gui-{version}-amd64.tar.gz` - GUI 安装包
- `key-box-cli-{version}-amd64.tar.gz` - CLI 安装包

**安装方式**（.deb）：
```bash
sudo dpkg -i key-box-gui-{version}-amd64.deb
sudo dpkg -i key-box-cli-{version}-amd64.deb
```

**安装方式**（.tar.gz）：
```bash
tar -xzf key-box-gui-{version}-amd64.tar.gz
cd key-box-gui
sudo ./install.sh
```

### Windows 打包

生成 `.exe` 安装程序（需要 Inno Setup）：

```cmd
scripts\build-windows.bat
```

**前提条件**：
- 已安装 Inno Setup: https://jrsoftware.org/isdl.php

**输出文件**（如果有 Inno Setup）：
- `Key-Box-Setup-{version}.exe` - Windows 安装程序

**或输出文件**（如果没有 Inno Setup）：
- `key-box-gui-windows-{version}.zip` - GUI 版本
- `key-box-cli-windows-{version}.zip` - CLI 版本

**安装方式**（.exe）：
1. 双击运行 `Key-Box-Setup-{version}.exe`
2. 按向导完成安装
3. 从开始菜单或桌面图标启动

**安装方式**（.zip）：
1. 解压 zip 文件
2. 直接运行 `key-box-gui.exe` 或 `key-box-cli.exe`

### 全平台打包

```bash
./scripts/build-all.sh
# 选择 4) 全部平台
```

## 输出文件位置

所有打包输出位于 `build/dist/` 目录。

## 卸载说明

### macOS

```bash
# GUI 版本
rm -rf /Applications/Key-Box.app

# CLI 版本
sudo rm /usr/local/bin/key-box-cli
sudo rm /usr/local/bin/key-box
```

### Linux

```bash
# .deb 安装的
sudo apt remove key-box-gui
sudo apt remove key-box-cli

# 手动安装的
sudo rm /opt/key-box/key-box-gui
sudo rm /usr/local/bin/key-box-gui
sudo rm /usr/local/bin/key-box-cli
sudo rm /usr/local/bin/key-box
sudo rm /usr/share/applications/key-box.desktop
```

### Windows

运行卸载程序：
1. 打开 "控制面板" > "程序和功能"
2. 找到 "Key-Box" 并卸载

或从开始菜单运行 "卸载 Key-Box"

## 数据说明

安装和卸载脚本**不会删除数据库文件**：

- macOS/Linux: `~/.key-box.db`
- Windows: `%USERPROFILE%\.key-box.db`

如需完全删除，请手动删除数据库文件。

## 环境变量

首次运行前需要设置环境变量 `SEC_APP_SALT`：

### macOS/Linux
```bash
export SEC_APP_SALT="your_salt_here"
```

建议将其添加到 `~/.bashrc` 或 `~/.zshrc`。

### Windows
```cmd
set SEC_APP_SALT=your_salt_here
```

或设置系统环境变量（永久生效）：
1. 右键 "此电脑" > "属性"
2. "高级系统设置" > "环境变量"
3. 添加新的用户变量 `SEC_APP_SALT`

## 注意事项

1. **首次编译需要下载依赖**，请确保网络连接正常
2. **macOS Universal Binary** 需要运行 `lipo` 命令，仅适用于 macOS
3. **Windows 安装程序** 需要 Inno Setup 5.5+
4. **Linux .deb 包** 需要 `dpkg-deb` 命令
5. **GUI 版本** 依赖 Fyne 框架，打包时会包含在可执行文件中
6. **macOS .app 包** 支持 Intel 和 Apple Silicon（Universal Binary）

## 故障排除

### macOS: lipo 命令未找到

Xcode 命令行工具未安装，运行：

```bash
xcode-select --install
```

### Linux: dpkg-deb 命令未找到

```bash
# Ubuntu/Debian
sudo apt install dpkg-dev

# CentOS/RHEL
sudo yum install rpm-build
```

### Windows: Inno Setup 未安装

下载并安装：https://jrsoftware.org/isdl.php

### Fyne GUI 无法运行

**macOS**：可能需要在"系统偏好设置 > 安全性与隐私"中允许运行

**Linux**：确保安装了必要的图形库

```bash
# Ubuntu/Debian
sudo apt install libgtk-3-dev libgl1-mesa-dev xorg-dev

# CentOS/RHEL
sudo yum install gtk3-devel mesa-libGL-devel libX11-devel

# Arch
sudo pacman -S gtk3 glu libx11
```

**Windows**：通常无需额外配置

### 编译错误: CGO_ENABLED

macOS 编译 GUI 时需要启用 CGO：

```bash
CGO_ENABLED=1 go build ./cmd/gui
```

## 版本号

默认版本号为 `1.0.0`，如需修改，请编辑对应脚本文件中的 `VERSION` 变量。

## 自定义图标

### macOS

准备 `resources/icon.icns` 文件，放在项目根目录下：

```bash
# 创建图标目录
mkdir -p resources

# 使用 iconutil 转换图标（需要 1024x1024 PNG）
iconutil -c icns resources/icon.icns -s iconset
```

### Windows

在 Inno Setup 安装脚本中指定图标路径。

### Linux

在 `.desktop` 文件中指定图标路径。

## 许可证

打包脚本生成的安装程序包含项目的许可证信息。
