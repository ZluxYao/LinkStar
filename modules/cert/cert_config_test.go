package cert

import (
	"testing"

	"linkstar/modules/cert/model"
)

// 全新安装必须自带一张自签证书。
//
// 没有这张的话，「洞口终结 TLS」「反代 HTTPS」这些开关勾了也用不了——
// TLS 规定服务器必须出示证书，一张都没有就是握手失败，
// 用户那边看到的只是「连不上」，根本联想不到是缺证书。
func TestReadConfigSeedsDefaultSelfSigned(t *testing.T) {
	t.Chdir(t.TempDir())

	cfg, err := ReadConfig()
	if err != nil {
		t.Fatalf("读配置失败: %v", err)
	}

	if len(cfg.Certificates) != 1 {
		t.Fatalf("全新安装应该只有一张垫底证书，实际 %d 张", len(cfg.Certificates))
	}
	c := cfg.Certificates[0]
	if c.Source != model.SourceSelfSigned {
		t.Errorf("垫的这张不是自签: %s", c.Source)
	}
	if !c.Enabled {
		t.Error("垫的这张没启用，等于没垫")
	}
	if !c.IsDefault {
		t.Error("垫的这张不是默认证书，SNI 落空时仍然挑不出证书")
	}
	if c.ID == 0 {
		t.Error("ID 为 0，PEM 会落到 data/cert/0/，和「没有证书」分不开")
	}
	// InitCert 靠「自签 + NotAfter 为零」认出「还没签过」，当场补签一张。
	// 这里要是带上了有效期，启动时就会去加载一份并不存在的 PEM 并报错。
	if !c.NotAfter.IsZero() {
		t.Errorf("垫的时候还没签发，NotAfter 必须是零值，实际 %s", c.NotAfter)
	}
}

// 用户把这张删掉之后不能自己长回来。
// 「删了重启又出现」比一开始就没有更让人恼火，所以只在配置文件
// 第一次被创建出来的那一次垫，之后一律照着文件里写的来。
func TestReadConfigDoesNotReseedAfterDeletion(t *testing.T) {
	t.Chdir(t.TempDir())

	if _, err := ReadConfig(); err != nil {
		t.Fatalf("首次读配置失败: %v", err)
	}
	if err := SaveConfig(model.CertConfig{Certificates: []model.Certificate{}}); err != nil {
		t.Fatalf("写空配置失败: %v", err)
	}

	cfg, err := ReadConfig()
	if err != nil {
		t.Fatalf("再次读配置失败: %v", err)
	}
	if len(cfg.Certificates) != 0 {
		t.Fatalf("删光之后又被垫回来了: %+v", cfg.Certificates)
	}
}
