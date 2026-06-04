package db

import (
	"path/filepath"
	"testing"
)

func TestInitDBAtCreatesMigrationAndPasswordSetupSchema(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "key-box.db")

	database, err := InitDBAt(dbPath)
	if err != nil {
		t.Fatalf("InitDBAt() error = %v", err)
	}
	defer database.Close()

	assertColumnExists(t, database, "users", "password_salt")
	assertColumnExists(t, database, "users", "password_verifier")
	assertColumnExists(t, database, "users", "password_kdf")
	assertColumnExists(t, database, "users", "requires_password_setup")
	assertColumnExists(t, database, "users", "profile_updated_at")

	var migrationCount int
	if err := database.QueryRow(`SELECT COUNT(*) FROM schema_migrations WHERE version = ?`, 1).Scan(&migrationCount); err != nil {
		t.Fatalf("query schema_migrations: %v", err)
	}
	if migrationCount != 1 {
		t.Fatalf("migration version 1 count = %d, want 1", migrationCount)
	}
}

func TestInitDBAtMigrationsAreIdempotent(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "key-box.db")

	first, err := InitDBAt(dbPath)
	if err != nil {
		t.Fatalf("first InitDBAt() error = %v", err)
	}
	first.Close()

	second, err := InitDBAt(dbPath)
	if err != nil {
		t.Fatalf("second InitDBAt() error = %v", err)
	}
	defer second.Close()

	var migrationCount int
	if err := second.QueryRow(`SELECT COUNT(*) FROM schema_migrations WHERE version = ?`, 1).Scan(&migrationCount); err != nil {
		t.Fatalf("query schema_migrations: %v", err)
	}
	if migrationCount != 1 {
		t.Fatalf("migration version 1 count after rerun = %d, want 1", migrationCount)
	}
}

func TestUserPasswordVerifierLifecycle(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "key-box.db")

	database, err := InitDBAt(dbPath)
	if err != nil {
		t.Fatalf("InitDBAt() error = %v", err)
	}
	defer database.Close()

	user := &User{
		Username:  "alice",
		Salt:      []byte("answer-salt"),
		Question1: "q1",
		Question2: "q2",
		Question3: "q3",
		EncM:      []byte("enc-m"),
		EncB:      []byte("enc-b"),
		EncC:      []byte("enc-c"),
	}
	if err := database.CreateUser(user); err != nil {
		t.Fatalf("CreateUser() error = %v", err)
	}

	requiresSetup, err := database.RequiresPasswordSetup("alice")
	if err != nil {
		t.Fatalf("RequiresPasswordSetup() error = %v", err)
	}
	if !requiresSetup {
		t.Fatal("RequiresPasswordSetup() = false, want true for migrated or passwordless user")
	}

	want := &PasswordVerifier{
		Salt:     []byte("password-salt"),
		Verifier: []byte("password-verifier"),
		KDF:      "pbkdf2-sha256:test",
	}
	if err := database.SetPasswordVerifier("alice", want); err != nil {
		t.Fatalf("SetPasswordVerifier() error = %v", err)
	}

	got, err := database.GetPasswordVerifier("alice")
	if err != nil {
		t.Fatalf("GetPasswordVerifier() error = %v", err)
	}
	if string(got.Salt) != string(want.Salt) || string(got.Verifier) != string(want.Verifier) || got.KDF != want.KDF {
		t.Fatalf("GetPasswordVerifier() = %#v, want %#v", got, want)
	}

	requiresSetup, err = database.RequiresPasswordSetup("alice")
	if err != nil {
		t.Fatalf("RequiresPasswordSetup() after set error = %v", err)
	}
	if requiresSetup {
		t.Fatal("RequiresPasswordSetup() after SetPasswordVerifier = true, want false")
	}
}

func assertColumnExists(t *testing.T, database *DB, table, column string) {
	t.Helper()

	rows, err := database.Query(`PRAGMA table_info(` + table + `)`)
	if err != nil {
		t.Fatalf("PRAGMA table_info(%s): %v", table, err)
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
			t.Fatalf("scan table info: %v", err)
		}
		if name == column {
			return
		}
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("iterate table info: %v", err)
	}

	t.Fatalf("column %s.%s does not exist", table, column)
}
