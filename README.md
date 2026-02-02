# Local Password Manager (Key-Box)

一个基于 Go 语言开发的安全本地密码管理器。采用 Shamir's Secret Sharing (SSS)、AES-GCM、HKDF 和 TOTP 等现代密码学标准，确保您的账号密码安全存储。


## ✨ 特性

- **零知识证明架构**: 服务器（或本地数据库）不存储您的明文密码或密保答案。
- **多级密钥保护**: 结合 SSS 分片、HKDF 密钥派生和环境因子加密。
- **双因素认证 (2FA)**: 内置 TOTP 算法，支持 Google Authenticator 等应用。
- **灵活配置**: 支持 `~/.key-box.config` 配置文件和环境变量 `SEC_APP_SALT` 两种方式配置 Salt。
- **跨平台**: 支持 Windows、macOS 和 Linux。
- **本地存储**: 使用嵌入式 SQLite 数据库，数据完全掌控在您手中。
- **图形化界面 (GUI)**: 提供直观的操作界面 (基于 Fyne)。

## 🛠️ 安装与构建

### 前置要求
- Go 1.23+

### 1. 下载源码
```bash
git clone <repository-url>
cd key-box
```

### 2. 下载依赖
```bash
go mod tidy
```

### 3. 编译

#### 命令行版本 (CLI)
**macOS / Linux:**
```bash
go build -o key-box-client cmd/client/main.go
```
**Windows:**
```powershell
go build -o key-box-client.exe cmd/client/main.go
```

#### 图形界面版本 (GUI)
**macOS / Linux:**
```bash
go build -o key-box-gui cmd/gui/main.go
```
**Windows:**
```powershell
go build -o key-box-gui.exe cmd/gui/main.go
```
*注意：GUI 版本首次运行可能需要较长时间编译依赖。Windows 下编译 GUI 建议添加 `-ldflags -H=windowsgui` 参数以隐藏控制台窗口。*

## 🚀 使用指南 (GUI 版本)

### 1. 运行程序
双击 `key-box-gui` 或在终端运行：
```bash
./key-box-gui
```

### 2. 配置 Salt (自动管理)
程序支持两种配置 Salt 的方式，优先级如下：

| 配置方式 | 文件路径/变量 | 优先级 |
|---------|--------------|--------|
| 配置文件 | `~/.key-box.config` | 高 |
| 环境变量 | `SEC_APP_SALT` | 低 |

- **如果是首次运行且未配置**: 程序会自动生成一个安全的随机 Salt，并自动保存到配置文件 `~/.key-box.config`。
- **配置文件优先**: 程序优先从 `~/.key-box.config` 读取 Salt，只有配置文件不存在时才读取环境变量 `SEC_APP_SALT`。
- **跨设备迁移**: 如需在其他设备使用相同数据，请将 `~/.key-box.config` 文件复制过去，或手动设置环境变量 `SEC_APP_SALT` 为相同的值。
- **OTP生成方式**: 使用[OTP生成工具](https://github.com/zaneway/HeTu),或使用[基础代码程序](https://github.com/zaneway/otp)

### 3. 功能操作
界面分为三个标签页：
- **登录**: 输入用户名和 6 位 OTP 验证码。
- **注册**: 填写用户名、三个密保问题及答案。注册成功后会显示 **Key B**，请务必导入 Authenticator App。
- **重置密码**: 通过密保问题重置 Key B。

**登录成功后**，您将进入密码库界面，支持：
- 查看已保存的密码（密码默认脱敏显示为 `********`）。
- 点击 "复制" 按钮将明文密码复制到剪贴板。
- 添加新的密码记录。
- **备份数据**: 导出加密数据库并提示保存 Salt 值。
- **恢复数据**: 从备份文件恢复数据。
- 退出登录。

## 💾 数据备份与恢复

### 备份数据
1. 登录后，点击工具栏的 "备份数据" 按钮。
2. 系统会弹出对话框，显示当前 Salt 值（读取自 `~/.key-box.config` 或环境变量）。
3. **⚠️ 重要**: 请务必保存该 Salt 值，它是解密备份数据的唯一凭证。
4. 确认后选择保存位置，数据库将导出为带时间戳的文件。

### 恢复数据
1. 确保配置文件 `~/.key-box.config` 或环境变量 `SEC_APP_SALT` 中的 Salt 值与备份时一致。
   - 推荐方式：将原设备的 `~/.key-box.config` 文件复制到新设备
   - 备选方式：设置环境变量 `export SEC_APP_SALT="原Salt值"`
2. 点击工具栏的 "恢复数据" 按钮。
3. 阅读警告提示后，选择备份的 `.db` 文件。
4. 恢复成功后，建议重启应用以加载新数据。

**安全提示**:
- 备份文件 + Salt 值 = 完整的数据访问权限，请妥善保管。
- 建议通过加密渠道传输备份文件（如加密云盘）。
- Salt 值可单独记录在其他密码管理器或纸质笔记中。

## 🚀 使用指南 (CLI 版本)

### 1. 配置 Salt (推荐方式)
CLI 版本支持两种配置 Salt 的方式：

**方式一：配置文件（推荐）**
程序会自动管理 `~/.key-box.config` 文件。首次运行时如果没有配置，会自动生成并保存到此文件中。

**方式二：环境变量（备选）**
如果配置文件不存在，程序会尝试从环境变量读取：

**macOS / Linux:**
```bash
export SEC_APP_SALT="your-unique-secret-salt-2026"
```

**Windows (PowerShell):**
```powershell
$env:SEC_APP_SALT="your-unique-secret-salt-2026"
```

**配置优先级**: `~/.key-box.config` > `SEC_APP_SALT` 环境变量 > 自动生成

### 2. 运行程序
```bash
./key-box-client
```

首次运行时，程序会自动生成 Salt 并保存到 `~/.key-box.config`，请妥善保管此配置文件。

## 📂 文件说明
- `key-box-client`: 命令行客户端。
- `key-box-gui`: 图形界面客户端。
- `.key-box.db`: 加密数据库文件（默认生成在用户主目录 `~/.key-box.db`）。
- `~/.key-box.config`: Salt 配置文件（优先级高于环境变量，请妥善保管）。

## 🛡️ 安全架构简述
- **密钥 A**: 由密保答案通过 SSS 算法合成，不存储。
- **密钥 M**: 随机生成，由 A 加密存储。
- **密钥 B**: 由 M 和用户名通过 HKDF 派生，作为 TOTP 种子和数据加密的主密钥。
- **Root Key**: 由 Salt（来自 `~/.key-box.config` 或环境变量）和硬编码常量异或生成，用于加密存储密钥 B。
- **密钥 C**: 随机生成，用于加密实际的用户数据，由 B 加密存储。

---
*注意：请妥善保管您的 `~/.key-box.config` 配置文件和密保答案，一旦丢失将无法恢复数据。*
