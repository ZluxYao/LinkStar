package webhook

import (
	"fmt"
	"io"
	httpUtils "linkstar/utils/http_utils"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// Send 按配置来发送一次 webhook
func Send(cfg WebhookConfig, fields map[string]string) (respBody string, err error) {
	replacer := buildReplacer(fields) //构造成 #{key} -> value 的替换器

	// 整理参数
	method := strings.ToUpper(strings.TrimSpace(cfg.Method))
	if method == "" {
		method = http.MethodGet
	}
	switch method {
	case http.MethodGet, http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete, http.MethodHead, http.MethodOptions:
	default:
		return "", fmt.Errorf("不支持的请求方法: %s", method)
	}

	rawURL := strings.TrimSpace(replacer.Replace(cfg.URL))
	if rawURL == "" {
		return "", fmt.Errorf("webhook URL 为空")
	}
	body := replacer.Replace(cfg.Body)

	// 构建客户端
	client, err := buildClient(cfg.Proxy)

	for i := 0; i < 3; i++ {
		respBody, err = sendOnce(client, method, rawURL, body, cfg, replacer)
		if err == nil {
			return respBody, nil
		}

		delay := time.Duration(1<<i) * time.Second
		time.Sleep(delay)
	}
	return respBody, err
}

// sendOnce 发送一次 webhook 请求并判定是否成功
func sendOnce(client *http.Client, method, rawURL, body string, cfg WebhookConfig, replacer *strings.Replacer) (string, error) {
	var reqBody io.Reader
	if method != http.MethodGet && body != "" {
		reqBody = strings.NewReader(body)
	}

	req, err := http.NewRequest(method, rawURL, reqBody)
	if err != nil {
		return "", err
	}
	applyHeaders(req, cfg.Headers, replacer) // 写入header

	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}

	data, httpErr := httpUtils.GetHTTPResponseBody(resp)
	respBody := string(data)
	if httpErr != nil {
		// 非 2xx,GetHTTPResponseBody 已带上状态码与返回内容
		return respBody, httpErr
	}

	// 字符串检测
	if !cfg.DisableSuccessCheck {
		needle := strings.TrimSpace(cfg.SuccessContains)
		if needle != "" && !strings.Contains(respBody, needle) {
			return respBody, fmt.Errorf("返回体未包含成功字符串: %s", needle)
		}
	}
	return respBody, nil
}

// buildReplacer 把 fields 构造成 #{key} -> value 的替换器
func buildReplacer(fields map[string]string) *strings.Replacer {
	pairs := make([]string, 0, len(fields)*2)

	for k, v := range fields {
		pairs = append(pairs, "#{"+k+"}", v)
	}

	return strings.NewReplacer(pairs...)
}

// buildClient 构建 webhook 请求客户端
func buildClient(proxy string) (*http.Client, error) {
	transport := &http.Transport{Proxy: nil}
	// 解析http代理
	if p := strings.TrimSpace(proxy); p != "" {
		u, err := url.Parse(p)
		if err != nil {
			return nil, fmt.Errorf("代理地址无效: %w", err)
		}
		transport.Proxy = http.ProxyURL(u)
	}

	return &http.Client{
		Timeout:   30 * time.Second,
		Transport: transport,
	}, nil
}

// applyHeaders 逐行解析 "Key: Value"，value 同样做占位符替换
func applyHeaders(req *http.Request, headers string, replacer *strings.Replacer) {
	for _, line := range strings.Split(headers, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		idx := strings.Index(line, ":")
		if idx <= 0 {
			continue
		}
		key := strings.TrimSpace(line[:idx])
		val := strings.TrimSpace(replacer.Replace(line[idx+1:]))
		if key != "" {
			req.Header.Set(key, val)
		}
	}
}
