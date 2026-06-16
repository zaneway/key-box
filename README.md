# Key-Box

[English](README.md) | [简体中文](README.zh-CN.md)

Key-Box is a local-first password manager written in Go. It provides both a desktop GUI and a CLI, stores data in a local SQLite database, and protects secrets with a layered key model built around SSS, AES-GCM, HKDF, login password verification, and TOTP.

The project is designed for users who want to manage passwords offline without relying on a cloud vault. All password entries stay on the local machine unless the user explicitly exports an encrypted backup.

> Security notice: Key-Box is not a formally audited password manager. Treat it as an engineering project with a clear security model, not as a replacement for audited commercial or enterprise password management systems in high-assurance environments.

## Features

- Local SQLite storage: default database path is `~/.key-box.db`.
- Desktop GUI: built with Fyne.
- CLI client: available for terminal workflows.
- Login protection: login password plus 6-digit TOTP verification.
- Recovery flow: security questions are used to recover and rotate authentication material.
- Encrypted vault entries: account, password, and remark data are encrypted with the user data key.
- Metadata search: title, site, URL, category, account, and remark search after login.
- Copy actions: account, password, and remark fields can be copied from the GUI.
- Clipboard protection: copied secrets can be cleared after a configured duration.
- Auto lock: the GUI can lock the current session after a configured duration.
- Backup and restore: exports encrypted JSON backups with user metadata, encrypted vault items, and app settings.
- Cross-platform build scripts: macOS, Linux, and Windows packaging scripts are included.

## Project Status

Key-Box currently focuses on local single-machine password management.

Implemented:

- GUI registration, login, reset, vault management, backup, restore, and configuration center.
- CLI registration, login, reset, and basic vault operations.
- SQLite schema migrations.
- Encrypted backup and restore flow.
- Unit tests for core auth, crypto, database, and vault logic.

Known limitations:

- Sensitive values exist in process memory while the vault is unlocked.
- Clipboard contents may be readable by other local applications before cleanup.
- The GUI auto-lock timer is currently timer based; it is not a complete OS-level idle detector.
- The project has not undergone external cryptographic review.

## Quick Start

### Requirements

- Go 1.24 or newer.
- CGO-enabled build environment for SQLite.
- GUI builds require Fyne platform dependencies.

On Linux, install common GUI build dependencies first:

```bash
sudo apt install libgtk-3-dev libgl1-mesa-dev xorg-dev
```

### Clone

```bash
git clone <repository-url>
cd key-box
```

### Run Tests

```bash
go test ./...
```

### Build GUI

```bash
go build -o key-box-gui ./cmd/gui
```

Windows GUI builds usually should hide the console window:

```powershell
go build -ldflags "-H=windowsgui" -o key-box-gui.exe ./cmd/gui
```

### Build CLI

```bash
go build -o key-box-client ./cmd/client
```

## Running

### GUI

```bash
./key-box-gui
```

The GUI supports:

- Registering a local account.
- Setting or changing a login password.
- Logging in with login password and TOTP.
- Adding, editing, deleting, searching, and copying vault records.
- Managing categories and remarks.
- Backing up and restoring encrypted data.
- Configuring auto-lock and clipboard protection in the configuration center.

### CLI

```bash
./key-box-client
```

The CLI is menu-driven and is useful for simple terminal workflows or environments where the GUI is not available.

## Salt Configuration

Key-Box uses `SEC_APP_SALT` as part of the Root Key derivation used to protect the authentication key.

Configuration priority:

| Source | Location | Priority |
| --- | --- | --- |
| Config file | `~/.key-box.config` | High |
| Environment variable | `SEC_APP_SALT` | Low |

First run behavior:

- If no local users exist and no salt is configured, Key-Box generates a random salt and saves it to `~/.key-box.config`.
- If users already exist but no salt is configured, Key-Box asks the user to restore the original salt before login.

Manual configuration:

```bash
printf '%s' '<original-salt>' > ~/.key-box.config
chmod 600 ~/.key-box.config
```

Environment variable fallback:

```bash
export SEC_APP_SALT="<original-salt>"
```

On Windows PowerShell:

```powershell
$env:SEC_APP_SALT="<original-salt>"
```

Keep `~/.key-box.config` with your backups. Without the original salt, existing accounts cannot decrypt the stored authentication key.

## Backup and Restore

Backups are JSON files generated from the GUI. They contain:

- User metadata and encrypted key material.
- Encrypted vault records.
- App settings such as auto-lock and clipboard protection durations.
- Export timestamp and backup format version.

Backups do not include `SEC_APP_SALT`. Store the backup file and salt separately.

Typical cross-device restore:

1. Copy the backup JSON file to the new machine.
2. Copy the original `~/.key-box.config` to the new machine, or configure the same `SEC_APP_SALT`.
3. Start Key-Box.
4. Use the restore flow from the login page or from the unlocked vault page.
5. Log in with the original TOTP setup.

Recommended additional protection for backup files:

```bash
gpg -c key-box-backup-YYYYMMDD-HHMMSS.json
```

## Security Model

Key-Box uses a layered key hierarchy.

| Material | Purpose | Storage |
| --- | --- | --- |
| Security answers | Recover Key A through SSS-derived shares | Not stored directly |
| Key A | Protects the master key | Reconstructed at recovery time |
| Key M | Derives the authentication key | Encrypted in SQLite |
| Key B | TOTP seed and protector for Key C | Encrypted by Root Key |
| Root Key | Protects Key B | Derived at runtime from salt and fixed material |
| Key C | Encrypts vault entries | Encrypted by Key B |
| Vault data | Account, password, and remark JSON | AES-GCM ciphertext |

Important boundaries:

- Vault passwords and remarks are not stored as plaintext in SQLite.
- Site/title/category metadata is stored as plaintext to support listing and filtering.
- Security questions are stored as plaintext; answers are not.
- The local database alone is insufficient to decrypt the vault without the correct salt and login flow.

See [docs/DESIGN.md](docs/DESIGN.md) for the detailed architecture.

## Data Files

Default files:

| File | Purpose |
| --- | --- |
| `~/.key-box.db` | SQLite database with users, encrypted vault entries, and app settings |
| `~/.key-box.config` | Salt used in Root Key derivation |

Packaging and uninstall scripts do not remove these files automatically.

## Build Scripts

Packaging scripts live in [scripts](scripts/):

| Script | Platform | Purpose |
| --- | --- | --- |
| `scripts/build-macos.sh` | macOS | Build macOS app and CLI packages |
| `scripts/build-linux.sh` | Linux | Build Linux packages |
| `scripts/build-windows.sh` | Windows cross-build helper | Build Windows artifacts where supported |
| `scripts/build-windows.bat` | Windows | Build Windows package |
| `scripts/build-all.sh` | macOS/Linux | Interactive packaging entry point |

See [scripts/README.md](scripts/README.md) for platform-specific packaging notes.

## Development

Useful commands:

```bash
go test ./...
go test ./internal/auth ./internal/crypto ./internal/db ./internal/vault
go build ./cmd/gui
go build ./cmd/client
```

Repository layout:

```text
cmd/
  client/        CLI entry point
  gui/           Fyne GUI entry point
internal/
  auth/          Registration, login, password reset, TOTP flow
  config/        Salt file and environment handling
  crypto/        AES-GCM, HKDF, SSS, password utilities
  db/            SQLite schema, migrations, settings, persistence
  vault/         Vault item encryption and query logic
docs/            Design notes, backup/restore docs, roadmap
scripts/         Packaging scripts
```

## Roadmap

Potential future work:

- Stronger memory hygiene for unlocked secrets.
- More complete idle detection for auto-lock.
- Importers for common password manager formats.
- Security audit and threat model review.
- Browser integration or autofill support.
- Hardware-backed authentication options.

## Contributing

Contributions should keep the local-first and security-sensitive nature of the project in mind.

Before opening a pull request:

1. Keep changes scoped.
2. Add or update tests for behavior changes.
3. Run `go test ./...`.
4. Update documentation when user workflows, backup format, or security assumptions change.

Security-sensitive changes should explain the threat model, compatibility impact, and rollback strategy.

## License

This project is licensed under the Apache License 2.0. See [LICENSE](LICENSE).
