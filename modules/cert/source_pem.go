package cert

import (
	"crypto/tls"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"os"
	"strings"
	"time"

	"linkstar/modules/cert/model"
)

// loadFromDisk 从磁盘读取一张证书并装入 Manager。
// upload / acme-* 读 data/cert/{id}/，path 读用户填写的路径。
func loadFromDisk(c model.Certificate) error {
	certPath, keyPath := effectivePaths(c)
	if certPath == "" || keyPath == "" {
		return fmt.Errorf("证书或私钥路径为空")
	}

	pair, err := tls.LoadX509KeyPair(certPath, keyPath)
	if err != nil {
		return fmt.Errorf("加载证书失败: %w", err)
	}
	if err := parseLeaf(&pair); err != nil {
		return err
	}

	certMtime, _ := fileMtime(certPath)
	keyMtime, _ := fileMtime(keyPath)

	Runtime.Manager.Store(c.ID, &pair, c, certPath, keyPath, certMtime, keyMtime)
	return nil
}

// loadAndCommit 加载证书并把有效期/签发者/错误写回配置
func loadAndCommit(c model.Certificate) error {
	err := loadFromDisk(c)
	if err != nil {
		c.LastError = err.Error()
		Runtime.commitStatus(c)
		return err
	}

	if pair, lookupErr := Runtime.Manager.Lookup(c.ID, ""); lookupErr == nil && pair.Leaf != nil {
		c.NotBefore = pair.Leaf.NotBefore
		c.NotAfter = pair.Leaf.NotAfter
		c.Issuer = pair.Leaf.Issuer.CommonName
		if len(pair.Leaf.DNSNames) > 0 {
			c.Domains = normalizeDomains(pair.Leaf.DNSNames)
		}
	}
	c.LastError = ""
	Runtime.commitStatus(c)
	return nil
}

// writePEM 把上传/签发得到的 PEM 写入 data/cert/{id}/
func writePEM(id uint, certPEM, keyPEM []byte) error {
	if _, err := ensureCertDir(id); err != nil {
		return err
	}
	certPath, keyPath := managedPaths(id)

	if err := os.WriteFile(certPath, certPEM, 0644); err != nil {
		return fmt.Errorf("写入证书失败: %w", err)
	}
	// 私钥只给属主读写
	if err := os.WriteFile(keyPath, keyPEM, 0600); err != nil {
		return fmt.Errorf("写入私钥失败: %w", err)
	}
	return nil
}

// ValidatePEM 校验一对 PEM 能否组成可用的证书，并返回解析后的叶子证书
func ValidatePEM(certPEM, keyPEM []byte) (*x509.Certificate, error) {
	pair, err := tls.X509KeyPair(certPEM, keyPEM)
	if err != nil {
		return nil, fmt.Errorf("证书与私钥不匹配或格式错误: %w", err)
	}
	if err := parseLeaf(&pair); err != nil {
		return nil, err
	}
	return pair.Leaf, nil
}

// looksLikePEM 粗筛，给上传接口返回更可读的错误
func looksLikePEM(data []byte) bool {
	block, _ := pem.Decode(data)
	return block != nil
}

func fileMtime(p string) (time.Time, error) {
	fi, err := os.Stat(p)
	if err != nil {
		return time.Time{}, err
	}
	return fi.ModTime(), nil
}

// checkPathReload 比对 path 来源证书的文件 mtime，有变化就重新加载。
// 这样外部 certbot / acme.sh 续期后无需重启 LinkStar，也不会断开 STUN 洞。
func checkPathReload(c model.Certificate) {
	certPath, keyPath, oldCert, oldKey, ok := Runtime.Manager.mtimes(c.ID)
	if !ok {
		// 还没加载成功过，直接尝试加载
		if err := loadAndCommit(c); err != nil {
			logCertError(c, "加载失败", err)
		}
		return
	}

	newCert, err1 := fileMtime(certPath)
	newKey, err2 := fileMtime(keyPath)
	if err1 != nil || err2 != nil {
		return // 文件暂时不可读（续期过程中），下一轮再看
	}
	if newCert.Equal(oldCert) && newKey.Equal(oldKey) {
		return
	}

	if err := loadAndCommit(c); err != nil {
		logCertError(c, "热重载失败", err)
		return
	}
	logCertInfo(c, "检测到文件变更，已热重载")
}

// pemKeyType 供上传接口给出更友好的提示
func pemKeyType(keyPEM []byte) string {
	block, _ := pem.Decode(keyPEM)
	if block == nil {
		return ""
	}
	return strings.ToUpper(block.Type)
}
