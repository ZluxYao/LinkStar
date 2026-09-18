package auth

import (
	"errors"
	"os"
	"strings"
	"testing"
)

// newTestRuntime 给每个用例一个独立的临时工作目录，
// WithLock 落盘会写 config/authConfig.json，别碰到真实配置。
func newTestRuntime(t *testing.T) *AuthRuntime {
	t.Helper()
	t.Chdir(t.TempDir())
	if err := os.MkdirAll("config", 0o755); err != nil {
		t.Fatalf("创建 config 目录失败：%v", err)
	}
	return &AuthRuntime{
		Config: Config{JwtSecret: randomSecret(), TokenTTLHours: defaultTokenTTLHours},
	}
}

// 改密码必须把签名密钥一起换掉，否则旧 token 还能再用 7 天，改了等于没改。
func TestChangePasswordInvalidatesOldToken(t *testing.T) {
	r := newTestRuntime(t)
	const oldPwd = "old-password-1"
	const newPwd = "new-password-2"

	if err := r.Setup(oldPwd); err != nil {
		t.Fatalf("Setup 失败：%v", err)
	}
	oldToken, err := r.Login(oldPwd)
	if err != nil {
		t.Fatalf("Login 失败：%v", err)
	}
	if !r.ValidateToken(oldToken) {
		t.Fatal("改密码前旧 token 应该是有效的")
	}

	newToken, err := r.ChangePassword(oldPwd, newPwd)
	if err != nil {
		t.Fatalf("ChangePassword 失败：%v", err)
	}
	if r.ValidateToken(oldToken) {
		t.Fatal("改密码后旧 token 必须失效")
	}
	if !r.ValidateToken(newToken) {
		t.Fatal("改密码后返回的新 token 应该立刻能用")
	}

	if _, err := r.Login(oldPwd); !errors.Is(err, ErrWrongPassword) {
		t.Fatalf("旧密码应该登不上了，实际：%v", err)
	}
	if _, err := r.Login(newPwd); err != nil {
		t.Fatalf("新密码应该能登录：%v", err)
	}
}

// 旧密码不对时什么都不能变：密码没换、token 也不能被顺带作废。
func TestChangePasswordWrongOldKeepsEverything(t *testing.T) {
	r := newTestRuntime(t)
	const pwd = "old-password-1"

	if err := r.Setup(pwd); err != nil {
		t.Fatalf("Setup 失败：%v", err)
	}
	token, err := r.Login(pwd)
	if err != nil {
		t.Fatalf("Login 失败：%v", err)
	}

	if _, err := r.ChangePassword("not-the-password", "new-password-2"); !errors.Is(err, ErrWrongPassword) {
		t.Fatalf("旧密码错误时应返回 ErrWrongPassword，实际：%v", err)
	}
	if !r.ValidateToken(token) {
		t.Fatal("旧密码校验失败不该把已有 token 踢掉")
	}
	if _, err := r.Login(pwd); err != nil {
		t.Fatalf("原密码应该还能用：%v", err)
	}
}

// 接口谁都能直接调，长度下限必须在服务端拦，不能只靠前端。
func TestPasswordLengthEnforcedOnServer(t *testing.T) {
	short := strings.Repeat("a", MinPasswordLength-1)
	ok := strings.Repeat("a", MinPasswordLength)

	t.Run("setup", func(t *testing.T) {
		r := newTestRuntime(t)
		if err := r.Setup(""); !errors.Is(err, ErrEmptyPassword) {
			t.Fatalf("空密码应返回 ErrEmptyPassword，实际：%v", err)
		}
		if err := r.Setup(short); !errors.Is(err, ErrPasswordTooShort) {
			t.Fatalf("短密码应返回 ErrPasswordTooShort，实际：%v", err)
		}
		if r.IsInitialized() {
			t.Fatal("被拒绝的 Setup 不应该把系统标成已初始化")
		}
		if err := r.Setup(ok); err != nil {
			t.Fatalf("够长的密码应该能设置：%v", err)
		}
	})

	t.Run("change", func(t *testing.T) {
		r := newTestRuntime(t)
		if err := r.Setup(ok); err != nil {
			t.Fatalf("Setup 失败：%v", err)
		}
		if _, err := r.ChangePassword(ok, short); !errors.Is(err, ErrPasswordTooShort) {
			t.Fatalf("短的新密码应返回 ErrPasswordTooShort，实际：%v", err)
		}
	})

	// 按字符数算，八个汉字也算八位，不该被字节长度糊弄过去或误杀
	t.Run("cjk", func(t *testing.T) {
		r := newTestRuntime(t)
		if err := r.Setup("密码七个字符"); !errors.Is(err, ErrPasswordTooShort) {
			t.Fatalf("6 个汉字应该算 6 位，实际：%v", err)
		}
		if err := r.Setup("我的密码是八个字"); err != nil {
			t.Fatalf("8 个汉字应该算 8 位：%v", err)
		}
	})
}
