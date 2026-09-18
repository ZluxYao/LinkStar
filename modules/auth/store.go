package auth

import (
	"errors"
	"fmt"
	"time"
	"unicode/utf8"

	"linkstar/utils/jwt"
	"linkstar/utils/pwd"
)

// MinPasswordLength 密码最短长度。
// 这里没有用户名，别人想进来只能一串一串地试这个密码，
// 所以长度基本就是全部的门槛，短密码等于没设。
const MinPasswordLength = 8

var (
	ErrNotInitialized   = errors.New("尚未初始化")
	ErrAlreadyInit      = errors.New("已初始化")
	ErrWrongPassword    = errors.New("密码错误")
	ErrEmptyPassword    = errors.New("密码不能为空")
	ErrPasswordTooShort = fmt.Errorf("密码至少 %d 位", MinPasswordLength)
	ErrPasswordBusy     = errors.New("密码校验繁忙，请稍后重试")
	ErrLoginLimited     = errors.New("登录失败次数过多，请稍后重试")
)

// checkNewPassword 设置密码和修改密码共用的下限检查。
// 前端也会拦一道，但接口谁都能直接调，服务端必须自己拦。
// 按字符数不按字节数，八个汉字也算八位。
func checkNewPassword(password string) error {
	if password == "" {
		return ErrEmptyPassword
	}
	if utf8.RuneCountInString(password) < MinPasswordLength {
		return ErrPasswordTooShort
	}
	return nil
}

// WithLock 在写锁内修改配置并持久化
func (r *AuthRuntime) WithLock(mutator func(cfg *Config) error) error {
	r.mu.Lock()
	if err := mutator(&r.Config); err != nil {
		r.mu.Unlock()
		return err
	}
	snapshot := r.Config
	r.mu.Unlock()
	return UpdateConfig(snapshot)
}

// Read 在读锁内读取配置
func (r *AuthRuntime) Read(reader func(cfg *Config)) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	reader(&r.Config)
}

// IsInitialized 是否已设置过密码
func (r *AuthRuntime) IsInitialized() bool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.Config.Initialized
}

// Setup 首次设置 admin 密码，仅在未初始化时可用
func (r *AuthRuntime) Setup(password string) error {
	if err := checkNewPassword(password); err != nil {
		return err
	}
	// WithLock 内仍会二次检查；此处提前返回，避免初始化后继续执行昂贵的密码哈希。
	if r.IsInitialized() {
		return ErrAlreadyInit
	}
	hash, err := pwd.HashPassword(password)
	if errors.Is(err, pwd.ErrArgon2Busy) {
		return ErrPasswordBusy
	}
	if err != nil {
		return err
	}
	return r.WithLock(func(cfg *Config) error {
		if cfg.Initialized {
			return ErrAlreadyInit
		}
		// 兜底：若 InitAuth 尚未生成 secret（启动竞态），此处补上，避免签发用空密钥
		if cfg.JwtSecret == "" {
			cfg.JwtSecret = randomSecret()
		}
		if cfg.TokenTTLHours <= 0 {
			cfg.TokenTTLHours = defaultTokenTTLHours
		}
		cfg.PasswordHash = hash
		cfg.Initialized = true
		return nil
	})
}

// Login 校验密码，成功返回签发的 token
func (r *AuthRuntime) Login(password string) (string, error) {
	r.mu.RLock()
	cfg := r.Config
	r.mu.RUnlock()

	if !cfg.Initialized {
		return "", ErrNotInitialized
	}
	if !globalLoginFailureLimiter.begin(time.Now()) {
		return "", ErrLoginLimited
	}
	loginResult := loginAttemptIgnored
	defer func() {
		globalLoginFailureLimiter.finish(loginResult, time.Now())
	}()

	matched, err := pwd.VerifyPassword(cfg.PasswordHash, password)
	if errors.Is(err, pwd.ErrArgon2Busy) {
		return "", ErrPasswordBusy
	}
	if err != nil {
		return "", err
	}
	if !matched {
		loginResult = loginAttemptFailed
		return "", ErrWrongPassword
	}
	loginResult = loginAttemptSucceeded
	return jwt.GenerateToken(cfg.JwtSecret, time.Duration(cfg.TokenTTLHours)*time.Hour)
}

// ValidateToken 校验 token 是否有效
func (r *AuthRuntime) ValidateToken(token string) bool {
	r.mu.RLock()
	secret := r.Config.JwtSecret
	r.mu.RUnlock()
	if secret == "" {
		return false
	}
	_, err := jwt.ParseToken(token, secret)
	return err == nil
}

// ChangePassword 校验旧密码后更新为新密码，同时换掉 JwtSecret，返回新签发的 token。
//
// 换 secret 是必须的：改密码通常是因为怀疑密码泄露了，而已经发出去的 token
// 默认还能再用 7 天，不换 secret 的话拿着旧 token 的人照样进得来，改了等于没改。
// 换掉之后所有旧 token 立刻失效，返回的新 token 给当前这个浏览器续上，
// 其他设备上的登录状态会被踢掉，需要用新密码重新登录。
func (r *AuthRuntime) ChangePassword(oldPassword, newPassword string) (string, error) {
	if err := checkNewPassword(newPassword); err != nil {
		return "", err
	}
	r.mu.RLock()
	cfg := r.Config
	r.mu.RUnlock()

	if !cfg.Initialized {
		return "", ErrNotInitialized
	}
	matched, err := pwd.VerifyPassword(cfg.PasswordHash, oldPassword)
	if errors.Is(err, pwd.ErrArgon2Busy) {
		return "", ErrPasswordBusy
	}
	if err != nil {
		return "", err
	}
	if !matched {
		return "", ErrWrongPassword
	}
	hash, err := pwd.HashPassword(newPassword)
	if errors.Is(err, pwd.ErrArgon2Busy) {
		return "", ErrPasswordBusy
	}
	if err != nil {
		return "", err
	}

	var secret string
	var ttlHours int
	if err := r.WithLock(func(cfg *Config) error {
		cfg.PasswordHash = hash
		cfg.JwtSecret = randomSecret()
		if cfg.TokenTTLHours <= 0 {
			cfg.TokenTTLHours = defaultTokenTTLHours
		}
		secret, ttlHours = cfg.JwtSecret, cfg.TokenTTLHours
		return nil
	}); err != nil {
		return "", err
	}
	return jwt.GenerateToken(secret, time.Duration(ttlHours)*time.Hour)
}
