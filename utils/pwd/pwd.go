package pwd

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"

	"github.com/pbnjay/memory"
	"golang.org/x/crypto/argon2"
	"golang.org/x/crypto/bcrypt"
)

// 内存阈值：>=512MB 用 argon2id，否则 bcrypt。argon2id 单次哈希吃约 64MB，
// 低内存路由器（OpenWrt 等）并发登录会 OOM，故降级到内存占用极小的 bcrypt。
const argon2MemoryThreshold = 512 * 1024 * 1024

// argon2id 参数（OWASP 推荐均衡配置：m=19MiB, t=2, p=1）
const (
	argon2Time    = 2
	argon2Memory  = 19 * 1024 // 19MiB
	argon2Threads = 1
	argon2KeyLen  = 32
	argon2SaltLen = 16
)

// useArgon2 在包初始化时计算一次并缓存：总内存达标就用 argon2id。
var useArgon2 = memory.TotalMemory() >= argon2MemoryThreshold

// HashPassword 按当前机器内存自动选择算法生成密码哈希。
func HashPassword(password string) (string, error) {
	if useArgon2 {
		return hashArgon2(password)
	}
	return GenerateFromPassword(password)
}

// VerifyPassword 按哈希串前缀自动识别算法并校验。
func VerifyPassword(hash, password string) bool {
	if strings.HasPrefix(hash, "$argon2id$") {
		return verifyArgon2(hash, password)
	}
	return ComparePasswordsHash(hash, password)
}

// hashArgon2 生成标准编码的 argon2id 哈希串。
func hashArgon2(password string) (string, error) {
	salt := make([]byte, argon2SaltLen)
	if _, err := rand.Read(salt); err != nil {
		return "", err
	}
	key := argon2.IDKey([]byte(password), salt, argon2Time, argon2Memory, argon2Threads, argon2KeyLen)
	b64 := base64.RawStdEncoding
	return fmt.Sprintf(
		"$argon2id$v=%d$m=%d,t=%d,p=%d$%s$%s",
		argon2.Version, argon2Memory, argon2Time, argon2Threads,
		b64.EncodeToString(salt), b64.EncodeToString(key),
	), nil
}

// verifyArgon2 解析 argon2id 哈希串并用相同参数校验。
func verifyArgon2(hash, password string) bool {
	salt, key, mem, t, threads, err := decodeArgon2(hash)
	if err != nil {
		return false
	}
	got := argon2.IDKey([]byte(password), salt, t, mem, threads, uint32(len(key)))
	return subtle.ConstantTimeCompare(got, key) == 1
}

func decodeArgon2(hash string) (salt, key []byte, mem uint32, t uint32, threads uint8, err error) {
	parts := strings.Split(hash, "$")
	// ["", "argon2id", "v=19", "m=..,t=..,p=..", salt, key]
	if len(parts) != 6 || parts[1] != "argon2id" {
		return nil, nil, 0, 0, 0, errors.New("invalid argon2 hash")
	}
	var version int
	if _, err = fmt.Sscanf(parts[2], "v=%d", &version); err != nil {
		return nil, nil, 0, 0, 0, err
	}
	if _, err = fmt.Sscanf(parts[3], "m=%d,t=%d,p=%d", &mem, &t, &threads); err != nil {
		return nil, nil, 0, 0, 0, err
	}
	b64 := base64.RawStdEncoding
	if salt, err = b64.DecodeString(parts[4]); err != nil {
		return nil, nil, 0, 0, 0, err
	}
	if key, err = b64.DecodeString(parts[5]); err != nil {
		return nil, nil, 0, 0, 0, err
	}
	return salt, key, mem, t, threads, nil
}

// GenerateFromPassword 使用 Bcrypt 算法生成密码哈希值
func GenerateFromPassword(password string) (string, error) {
	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return "", err
	}
	return string(hashedPassword), nil
}

// ComparePasswordsHash 比较输入的密码与哈希值是否匹配
func ComparePasswordsHash(hashedPassword, password string) bool {
	err := bcrypt.CompareHashAndPassword([]byte(hashedPassword), []byte(password))
	return err == nil
}
