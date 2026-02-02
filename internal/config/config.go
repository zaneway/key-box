package config

import (
	"os"
	"path/filepath"
)

const (
	configFileName = ".key-box.config"
	saltKey        = "SEC_APP_SALT"
)

// GetSalt 获取配置文件中的 Salt
// 优先从配置文件读取，如果不存在则尝试从环境变量读取
func GetSalt() (string, error) {
	configPath, err := getConfigPath()
	if err != nil {
		return "", err
	}

	// 如果配置文件不存在，尝试从环境变量读取
	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		if salt := os.Getenv(saltKey); salt != "" {
			// 将环境变量中的 Salt 保存到配置文件
			_ = saveSalt(configPath, salt)
			return salt, nil
		}
		return "", nil
	}

	// 从配置文件读取
	content, err := os.ReadFile(configPath)
	if err != nil {
		return "", err
	}
	return string(content), nil
}

// SaveSalt 保存 Salt 到配置文件
func SaveSalt(salt string) error {
	configPath, err := getConfigPath()
	if err != nil {
		return err
	}
	return saveSalt(configPath, salt)
}

// getConfigPath 获取配置文件路径
func getConfigPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, configFileName), nil
}

// saveSalt 保存 Salt 到指定路径
func saveSalt(path, salt string) error {
	return os.WriteFile(path, []byte(salt), 0600)
}
