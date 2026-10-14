//go:build unix

package utilsFile

import (
	"fmt"
	"os"
	"path/filepath"
)

// ProtectConfigFile 把配置文件收成仅属主可读写。
// 父目录只有在明确是项目的 config 目录时才收成 0700：目录名是 config，
// 或路径等于、以该配置目录结尾。通用 JSON 写入也会调用这里，
// 不能把 data/、临时目录或其他父目录一并改掉。
// 收紧失败必须返回：文件已经写好，但其他用户仍可能读到密钥。
func ProtectConfigFile(filePath string) error {
	if err := os.Chmod(filePath, 0600); err != nil {
		return fmt.Errorf("收紧配置文件 %s 失败，密钥可能仍可被其他用户读到: %w", filePath, err)
	}
	dir := filepath.Dir(filePath)
	if !isProjectConfigDir(dir) {
		return nil
	}
	if err := os.Chmod(dir, 0700); err != nil {
		return fmt.Errorf("收紧配置目录 %s 失败，密钥可能仍可被其他用户读到: %w", dir, err)
	}
	return nil
}

// VerifyConfigProtected 核对 Unix 权限位：文件 0600，项目 config 目录 0700。
func VerifyConfigProtected(filePath, dirPath string) error {
	info, err := os.Stat(filePath)
	if err != nil {
		return err
	}
	if info.Mode().Perm() != 0600 {
		return fmt.Errorf("配置文件 %s 权限是 %o，想要 600", filePath, info.Mode().Perm())
	}
	if dirPath == "" || !isProjectConfigDir(dirPath) {
		return nil
	}
	dirInfo, err := os.Stat(dirPath)
	if err != nil {
		return err
	}
	if dirInfo.Mode().Perm() != 0700 {
		return fmt.Errorf("配置目录 %s 权限是 %o，想要 700", dirPath, dirInfo.Mode().Perm())
	}
	return nil
}
