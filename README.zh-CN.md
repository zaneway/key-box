# Key-Box

[English](README.md) | [简体中文](README.zh-CN.md)

Key-Box 是一个使用 Go 开发的本地优先密码管理器。它同时提供桌面 GUI 和 CLI，使用本地 SQLite 数据库存储数据，并围绕 SSS、AES-GCM、HKDF、登录密码校验和 TOTP 构建分层密钥模型来保护敏感信息。

该项目面向希望离线管理密码、不依赖云端密码库的用户。除非用户显式导出加密备份，所有密码条目都保留在本机。

> 安全提示：Key-Box 不是经过正式审计的密码管理器。它更适合作为具备清晰安全模型的工程项目使用，不应在高保障环境中替代经过审计的商业或企业级密码管理系统。

## 功能特性

- 本地 SQLite 存储：默认数据库路径为 `~/.key-box.db`。
- 桌面 GUI：基于 Fyne 构建。
- CLI 客户端：支持终端工作流。
- 登录保护：登录密码加 6 位 TOTP 验证。
- 恢复流程：通过密保问题恢复并轮转认证材料。
- 加密密码库条目：账号、密码和备注数据使用用户数据密钥加密。
- 元数据搜索：登录后可搜索标题、站点、URL、分类、账号和备注。
- 复制操作：GUI 中可复制账号、密码和备注字段。
- 剪贴板保护：复制的敏感内容可在配置时间后清理。
- 自动锁定：GUI 可在配置时间后锁定当前会话。
- 备份与恢复：导出包含用户元数据、加密密码库条目和应用设置的加密 JSON 备份。
- 跨平台构建脚本：包含 macOS、Linux 和 Windows 打包脚本。

## 项目状态

Key-Box 当前聚焦本地单机密码管理。

已实现：

- GUI 注册、登录、重置、密码库管理、备份、恢复和配置中心。
- CLI 注册、登录、重置和基础密码库操作。
- SQLite schema 迁移。
- 加密备份与恢复流程。
- 核心认证、密码学、数据库和密码库逻辑的单元测试。

已知限制：

- 密码库解锁期间，敏感值会存在于进程内存中。
- 剪贴板内容在清理前可能被其他本地应用读取。
- GUI 自动锁定当前基于计时器，不是完整的操作系统级空闲检测。
- 项目尚未经过外部密码学审计。

## 快速开始

### 环境要求

- Go 1.24 或更高版本。
- 支持 CGO 的 SQLite 构建环境。
- GUI 构建需要 Fyne 平台依赖。

Linux 环境先安装常见 GUI 构建依赖：

```bash
sudo apt install libgtk-3-dev libgl1-mesa-dev xorg-dev
```

### 克隆代码

```bash
git clone <repository-url>
cd key-box
```

### 运行测试

```bash
go test ./...
```

### 构建 GUI

```bash
go build -o key-box-gui ./cmd/gui
```

Windows GUI 构建通常应隐藏控制台窗口：

```powershell
go build -ldflags "-H=windowsgui" -o key-box-gui.exe ./cmd/gui
```

### 构建 CLI

```bash
go build -o key-box-client ./cmd/client
```

## 运行方式

### GUI

```bash
./key-box-gui
```

GUI 支持：

- 注册本地账号。
- 设置或修改登录密码。
- 使用登录密码和 TOTP 登录。
- 新增、编辑、删除、搜索和复制密码库记录。
- 管理分类和备注。
- 备份和恢复加密数据。
- 在配置中心设置自动锁定和剪贴板保护。

### CLI

```bash
./key-box-client
```

CLI 使用菜单驱动，适合简单终端工作流或 GUI 不可用的环境。

## Salt 配置

Key-Box 使用 `SEC_APP_SALT` 参与 Root Key 派生，Root Key 用于保护认证密钥。

配置优先级：

| 来源 | 位置 | 优先级 |
| --- | --- | --- |
| 配置文件 | `~/.key-box.config` | 高 |
| 环境变量 | `SEC_APP_SALT` | 低 |

首次运行行为：

- 如果本地没有用户且未配置 salt，Key-Box 会生成随机 salt 并保存到 `~/.key-box.config`。
- 如果已有用户但未配置 salt，Key-Box 会要求用户恢复原始 salt 后再登录。

手动配置：

```bash
printf '%s' '<original-salt>' > ~/.key-box.config
chmod 600 ~/.key-box.config
```

环境变量兜底：

```bash
export SEC_APP_SALT="<original-salt>"
```

Windows PowerShell：

```powershell
$env:SEC_APP_SALT="<original-salt>"
```

请将 `~/.key-box.config` 与备份一起妥善保存。缺少原始 salt 时，已有账号无法解密存储的认证密钥。

## 备份与恢复

备份是由 GUI 生成的 JSON 文件，包含：

- 用户元数据和加密密钥材料。
- 加密密码库记录。
- 自动锁定、剪贴板保护时长等应用设置。
- 导出时间戳和备份格式版本。

备份文件不包含 `SEC_APP_SALT`。请将备份文件和 salt 分开保存。

典型跨设备恢复流程：

1. 将备份 JSON 文件复制到新设备。
2. 将原始 `~/.key-box.config` 复制到新设备，或配置相同的 `SEC_APP_SALT`。
3. 启动 Key-Box。
4. 从登录页或已解锁密码库页进入恢复流程。
5. 使用原 TOTP 设置登录。

建议对备份文件增加额外保护：

```bash
gpg -c key-box-backup-YYYYMMDD-HHMMSS.json
```

## 安全模型

Key-Box 使用分层密钥体系。

| 材料 | 用途 | 存储方式 |
| --- | --- | --- |
| 密保答案 | 通过 SSS 派生分片恢复 Key A | 不直接存储 |
| Key A | 保护 master key | 恢复时重建 |
| Key M | 派生认证密钥 | 加密存储在 SQLite |
| Key B | TOTP 种子，并保护 Key C | 由 Root Key 加密 |
| Root Key | 保护 Key B | 运行时由 salt 和固定材料派生 |
| Key C | 加密密码库条目 | 由 Key B 加密 |
| 密码库数据 | 账号、密码和备注 JSON | AES-GCM 密文 |

关键边界：

- 密码库中的密码和备注不会以明文存储在 SQLite。
- 站点、标题和分类元数据以明文存储，用于列表展示和过滤。
- 密保问题以明文存储，答案不存储。
- 仅有本地数据库不足以解密密码库，还需要正确的 salt 和登录流程。

详细架构见 [docs/DESIGN.md](docs/DESIGN.md)。

## 数据文件

默认文件：

| 文件 | 用途 |
| --- | --- |
| `~/.key-box.db` | 保存用户、加密密码库条目和应用设置的 SQLite 数据库 |
| `~/.key-box.config` | Root Key 派生使用的 salt |

打包和卸载脚本不会自动删除这些文件。

## 构建脚本

打包脚本位于 [scripts](scripts/)：

| 脚本 | 平台 | 用途 |
| --- | --- | --- |
| `scripts/build-macos.sh` | macOS | 构建 macOS app 和 CLI 包 |
| `scripts/build-linux.sh` | Linux | 构建 Linux 包 |
| `scripts/build-windows.sh` | Windows 交叉构建辅助 | 在支持的环境中构建 Windows 产物 |
| `scripts/build-windows.bat` | Windows | 构建 Windows 包 |
| `scripts/build-all.sh` | macOS/Linux | 交互式打包入口 |

平台相关打包说明见 [scripts/README.md](scripts/README.md)。

## 开发

常用命令：

```bash
go test ./...
go test ./internal/auth ./internal/crypto ./internal/db ./internal/vault
go build ./cmd/gui
go build ./cmd/client
```

仓库结构：

```text
cmd/
  client/        CLI 入口
  gui/           Fyne GUI 入口
internal/
  auth/          注册、登录、密码重置、TOTP 流程
  config/        Salt 文件和环境变量处理
  crypto/        AES-GCM、HKDF、SSS、密码工具
  db/            SQLite schema、迁移、设置、持久化
  vault/         密码库条目加密和查询逻辑
docs/            设计说明、备份恢复文档、路线图
scripts/         打包脚本
```

## 路线图

潜在后续工作：

- 改进解锁后敏感数据的内存处理。
- 更完整的自动锁定空闲检测。
- 支持常见密码管理器格式导入。
- 安全审计和威胁模型评审。
- 浏览器集成或自动填充支持。
- 硬件支持的认证能力。

## 贡献

贡献时应考虑项目本地优先和安全敏感的特性。

提交 pull request 前：

1. 保持变更范围聚焦。
2. 对行为变更新增或更新测试。
3. 运行 `go test ./...`。
4. 当用户工作流、备份格式或安全假设变化时，同步更新文档。

安全敏感变更应说明威胁模型、兼容性影响和回滚策略。

## 许可证

本项目使用 Apache License 2.0。详见 [LICENSE](LICENSE)。
