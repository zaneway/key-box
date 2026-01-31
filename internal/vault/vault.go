package vault

import (
	"encoding/json"
	"fmt"

	"sec-keys/internal/crypto"
	"sec-keys/internal/db"
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
