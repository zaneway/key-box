package vault

import (
	"encoding/json"
	"fmt"
	"strings"

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
	Remark   string `json:"remark,omitempty"`
}

type ItemInput struct {
	Title    string
	Site     string
	URL      string
	Category string
	Username string
	Password string
	Remark   string
	Favorite bool
}

type ItemFilter struct {
	Search   string
	Category string
}

type VaultItem struct {
	ID        int
	Title     string
	Site      string
	URL       string
	Category  string
	Username  string
	Password  string
	Remark    string
	Favorite  bool
	UpdatedAt string
}

// AddItem 加密并存储一个新的密码条目。
// 核心逻辑:
//  1. 将明文数据 (用户名、密码) 序列化为 JSON。
//  2. 使用 Key C 对 JSON 数据进行 AES-GCM 加密。
//     注意: Key C 是数据专用密钥，只有在用户登录并通过 TOTP 验证后才能获取。
//  3. 将加密后的 Blob 和明文索引 (Site) 存储到数据库。
func (m *Manager) AddItem(username string, keyC []byte, site, itemUser, itemPass string) error {
	return m.AddDetailedItem(username, keyC, ItemInput{
		Title:    site,
		Site:     site,
		Category: "未分类",
		Username: itemUser,
		Password: itemPass,
	})
}

func (m *Manager) AddDetailedItem(username string, keyC []byte, input ItemInput) error {
	input = normalizeInput(input)
	data := ItemData{
		Username: input.Username,
		Password: input.Password,
		Remark:   input.Remark,
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

	return m.db.SaveVaultItemWithMeta(username, input.Title, input.Site, input.URL, input.Category, input.Favorite, encData)
}

// ListItems 读取并解密所有密码条目。
// 核心逻辑:
// 1. 从数据库获取该用户的所有加密条目。
// 2. 使用传入的 Key C 逐个解密。
// 3. 如果解密失败 (例如数据损坏或密钥错误)，返回错误。
// 4. 反序列化 JSON 得到明文。
func (m *Manager) ListItems(username string, keyC []byte) ([]VaultItem, error) {
	return m.ListItemsFiltered(username, keyC, ItemFilter{})
}

func (m *Manager) ListItemsFiltered(username string, keyC []byte, filter ItemFilter) ([]VaultItem, error) {
	rows, err := m.db.GetVaultItems(username)
	if err != nil {
		return nil, err
	}

	var results []VaultItem
	search := strings.ToLower(strings.TrimSpace(filter.Search))
	category := strings.TrimSpace(filter.Category)
	for _, row := range rows {
		if category != "" && category != "全部" && row.Category != category {
			continue
		}
		// 解密数据
		decrypted, err := crypto.DecryptAESGCM(keyC, row.EncData)
		if err != nil {
			return nil, fmt.Errorf("failed to decrypt item %d: %v", row.ID, err)
		}

		var data ItemData
		if err := json.Unmarshal(decrypted, &data); err != nil {
			return nil, fmt.Errorf("failed to unmarshal item %d: %v", row.ID, err)
		}

		item := VaultItem{
			ID:        row.ID,
			Title:     row.Title,
			Site:      row.Site,
			URL:       row.URL,
			Category:  row.Category,
			Username:  data.Username,
			Password:  data.Password,
			Remark:    data.Remark,
			Favorite:  row.Favorite,
			UpdatedAt: row.UpdatedAt,
		}
		if search != "" && !matchesSearch(item, search) {
			continue
		}
		results = append(results, item)
	}
	return results, nil
}

// UpdateItem 更新已存储的密码条目。
// 核心逻辑:
// 1. 将新的明文数据序列化为 JSON。
// 2. 使用 Key C 加密。
// 3. 更新数据库记录。
func (m *Manager) UpdateItem(keyC []byte, id int, site, itemUser, itemPass string) error {
	return m.UpdateDetailedItem(keyC, id, ItemInput{
		Title:    site,
		Site:     site,
		Category: "未分类",
		Username: itemUser,
		Password: itemPass,
	})
}

func (m *Manager) UpdateDetailedItem(keyC []byte, id int, input ItemInput) error {
	input = normalizeInput(input)
	data := ItemData{
		Username: input.Username,
		Password: input.Password,
		Remark:   input.Remark,
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

	return m.db.UpdateVaultItemWithMeta(id, input.Title, input.Site, input.URL, input.Category, input.Favorite, encData)
}

// DeleteItem 删除已存储的密码条目。
func (m *Manager) DeleteItem(id int) error {
	return m.db.DeleteVaultItem(id)
}

func (m *Manager) ListCategories(username string) ([]string, error) {
	return m.db.ListVaultCategories(username)
}

func (m *Manager) RenameCategory(username, oldCategory, newCategory string) error {
	oldCategory = strings.TrimSpace(oldCategory)
	newCategory = strings.TrimSpace(newCategory)
	if oldCategory == "" || oldCategory == "未分类" {
		return fmt.Errorf("cannot rename category %q", oldCategory)
	}
	if newCategory == "" {
		return fmt.Errorf("new category cannot be empty")
	}
	return m.db.RenameVaultCategory(username, oldCategory, newCategory)
}

func (m *Manager) DeleteCategory(username, category string) error {
	category = strings.TrimSpace(category)
	if category == "" || category == "未分类" {
		return fmt.Errorf("cannot delete category %q", category)
	}
	return m.db.MoveVaultCategoryToDefault(username, category)
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

func normalizeInput(input ItemInput) ItemInput {
	input.Title = strings.TrimSpace(input.Title)
	input.Site = strings.TrimSpace(input.Site)
	input.URL = strings.TrimSpace(input.URL)
	input.Category = strings.TrimSpace(input.Category)
	input.Username = strings.TrimSpace(input.Username)
	input.Remark = strings.TrimSpace(input.Remark)
	if input.Site == "" {
		input.Site = input.Title
	}
	if input.Title == "" {
		input.Title = input.Site
	}
	if input.Category == "" {
		input.Category = "未分类"
	}
	return input
}

func matchesSearch(item VaultItem, search string) bool {
	values := []string{item.Title, item.Site, item.URL, item.Category, item.Username, item.Remark}
	for _, value := range values {
		if strings.Contains(strings.ToLower(value), search) {
			return true
		}
	}
	return false
}
