package vault

import (
	"encoding/json"
	"fmt"

	"key-box/internal/crypto"
	"key-box/internal/db"
)

type Manager struct {
	db *db.DB
}

func NewManager(db *db.DB) *Manager {
	return &Manager{db: db}
}

type ItemData struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

type VaultItem struct {
	ID       int
	Site     string
	Username string
	Password string
}

// AddItem 加密并存储一个新的密码条目。
// 核心逻辑:
//  1. 将明文数据 (用户名、密码) 序列化为 JSON。
//  2. 使用 Key C 对 JSON 数据进行 AES-GCM 加密。
//     注意: Key C 是数据专用密钥，只有在用户登录并通过 TOTP 验证后才能获取。
//  3. 将加密后的 Blob 和明文索引 (Site) 存储到数据库。
func (m *Manager) AddItem(username string, keyC []byte, site, itemUser, itemPass string) error {
	data := ItemData{
		Username: itemUser,
		Password: itemPass,
	}
	jsonData, err := json.Marshal(data)
	if err != nil {
		return err
	}

	// 使用 Key C 加密实际数据
	encData, err := crypto.EncryptAESGCM(keyC, jsonData)
	if err != nil {
		return err
	}

	return m.db.SaveVaultItem(username, site, encData)
}

// ListItems 读取并解密所有密码条目。
// 核心逻辑:
// 1. 从数据库获取该用户的所有加密条目。
// 2. 使用传入的 Key C 逐个解密。
// 3. 如果解密失败 (例如数据损坏或密钥错误)，返回错误。
// 4. 反序列化 JSON 得到明文。
func (m *Manager) ListItems(username string, keyC []byte) ([]VaultItem, error) {
	rows, err := m.db.GetVaultItems(username)
	if err != nil {
		return nil, err
	}

	var results []VaultItem
	for _, row := range rows {
		// 解密数据
		decrypted, err := crypto.DecryptAESGCM(keyC, row.EncData)
		if err != nil {
			return nil, fmt.Errorf("failed to decrypt item %d: %v", row.ID, err)
		}

		var data ItemData
		if err := json.Unmarshal(decrypted, &data); err != nil {
			return nil, fmt.Errorf("failed to unmarshal item %d: %v", row.ID, err)
		}

		results = append(results, VaultItem{
			ID:       row.ID,
			Site:     row.Site,
			Username: data.Username,
			Password: data.Password,
		})
	}
	return results, nil
}

// UpdateItem 更新已存储的密码条目。
// 核心逻辑:
// 1. 将新的明文数据序列化为 JSON。
// 2. 使用 Key C 加密。
// 3. 更新数据库记录。
func (m *Manager) UpdateItem(keyC []byte, id int, site, itemUser, itemPass string) error {
	data := ItemData{
		Username: itemUser,
		Password: itemPass,
	}
	jsonData, err := json.Marshal(data)
	if err != nil {
		return err
	}

	// 使用 Key C 加密实际数据
	encData, err := crypto.EncryptAESGCM(keyC, jsonData)
	if err != nil {
		return err
	}

	return m.db.UpdateVaultItem(id, site, encData)
}

// DeleteItem 删除已存储的密码条目。
func (m *Manager) DeleteItem(id int) error {
	return m.db.DeleteVaultItem(id)
}

// DeleteAllItems 删除用户的所有密码条目（用于覆盖恢复）
func (m *Manager) DeleteAllItems(username string) error {
	stmt := `DELETE FROM vault WHERE username = ?`
	_, err := m.db.Exec(stmt, username)
	return err
}

// GetEncryptedItems 获取加密的密码条目（用于备份）
func (m *Manager) GetEncryptedItems(username string) ([]db.VaultItem, error) {
	return m.db.GetVaultItems(username)
}

// RestoreEncryptedItem 恢复加密的密码条目（用于恢复备份）
func (m *Manager) RestoreEncryptedItem(username, site string, encData []byte) error {
	return m.db.SaveVaultItem(username, site, encData)
}
