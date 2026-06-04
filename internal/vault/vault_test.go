package vault

import (
	"bytes"
	"path/filepath"
	"testing"

	"key-box/internal/crypto"
	"key-box/internal/db"
)

func TestAddItemStoresEncryptedDataAndListDecryptsIt(t *testing.T) {
	database, err := db.InitDBAt(filepath.Join(t.TempDir(), "key-box.db"))
	if err != nil {
		t.Fatalf("InitDBAt() error = %v", err)
	}
	defer database.Close()

	keyC, err := crypto.GenerateRandomBytes(32)
	if err != nil {
		t.Fatalf("GenerateRandomBytes() error = %v", err)
	}

	manager := NewManager(database)
	if err := manager.AddItem("alice", keyC, "GitHub", "alice@example.com", "secret-password"); err != nil {
		t.Fatalf("AddItem() error = %v", err)
	}

	encryptedItems, err := manager.GetEncryptedItems("alice")
	if err != nil {
		t.Fatalf("GetEncryptedItems() error = %v", err)
	}
	if len(encryptedItems) != 1 {
		t.Fatalf("len(encryptedItems) = %d, want 1", len(encryptedItems))
	}
	if bytes.Contains(encryptedItems[0].EncData, []byte("alice@example.com")) {
		t.Fatal("encrypted data contains plaintext username")
	}
	if bytes.Contains(encryptedItems[0].EncData, []byte("secret-password")) {
		t.Fatal("encrypted data contains plaintext password")
	}

	items, err := manager.ListItems("alice", keyC)
	if err != nil {
		t.Fatalf("ListItems() error = %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("len(items) = %d, want 1", len(items))
	}
	if items[0].Site != "GitHub" || items[0].Username != "alice@example.com" || items[0].Password != "secret-password" {
		t.Fatalf("ListItems()[0] = %#v, want original item", items[0])
	}
}

func TestAddDetailedItemStoresMetadataAndSearches(t *testing.T) {
	database, err := db.InitDBAt(filepath.Join(t.TempDir(), "key-box.db"))
	if err != nil {
		t.Fatalf("InitDBAt() error = %v", err)
	}
	defer database.Close()

	keyC, err := crypto.GenerateRandomBytes(32)
	if err != nil {
		t.Fatalf("GenerateRandomBytes() error = %v", err)
	}

	manager := NewManager(database)
	if err := manager.AddDetailedItem("alice", keyC, ItemInput{
		Title:    "GitHub Work",
		Site:     "GitHub",
		URL:      "https://github.com",
		Category: "工作",
		Username: "alice@example.com",
		Password: "secret-password",
		Favorite: true,
	}); err != nil {
		t.Fatalf("AddDetailedItem() error = %v", err)
	}
	if err := manager.AddDetailedItem("alice", keyC, ItemInput{
		Title:    "Bank",
		Site:     "Example Bank",
		URL:      "https://bank.example",
		Category: "财务",
		Username: "alice-bank",
		Password: "bank-password",
	}); err != nil {
		t.Fatalf("AddDetailedItem(second) error = %v", err)
	}

	workItems, err := manager.ListItemsFiltered("alice", keyC, ItemFilter{Category: "工作"})
	if err != nil {
		t.Fatalf("ListItemsFiltered(category) error = %v", err)
	}
	if len(workItems) != 1 || workItems[0].Title != "GitHub Work" || workItems[0].URL != "https://github.com" || !workItems[0].Favorite {
		t.Fatalf("workItems = %#v, want GitHub Work favorite item", workItems)
	}

	searchItems, err := manager.ListItemsFiltered("alice", keyC, ItemFilter{Search: "bank"})
	if err != nil {
		t.Fatalf("ListItemsFiltered(search) error = %v", err)
	}
	if len(searchItems) != 1 || searchItems[0].Title != "Bank" {
		t.Fatalf("searchItems = %#v, want Bank item", searchItems)
	}
}

func TestLegacyAddItemGetsDefaultMetadata(t *testing.T) {
	database, err := db.InitDBAt(filepath.Join(t.TempDir(), "key-box.db"))
	if err != nil {
		t.Fatalf("InitDBAt() error = %v", err)
	}
	defer database.Close()

	keyC, err := crypto.GenerateRandomBytes(32)
	if err != nil {
		t.Fatalf("GenerateRandomBytes() error = %v", err)
	}

	manager := NewManager(database)
	if err := manager.AddItem("alice", keyC, "GitHub", "alice@example.com", "secret-password"); err != nil {
		t.Fatalf("AddItem() error = %v", err)
	}

	items, err := manager.ListItems("alice", keyC)
	if err != nil {
		t.Fatalf("ListItems() error = %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("len(items) = %d, want 1", len(items))
	}
	if items[0].Title != "GitHub" || items[0].Category != "未分类" {
		t.Fatalf("legacy metadata = title %q category %q, want GitHub / 未分类", items[0].Title, items[0].Category)
	}
}

func TestCategoryManagementRenamesAndDeletesCategories(t *testing.T) {
	database, err := db.InitDBAt(filepath.Join(t.TempDir(), "key-box.db"))
	if err != nil {
		t.Fatalf("InitDBAt() error = %v", err)
	}
	defer database.Close()

	keyC, err := crypto.GenerateRandomBytes(32)
	if err != nil {
		t.Fatalf("GenerateRandomBytes() error = %v", err)
	}

	manager := NewManager(database)
	for _, input := range []ItemInput{
		{Title: "GitHub", Site: "GitHub", Category: "工作", Username: "alice", Password: "p1"},
		{Title: "GitLab", Site: "GitLab", Category: "工作", Username: "alice", Password: "p2"},
		{Title: "Bank", Site: "Bank", Category: "财务", Username: "alice", Password: "p3"},
	} {
		if err := manager.AddDetailedItem("alice", keyC, input); err != nil {
			t.Fatalf("AddDetailedItem(%s) error = %v", input.Title, err)
		}
	}

	categories, err := manager.ListCategories("alice")
	if err != nil {
		t.Fatalf("ListCategories() error = %v", err)
	}
	assertStringSet(t, categories, []string{"工作", "财务"})

	if err := manager.RenameCategory("alice", "工作", "开发"); err != nil {
		t.Fatalf("RenameCategory() error = %v", err)
	}
	devItems, err := manager.ListItemsFiltered("alice", keyC, ItemFilter{Category: "开发"})
	if err != nil {
		t.Fatalf("ListItemsFiltered(开发) error = %v", err)
	}
	if len(devItems) != 2 {
		t.Fatalf("len(devItems) = %d, want 2", len(devItems))
	}

	if err := manager.DeleteCategory("alice", "开发"); err != nil {
		t.Fatalf("DeleteCategory() error = %v", err)
	}
	defaultItems, err := manager.ListItemsFiltered("alice", keyC, ItemFilter{Category: "未分类"})
	if err != nil {
		t.Fatalf("ListItemsFiltered(未分类) error = %v", err)
	}
	if len(defaultItems) != 2 {
		t.Fatalf("len(defaultItems) = %d, want 2", len(defaultItems))
	}
}

func assertStringSet(t *testing.T, got, want []string) {
	t.Helper()

	seen := make(map[string]bool, len(got))
	for _, value := range got {
		seen[value] = true
	}
	for _, value := range want {
		if !seen[value] {
			t.Fatalf("categories = %#v, missing %q", got, value)
		}
	}
}
