package webhook

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// ============ webhook 工具函数 ============
//
// 本包目前只暴露一个核心函数：Do
// 目的：把"发一次 HTTP 请求"压缩成 4 个变量，
// 让上层代码（cf 业务、未来其它 provider、通知场景）
// 不用再每次写 NewRequest + Header.Set 循环 + Do + ReadAll 这套 boilerplate
//
// 之后这个包可能再加：
//   - Send(cfg, vars)  用户配置驱动的高级版（带模板渲染）
//   - Scheduler        订阅 stun 事件 + 定时器
//   - Config CRUD      管理用户配的 webhooks
// 但这些都不是"工具函数"该承担的，分开放各自的文件

// 全局共享的 HTTP 客户端：超时、连接池、代理（未来加）都在这里统一配
var sharedClient = &http.Client{
	Timeout: 10 * time.Second,
}

// Do 极简 HTTP 调用器
//
// 参数：
//   method  - GET/POST/PUT/PATCH/DELETE 任意 HTTP 方法
//   url     - 完整 URL（调用方自己 fmt.Sprintf 拼好）
//   headers - 请求头，可传 nil
//   body    - 请求体，类型自动识别：
//               nil           → 不发 body
//               []byte        → 原样发
//               string        → 原样发
//               其它（struct/map）→ json.Marshal 后发，自动加 Content-Type: application/json
//
// 返回：
//   respBody   - 响应体字节流（调用方自己 json.Unmarshal 到 typed struct）
//   statusCode - HTTP 状态码
//   err        - 网络错误或 body 序列化错误（注意：4xx/5xx 不算 err，自己看 statusCode 判断）
func Do(method, url string, headers map[string]string, body any) (respBody []byte, statusCode int, err error) {
	// 1) 处理 body：根据类型转成 io.Reader
	var bodyReader io.Reader
	autoJSON := false

	if body != nil {
		switch b := body.(type) {
		case []byte:
			bodyReader = bytes.NewReader(b)
		case string:
			bodyReader = strings.NewReader(b)
		default:
			data, err := json.Marshal(b)
			if err != nil {
				return nil, 0, fmt.Errorf("body json.Marshal 失败: %w", err)
			}
			bodyReader = bytes.NewReader(data)
			autoJSON = true
		}
	}

	// 2) 构造请求
	req, err := http.NewRequest(method, url, bodyReader)
	if err != nil {
		return nil, 0, fmt.Errorf("构造请求失败: %w", err)
	}

	// 3) 写 headers
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	// 自动补 Content-Type（用户没指定且 body 是 struct/map 时）
	if autoJSON && req.Header.Get("Content-Type") == "" {
		req.Header.Set("Content-Type", "application/json")
	}

	// 4) 发请求
	resp, err := sharedClient.Do(req)
	if err != nil {
		return nil, 0, fmt.Errorf("发送请求失败: %w", err)
	}
	defer resp.Body.Close()

	// 5) 读响应
	respBody, err = io.ReadAll(resp.Body)
	if err != nil {
		return nil, resp.StatusCode, fmt.Errorf("读响应失败: %w", err)
	}

	return respBody, resp.StatusCode, nil
}
