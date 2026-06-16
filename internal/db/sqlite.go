package db

import (
	"database/sql"
	"os"
	"path/filepath"
	"time"

	_ "github.com/mattn/go-sqlite3"
)

type DB struct {
	*sql.DB
}

// InitDB 初始化嵌入式 SQLite 数据库。
// 默认路径: 用户主目录下的 .key-box.db
func InitDB() (*DB, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, err
	}
	dbPath := filepath.Join(home, ".key-box.db")

	return InitDBAt(dbPath)
}

// InitDBAt initializes a SQLite database at the given path.
// It is used by InitDB and tests so schema changes can be verified safely.
func InitDBAt(dbPath string) (*DB, error) {
	conn, err := sql.Open("sqlite3", dbPath)
	if err != nil {
		return nil, err
	}

	if err := conn.Ping(); err != nil {
		return nil, err
	}

	db := &DB{conn}
	if err := db.createTables(); err != nil {
		return nil, err
	}
	if err := db.runMigrations(); err != nil {
		return nil, err
	}

	return db, nil
}

// createTables 创建所需的数据库表结构。
// users: 存储用户元数据和加密后的密钥链。
// vault: 存储用户加密后的账号密码数据。
// app_settings: 存储应用级安全配置，不包含密钥或明文密码。
func (db *DB) createTables() error {
	usersTable := `
	CREATE TABLE IF NOT EXISTS users (
		username TEXT PRIMARY KEY,
		salt BLOB,             -- 用于密保答案 Hash 的随机盐
		question_1 TEXT,       -- 密保问题 (明文)
		question_2 TEXT,       -- 密保问题 (明文)
		question_3 TEXT,       -- 密保问题 (明文)
		enc_m BLOB,            -- 被 Key A 加密后的 Master Key
		enc_b BLOB,            -- 被 Root Key 加密后的 Auth Key
		enc_c BLOB,            -- 被 Key B 加密后的 Data Key
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP
	);`

	vaultTable := `
	CREATE TABLE IF NOT EXISTS vault (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		username TEXT,
		site TEXT,             -- 网站/应用名称 (明文索引)
		enc_data BLOB,         -- 被 Key C 加密后的账号密码 JSON
		updated_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		FOREIGN KEY(username) REFERENCES users(username)
	);`

	appSettingsTable := `
	CREATE TABLE IF NOT EXISTS app_settings (
		key TEXT PRIMARY KEY,
		value INTEGER NOT NULL,
		updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
	);`

	if _, err := db.Exec(usersTable); err != nil {
		return err
	}
	if _, err := db.Exec(vaultTable); err != nil {
		return err
	}
	if _, err := db.Exec(appSettingsTable); err != nil {
		return err
	}

	return nil
}

func (db *DB) runMigrations() error {
	if _, err := db.Exec(`
	CREATE TABLE IF NOT EXISTS schema_migrations (
		version INTEGER PRIMARY KEY,
		applied_at DATETIME DEFAULT CURRENT_TIMESTAMP
	);`); err != nil {
		return err
	}

	if err := db.runMigration(1, []migrationColumn{
		{table: "users", name: "password_salt", ddl: `ALTER TABLE users ADD COLUMN password_salt BLOB`},
		{table: "users", name: "password_verifier", ddl: `ALTER TABLE users ADD COLUMN password_verifier BLOB`},
		{table: "users", name: "password_kdf", ddl: `ALTER TABLE users ADD COLUMN password_kdf TEXT`},
		{table: "users", name: "requires_password_setup", ddl: `ALTER TABLE users ADD COLUMN requires_password_setup INTEGER NOT NULL DEFAULT 1`},
		{table: "users", name: "profile_updated_at", ddl: `ALTER TABLE users ADD COLUMN profile_updated_at DATETIME`},
	}); err != nil {
		return err
	}

	if err := db.runMigration(2, []migrationColumn{
		{table: "vault", name: "title", ddl: `ALTER TABLE vault ADD COLUMN title TEXT`},
		{table: "vault", name: "url", ddl: `ALTER TABLE vault ADD COLUMN url TEXT`},
		{table: "vault", name: "category", ddl: `ALTER TABLE vault ADD COLUMN category TEXT NOT NULL DEFAULT '未分类'`},
		{table: "vault", name: "favorite", ddl: `ALTER TABLE vault ADD COLUMN favorite INTEGER NOT NULL DEFAULT 0`},
		{table: "vault", name: "created_at", ddl: `ALTER TABLE vault ADD COLUMN created_at DATETIME`},
	}); err != nil {
		return err
	}
	_, err := db.Exec(`UPDATE vault SET title = site WHERE title IS NULL OR title = ''`)
	if err != nil {
		return err
	}
	_, err = db.Exec(`UPDATE vault SET created_at = COALESCE(created_at, updated_at) WHERE created_at IS NULL`)
	return err
}

type migrationColumn struct {
	table string
	name  string
	ddl   string
}

func (db *DB) runMigration(version int, columns []migrationColumn) error {
	applied, err := db.migrationApplied(version)
	if err != nil {
		return err
	}
	if applied {
		return nil
	}

	for _, column := range columns {
		exists, err := db.columnExists(column.table, column.name)
		if err != nil {
			return err
		}
		if exists {
			continue
		}
		if _, err := db.Exec(column.ddl); err != nil {
			return err
		}
	}

	_, err = db.Exec(`INSERT INTO schema_migrations (version) VALUES (?)`, version)
	return err
}

func (db *DB) migrationApplied(version int) (bool, error) {
	var count int
	if err := db.QueryRow(`SELECT COUNT(*) FROM schema_migrations WHERE version = ?`, version).Scan(&count); err != nil {
		return false, err
	}
	return count > 0, nil
}

func (db *DB) columnExists(table, column string) (bool, error) {
	rows, err := db.Query(`PRAGMA table_info(` + table + `)`)
	if err != nil {
		return false, err
	}
	defer rows.Close()

	for rows.Next() {
		var (
			cid        int
			name       string
			columnType string
			notNull    int
			defaultVal any
			pk         int
		)
		if err := rows.Scan(&cid, &name, &columnType, &notNull, &defaultVal, &pk); err != nil {
			return false, err
		}
		if name == column {
			return true, nil
		}
	}
	if err := rows.Err(); err != nil {
		return false, err
	}
	return false, nil
}

type User struct {
	Username  string
	Salt      []byte
	Question1 string
	Question2 string
	Question3 string
	EncM      []byte
	EncB      []byte
	EncC      []byte
}

type PasswordVerifier struct {
	Salt     []byte
	Verifier []byte
	KDF      string
}

const (
	appSettingAutoLockSeconds            = "auto_lock_seconds"
	appSettingClipboardProtectionSeconds = "clipboard_protection_seconds"

	defaultAutoLockSeconds            = 600
	defaultClipboardProtectionSeconds = 30
)

var (
	allowedAutoLockSeconds = map[int]bool{
		0:    true,
		300:  true,
		600:  true,
		1800: true,
	}
	allowedClipboardProtectionSeconds = map[int]bool{
		0:   true,
		30:  true,
		60:  true,
		300: true,
	}
)

// AppSettings 保存应用级安全配置。
// 取值使用秒数，0 表示关闭对应保护，避免本地化文案进入数据库。
type AppSettings struct {
	AutoLockSeconds            int `json:"auto_lock_seconds"`
	ClipboardProtectionSeconds int `json:"clipboard_protection_seconds"`
}

// DefaultAppSettings 返回默认安全配置。
func DefaultAppSettings() AppSettings {
	return AppSettings{
		AutoLockSeconds:            defaultAutoLockSeconds,
		ClipboardProtectionSeconds: defaultClipboardProtectionSeconds,
	}
}

// AutoLockDuration 返回自动锁定时长，0 表示永不自动锁定。
func (s AppSettings) AutoLockDuration() time.Duration {
	return time.Duration(s.AutoLockSeconds) * time.Second
}

// ClipboardProtectionDuration 返回剪贴板清理时长，0 表示不自动清理。
func (s AppSettings) ClipboardProtectionDuration() time.Duration {
	return time.Duration(s.ClipboardProtectionSeconds) * time.Second
}

// NormalizeAppSettings 将配置限制在产品允许的安全策略枚举内。
func NormalizeAppSettings(settings AppSettings) AppSettings {
	defaults := DefaultAppSettings()
	if !allowedAutoLockSeconds[settings.AutoLockSeconds] {
		settings.AutoLockSeconds = defaults.AutoLockSeconds
	}
	if !allowedClipboardProtectionSeconds[settings.ClipboardProtectionSeconds] {
		settings.ClipboardProtectionSeconds = defaults.ClipboardProtectionSeconds
	}
	return settings
}

// LoadAppSettings 从数据库读取应用级安全配置，不存在时返回默认配置。
func (db *DB) LoadAppSettings() (AppSettings, error) {
	settings := DefaultAppSettings()
	rows, err := db.Query(`SELECT key, value FROM app_settings WHERE key IN (?, ?)`, appSettingAutoLockSeconds, appSettingClipboardProtectionSeconds)
	if err != nil {
		return AppSettings{}, err
	}
	defer rows.Close()

	for rows.Next() {
		var key string
		var value int
		if err := rows.Scan(&key, &value); err != nil {
			return AppSettings{}, err
		}
		switch key {
		case appSettingAutoLockSeconds:
			settings.AutoLockSeconds = value
		case appSettingClipboardProtectionSeconds:
			settings.ClipboardProtectionSeconds = value
		}
	}
	if err := rows.Err(); err != nil {
		return AppSettings{}, err
	}
	return NormalizeAppSettings(settings), nil
}

// SaveAppSettings 规范化并保存应用级安全配置。
func (db *DB) SaveAppSettings(settings AppSettings) error {
	settings = NormalizeAppSettings(settings)
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	stmt := `INSERT INTO app_settings (key, value, updated_at)
		VALUES (?, ?, CURRENT_TIMESTAMP)
		ON CONFLICT(key) DO UPDATE SET value = excluded.value, updated_at = CURRENT_TIMESTAMP`
	if _, err := tx.Exec(stmt, appSettingAutoLockSeconds, settings.AutoLockSeconds); err != nil {
		return err
	}
	if _, err := tx.Exec(stmt, appSettingClipboardProtectionSeconds, settings.ClipboardProtectionSeconds); err != nil {
		return err
	}
	return tx.Commit()
}

func (db *DB) CreateUser(u *User) error {
	stmt := `INSERT INTO users (username, salt, question_1, question_2, question_3, enc_m, enc_b, enc_c) VALUES (?, ?, ?, ?, ?, ?, ?, ?)`
	_, err := db.Exec(stmt, u.Username, u.Salt, u.Question1, u.Question2, u.Question3, u.EncM, u.EncB, u.EncC)
	return err
}

func (db *DB) GetUser(username string) (*User, error) {
	stmt := `SELECT username, salt, question_1, question_2, question_3, enc_m, enc_b, enc_c FROM users WHERE username = ?`
	row := db.QueryRow(stmt, username)

	u := &User{}
	err := row.Scan(&u.Username, &u.Salt, &u.Question1, &u.Question2, &u.Question3, &u.EncM, &u.EncB, &u.EncC)
	if err != nil {
		return nil, err
	}
	return u, nil
}

func (db *DB) UserCount() (int, error) {
	var count int
	err := db.QueryRow(`SELECT COUNT(*) FROM users`).Scan(&count)
	return count, err
}

func (db *DB) RequiresPasswordSetup(username string) (bool, error) {
	var requiresSetup int
	err := db.QueryRow(`SELECT requires_password_setup FROM users WHERE username = ?`, username).Scan(&requiresSetup)
	if err != nil {
		return false, err
	}
	return requiresSetup != 0, nil
}

func (db *DB) SetPasswordVerifier(username string, verifier *PasswordVerifier) error {
	stmt := `
	UPDATE users
	SET password_salt = ?,
		password_verifier = ?,
		password_kdf = ?,
		requires_password_setup = 0,
		profile_updated_at = CURRENT_TIMESTAMP
	WHERE username = ?`
	_, err := db.Exec(stmt, verifier.Salt, verifier.Verifier, verifier.KDF, username)
	return err
}

func (db *DB) GetPasswordVerifier(username string) (*PasswordVerifier, error) {
	stmt := `SELECT password_salt, password_verifier, password_kdf FROM users WHERE username = ?`
	row := db.QueryRow(stmt, username)

	verifier := &PasswordVerifier{}
	if err := row.Scan(&verifier.Salt, &verifier.Verifier, &verifier.KDF); err != nil {
		return nil, err
	}
	return verifier, nil
}

func (db *DB) SaveVaultItem(username, site string, encData []byte) error {
	return db.SaveVaultItemWithMeta(username, site, site, "", "未分类", false, encData)
}

func (db *DB) SaveVaultItemWithMeta(username, title, site, url, category string, favorite bool, encData []byte) error {
	if title == "" {
		title = site
	}
	if category == "" {
		category = "未分类"
	}
	stmt := `INSERT INTO vault (username, title, site, url, category, favorite, enc_data, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)`
	_, err := db.Exec(stmt, username, title, site, url, category, favorite, encData)
	return err
}

type VaultItem struct {
	ID        int
	Title     string
	Site      string
	URL       string
	Category  string
	Favorite  bool
	EncData   []byte
	UpdatedAt string
}

func (db *DB) GetVaultItems(username string) ([]VaultItem, error) {
	stmt := `SELECT id, COALESCE(title, site), site, COALESCE(url, ''), COALESCE(category, '未分类'), favorite, enc_data, COALESCE(updated_at, '') FROM vault WHERE username = ? ORDER BY updated_at DESC, id DESC`
	rows, err := db.Query(stmt, username)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var items []VaultItem
	for rows.Next() {
		var i VaultItem
		if err := rows.Scan(&i.ID, &i.Title, &i.Site, &i.URL, &i.Category, &i.Favorite, &i.EncData, &i.UpdatedAt); err != nil {
			return nil, err
		}
		items = append(items, i)
	}
	return items, nil
}

func (db *DB) UpdateVaultItem(id int, site string, encData []byte) error {
	return db.UpdateVaultItemWithMeta(id, site, site, "", "未分类", false, encData)
}

func (db *DB) UpdateVaultItemWithMeta(id int, title, site, url, category string, favorite bool, encData []byte) error {
	if title == "" {
		title = site
	}
	if category == "" {
		category = "未分类"
	}
	stmt := `UPDATE vault SET title = ?, site = ?, url = ?, category = ?, favorite = ?, enc_data = ?, updated_at = CURRENT_TIMESTAMP WHERE id = ?`
	_, err := db.Exec(stmt, title, site, url, category, favorite, encData, id)
	return err
}

func (db *DB) DeleteVaultItem(id int) error {
	stmt := `DELETE FROM vault WHERE id = ?`
	_, err := db.Exec(stmt, id)
	return err
}

func (db *DB) ListVaultCategories(username string) ([]string, error) {
	rows, err := db.Query(`SELECT DISTINCT category FROM vault WHERE username = ? AND category IS NOT NULL AND category <> '' ORDER BY category`, username)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var categories []string
	for rows.Next() {
		var category string
		if err := rows.Scan(&category); err != nil {
			return nil, err
		}
		categories = append(categories, category)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return categories, nil
}

func (db *DB) RenameVaultCategory(username, oldCategory, newCategory string) error {
	_, err := db.Exec(`UPDATE vault SET category = ?, updated_at = CURRENT_TIMESTAMP WHERE username = ? AND category = ?`, newCategory, username, oldCategory)
	return err
}

func (db *DB) MoveVaultCategoryToDefault(username, category string) error {
	_, err := db.Exec(`UPDATE vault SET category = '未分类', updated_at = CURRENT_TIMESTAMP WHERE username = ? AND category = ?`, username, category)
	return err
}
