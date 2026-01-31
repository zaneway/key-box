package db

import (
	"database/sql"
	"os"
	"path/filepath"

	_ "github.com/mattn/go-sqlite3"
)

type DB struct {
	*sql.DB
}

// InitDB 初始化嵌入式 SQLite 数据库。
// 默认路径: 用户主目录下的 .key-box.db
func InitDB() (*DB, error) {
	// Use a local file
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, err
	}
	dbPath := filepath.Join(home, ".key-box.db")

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

	return db, nil
}

// createTables 创建所需的数据库表结构。
// users: 存储用户元数据和加密后的密钥链。
// vault: 存储用户加密后的账号密码数据。
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

	if _, err := db.Exec(usersTable); err != nil {
		return err
	}
	if _, err := db.Exec(vaultTable); err != nil {
		return err
	}

	return nil
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

func (db *DB) SaveVaultItem(username, site string, encData []byte) error {
	stmt := `INSERT INTO vault (username, site, enc_data) VALUES (?, ?, ?)`
	_, err := db.Exec(stmt, username, site, encData)
	return err
}

type VaultItem struct {
	ID      int
	Site    string
	EncData []byte
}

func (db *DB) GetVaultItems(username string) ([]VaultItem, error) {
	stmt := `SELECT id, site, enc_data FROM vault WHERE username = ?`
	rows, err := db.Query(stmt, username)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var items []VaultItem
	for rows.Next() {
		var i VaultItem
		if err := rows.Scan(&i.ID, &i.Site, &i.EncData); err != nil {
			return nil, err
		}
		items = append(items, i)
	}
	return items, nil
}

func (db *DB) UpdateVaultItem(id int, site string, encData []byte) error {
	stmt := `UPDATE vault SET site = ?, enc_data = ?, updated_at = CURRENT_TIMESTAMP WHERE id = ?`
	_, err := db.Exec(stmt, site, encData, id)
	return err
}

func (db *DB) DeleteVaultItem(id int) error {
	stmt := `DELETE FROM vault WHERE id = ?`
	_, err := db.Exec(stmt, id)
	return err
}
