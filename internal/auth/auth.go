package auth

import (
	"errors"
	"fmt"

	"sec-keys/internal/crypto"
	"sec-keys/internal/db"
)

type Service struct {
	db *db.DB
}

func NewService(db *db.DB) *Service {
	return &Service{db: db}
}

// RegisterResult contains the secret key B for the user to save.
type RegisterResult struct {
	SecretKeyBBase32 string
}

// Register 用户注册流程。
// 核心逻辑:
// 1. 生成用户专属的 Salt，用于密保答案处理。
// 2. 利用 SSS 算法，通过 3 个密保答案生成 密钥 A。
// 3. 生成随机 密钥 M (Master Key) 和 密钥 C (Data Key)。
// 4. 用 密钥 A 加密 M -> EncM (存储到 DB)。
// 5. 用 M 派生出 密钥 B (Auth Key)。
// 6. 用 RootKey 加密 B -> EncB (存储到 DB)。
// 7. 用 B 加密 C -> EncC (存储到 DB)。
// 8. 返回 Key B 的 Base32 编码供用户绑定 TOTP。
func (s *Service) Register(username, q1, q2, q3, a1, a2, a3 string) (*RegisterResult, error) {
	// Check if user exists
	if _, err := s.db.GetUser(username); err == nil {
		return nil, errors.New("user already exists")
	}

	// 1. 生成随机盐值 (Salt)
	// 这个 Salt 是公开的 (存储在 DB)，用于混淆 SSS 的分片 Hash 计算。
	salt, err := crypto.GenerateRandomBytes(16)
	if err != nil {
		return nil, err
	}

	// 2. 从答案派生 Key A
	// 这是整个链条的起点。只有知道全部三个答案才能恢复 A。
	answers := []string{a1, a2, a3}
	keyA, err := crypto.DeriveKeyA(answers, salt)
	if err != nil {
		return nil, err
	}

	// 3. 生成随机密钥 M 和 C
	// M 是用户的主密钥，C 是加密数据的实际密钥。
	keyM, err := crypto.GenerateRandomBytes(32)
	if err != nil {
		return nil, err
	}
	keyC, err := crypto.GenerateRandomBytes(32)
	if err != nil {
		return nil, err
	}

	// 4. 用 A 加密 M
	// 这样，只要能恢复 A (即回答对问题)，就能解密得到 M。
	encM, err := crypto.EncryptAESGCM(keyA, keyM)
	if err != nil {
		return nil, err
	}

	// 5. 从 M 派生 Key B
	// Key B 用于认证 (TOTP) 和保护 Key C。
	keyB, err := crypto.DeriveKeyB(keyM, username)
	if err != nil {
		return nil, err
	}

	// 6. 获取 Root Key
	// Root Key 用于保护 Key B 存储在数据库中。
	rootKey, err := crypto.GetRootKey()
	if err != nil {
		return nil, fmt.Errorf("root key error: %v (did you set SEC_APP_SALT?)", err)
	}

	// 7. 用 Root Key 加密 Key B
	// 即使 DB 泄露，攻击者没有 Root Key (环境变量+硬编码) 也无法获取 B。
	encB, err := crypto.EncryptAESGCM(rootKey, keyB)
	if err != nil {
		return nil, err
	}

	// 8. 用 Key B 加密 Key C
	// Key C 是数据加密密钥，平时是被 B 保护的。
	// 只有通过 TOTP 验证持有 Key B 后，才能解密得到 C。
	encC, err := crypto.EncryptAESGCM(keyB, keyC)
	if err != nil {
		return nil, err
	}

	// 9. 保存所有密文和元数据到数据库
	u := &db.User{
		Username:  username,
		Salt:      salt,
		Question1: q1,
		Question2: q2,
		Question3: q3,
		EncM:      encM,
		EncB:      encB,
		EncC:      encC,
	}

	if err := s.db.CreateUser(u); err != nil {
		return nil, err
	}

	return &RegisterResult{
		SecretKeyBBase32: crypto.EncodeKeyB(keyB),
	}, nil
}

func (s *Service) GetSecurityQuestions(username string) ([]string, error) {
	u, err := s.db.GetUser(username)
	if err != nil {
		return nil, err
	}
	return []string{u.Question1, u.Question2, u.Question3}, nil
}

// Login 用户登录流程。
// 核心逻辑:
// 1. 从 DB 读取用户的加密元数据。
// 2. 计算 RootKey 并解密得到 Key B。
// 3. 使用 Key B 验证用户输入的 TOTP。
// 4. 验证通过后，用 Key B 解密得到 Key C (数据密钥)。
// 5. 返回 Key C 供后续操作使用。
func (s *Service) Login(username, code string) ([]byte, error) {
	u, err := s.db.GetUser(username)
	if err != nil {
		return nil, errors.New("user not found")
	}

	// 1. 获取 Root Key
	rootKey, err := crypto.GetRootKey()
	if err != nil {
		return nil, err
	}

	// 2. 解密 Key B
	// 如果环境变量配置错误，RootKey 会变，解密 B 将失败 (GCM Auth Tag 校验失败)。
	keyB, err := crypto.DecryptAESGCM(rootKey, u.EncB)
	if err != nil {
		return nil, errors.New("failed to decrypt system key (root key mismatch?)")
	}

	// 3. 验证 TOTP
	// 证明用户持有 Key B (即 "最高权限凭证")。
	if !crypto.VerifyOTP(keyB, code) {
		return nil, errors.New("invalid OTP code")
	}

	// 4. 解密 Key C
	// 只有通过了上述步骤，才能拿到解密用户数据的钥匙。
	keyC, err := crypto.DecryptAESGCM(keyB, u.EncC)
	if err != nil {
		return nil, errors.New("failed to unlock vault (key C decryption failed)")
	}

	return keyC, nil
}

// ResetPassword 密码重置/密钥轮转流程。
// 核心逻辑:
// 1. 验证密保答案，恢复 Key A。
// 2. 用 A 解密得到旧的 M。
// 3. 用旧 M 派生出旧 B，进而解密得到 C (数据密钥)。
// 4. 生成全新的随机密钥 M_new (Key Rotation)。
// 5. 用 M_new 派生新 B_new。
// 6. 重新加密链条: A->M_new, RootKey->B_new, B_new->C。
// 7. 更新数据库。
// 结果: 用户获得新的 Key B，旧的 Key B 失效。数据本身 (由 C 加密) 无需重加密，只需重新保护 C。
func (s *Service) ResetPassword(username, a1, a2, a3 string) (*RegisterResult, error) {
	u, err := s.db.GetUser(username)
	if err != nil {
		return nil, errors.New("user not found")
	}

	// 1. 恢复 Key A
	answers := []string{a1, a2, a3}
	keyA, err := crypto.DeriveKeyA(answers, u.Salt)
	if err != nil {
		return nil, err
	}

	// 2. 解密 M (验证答案正确性)
	// 如果答案错误，Key A 错误，解密 M 必然失败。
	keyM, err := crypto.DecryptAESGCM(keyA, u.EncM)
	if err != nil {
		return nil, errors.New("failed to recover Key M (wrong security answers)")
	}

	// 3. 恢复旧 Key B
	// 需要旧 B 来解密 Key C。
	oldKeyB, err := crypto.DeriveKeyB(keyM, username)
	if err != nil {
		return nil, err
	}

	// 4. 解密 Key C (数据密钥)
	keyC, err := crypto.DecryptAESGCM(oldKeyB, u.EncC)
	if err != nil {
		return nil, errors.New("failed to recover Key C")
	}

	// 5. 生成新 M (实现密钥轮转)
	// 按照需求，我们需要 "再次生成随机数后获得新的密钥B"。
	// 通过轮转 M，我们可以彻底切断与旧密钥链的联系。
	newKeyM, err := crypto.GenerateRandomBytes(32)
	if err != nil {
		return nil, err
	}

	// 6. 重新加密新 M (用同一个 A)
	newEncM, err := crypto.EncryptAESGCM(keyA, newKeyM)
	if err != nil {
		return nil, err
	}

	// 7. 派生新 Key B
	newKeyB, err := crypto.DeriveKeyB(newKeyM, username)
	if err != nil {
		return nil, err
	}

	// 8. 用 Root Key 加密新 B
	rootKey, err := crypto.GetRootKey()
	if err != nil {
		return nil, err
	}
	newEncB, err := crypto.EncryptAESGCM(rootKey, newKeyB)
	if err != nil {
		return nil, err
	}

	// 9. 用新 B 重新加密 Key C
	// 这样 Key C 就被新 B 保护了。
	newEncC, err := crypto.EncryptAESGCM(newKeyB, keyC)
	if err != nil {
		return nil, err
	}

	// 10. 更新数据库记录
	stmt := `UPDATE users SET enc_m=?, enc_b=?, enc_c=? WHERE username=?`
	_, err = s.db.Exec(stmt, newEncM, newEncB, newEncC, username)
	if err != nil {
		return nil, err
	}

	return &RegisterResult{
		SecretKeyBBase32: crypto.EncodeKeyB(newKeyB),
	}, nil
}
