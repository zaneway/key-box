package crypto

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha1"
	"crypto/sha256"
	"encoding/base32"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/corvus-ch/shamir"
	"golang.org/x/crypto/hkdf"
)

// FixedKeyQ is the hardcoded key q.
var FixedKeyQ = []byte("this-is-fixed-key-q-for-sec-keys-project-1234567890") // 32+ bytes

// GenerateRandomBytes generates n random bytes.
func GenerateRandomBytes(n int) ([]byte, error) {
	b := make([]byte, n)
	_, err := rand.Read(b)
	if err != nil {
		return nil, err
	}
	return b, nil
}

// NormalizeAnswer trims spaces and converts to lower case.
func NormalizeAnswer(ans string) string {
	return strings.ToLower(strings.TrimSpace(ans))
}

// DeriveKeyA 使用 SSS (Shamir's Secret Sharing) 算法从三个密保答案恢复密钥 A。
// 安全决策:
// 1. 我们不直接存储答案的 Hash，也不存储答案本身。
// 2. 我们将答案 Hash 后作为 SSS 算法中的 "Shares" (分片) 的 Y 值。
// 3. 只有当用户提供正确的三个答案时，才能合成出正确的密钥 A。
// 4. 如果合成出的 A 无法解密 M，则说明答案错误。这种设计符合“零知识”原则。
func DeriveKeyA(answers []string, salt []byte) ([]byte, error) {
	if len(answers) != 3 {
		return nil, errors.New("must provide exactly 3 answers")
	}

	shares := make(map[byte][]byte)
	for i, ans := range answers {
		// 标准化: 去空格、转小写，增加容错性
		norm := NormalizeAnswer(ans)

		// 计算分片 Y 值: Hash(Salt + Answer)
		// 使用 SHA-256 确保生成的 Y 值长度为 32 字节，满足 SSS 要求。
		h := sha256.New()
		h.Write(salt)
		h.Write([]byte(norm))
		y := h.Sum(nil) // 32 bytes

		// 设置 X 坐标为 1, 2, 3
		shares[byte(i+1)] = y
	}

	// 核心逻辑: 利用 3 个 (x, y) 点恢复多项式在 x=0 处的值 (即密钥 A)
	// 使用 corvus-ch/shamir 库进行数学运算。
	secret, err := shamir.Combine(shares)
	if err != nil {
		return nil, fmt.Errorf("failed to combine shares: %v", err)
	}
	return secret, nil
}

// EncryptAESGCM 使用 AES-GCM 算法加密数据。
// 安全决策:
// 1. 选用 GCM 模式是因为它同时提供保密性 (Encryption) 和完整性校验 (Integrity)。
// 2. 每次加密都生成随机的 Nonce，防止重放攻击和相同明文产生相同密文。
// 3. Nonce 附加在密文头部，解密时提取。
func EncryptAESGCM(key, plaintext []byte) ([]byte, error) {
	// 创建 AES Cipher Block
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}

	// 包装为 GCM 模式
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}

	// 生成随机 Nonce (Number used ONCE)
	// GCM 标准推荐 Nonce 长度为 12 字节
	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, err
	}

	// Seal 执行加密和认证标签生成
	// 结果格式: [Nonce] + [Ciphertext] + [AuthTag]
	ciphertext := gcm.Seal(nonce, nonce, plaintext, nil)
	return ciphertext, nil
}

// DecryptAESGCM 使用 AES-GCM 算法解密数据。
func DecryptAESGCM(key, ciphertext []byte) ([]byte, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}

	if len(ciphertext) < gcm.NonceSize() {
		return nil, errors.New("ciphertext too short")
	}

	// 分离 Nonce 和 密文
	nonce, ciphertext := ciphertext[:gcm.NonceSize()], ciphertext[gcm.NonceSize():]

	// Open 执行解密和认证标签校验
	// 如果 Key 错误或数据被篡改，这里会返回 error
	plaintext, err := gcm.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return nil, err
	}
	return plaintext, nil
}

// DeriveKeyB 使用 HKDF 算法从 密钥 M 和 用户名 派生出 密钥 B。
// 安全决策:
// 1. HKDF (HMAC-based Key Derivation Function) 是标准的密钥派生算法 (RFC 5869)。
// 2. 即使 M (Master Key) 泄露，通过 Info 和 Salt 隔离，可以降低不同用途密钥的相关性。
// 3. 密钥 B 被用作 "最高权限恢复凭证" 和 TOTP 种子。
func DeriveKeyB(masterKeyM []byte, username string) ([]byte, error) {
	// HKDF-SHA256
	// Secret: M (源密钥)
	// Salt: username (确保不同用户的 B 不同，即使 M 碰撞)
	// Info: "auth-key" (上下文标识)
	hkdfStream := hkdf.New(sha256.New, masterKeyM, []byte(username), []byte("auth-key"))
	keyB := make([]byte, 32) // 生成 32 字节 (AES-256 长度)
	if _, err := io.ReadFull(hkdfStream, keyB); err != nil {
		return nil, err
	}
	return keyB, nil
}

// GetRootKey 计算 RootKey = Hash(Env) XOR FixedKeyQ。
// 安全决策:
// 1. RootKey 用于加密保护存储在数据库中的 Key B。
// 2. 它不直接存储在磁盘上，而是运行时计算。
// 3. 依赖 "双因素" 因子:
//   - 因子1 (p): 环境变量 SEC_APP_SALT (用户需保密)
//   - 因子2 (q): 硬编码常量 (编译在二进制中)
//   - 即使数据库泄露，没有环境变量也无法解密 Key B。
func GetRootKey() ([]byte, error) {
	envVal := os.Getenv("SEC_APP_SALT")
	if envVal == "" {
		return nil, errors.New("environment variable SEC_APP_SALT is not set")
	}

	// 计算 p = SHA256(Env)
	h := sha256.Sum256([]byte(envVal))
	p := h[:]

	// 计算 q = SHA256(FixedKeyQ)
	// 确保 p 和 q 长度一致 (32字节)，便于异或操作
	hq := sha256.Sum256(FixedKeyQ)
	q := hq[:]

	// 计算 RootKey = p XOR q
	rootKey := make([]byte, 32)
	for i := 0; i < 32; i++ {
		rootKey[i] = p[i] ^ q[i]
	}
	return rootKey, nil
}

// GenerateTOTP generates a 6-digit TOTP code based on the secret and time.
// Uses HMAC-SHA1 and 30s step, standard TOTP (RFC 6238).
func GenerateTOTP(secretKeyB []byte, t time.Time) string {
	// TOTP usually uses base32 secret. Here we have raw bytes Key B.
	// We can use Key B directly as the HMAC key.

	// Step 1: Calculate counter
	step := int64(30)
	counter := t.Unix() / step

	// Step 2: HMAC-SHA1(Key, Counter)
	buf := make([]byte, 8)
	binary.BigEndian.PutUint64(buf, uint64(counter))

	mac := hmac.New(sha1.New, secretKeyB)
	mac.Write(buf)
	sum := mac.Sum(nil)

	// Step 3: Dynamic Truncation
	offset := sum[len(sum)-1] & 0xf
	binCode := binary.BigEndian.Uint32(sum[offset : offset+4])
	binCode &= 0x7fffffff

	// Step 4: Modulo
	otp := binCode % 1000000

	return fmt.Sprintf("%06d", otp)
}

// VerifyOTP verifies the input code against current time and current time - 30s.
func VerifyOTP(secretKeyB []byte, inputCode string) bool {
	now := time.Now()
	if GenerateTOTP(secretKeyB, now) == inputCode {
		return true
	}
	// Check previous window (30s ago)
	if GenerateTOTP(secretKeyB, now.Add(-30*time.Second)) == inputCode {
		return true
	}
	return false
}

// EncodeKeyB returns base64 string of Key B for user display.
func EncodeKeyB(keyB []byte) string {
	return base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString(keyB)
}
