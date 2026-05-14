package home

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

const fetchIconDir = "data/icon"
const fetchHTTPTimeout = 8 * time.Second
const fetchMaxIconSize = 5 << 20 // 5 MiB
const fetchMaxHTMLSize = 512 << 10

var fetchUA = "Mozilla/5.0 (compatible; LinkStarIconFetcher/1.0)"

// 优先级越高越好
var iconRels = []struct {
	pattern  *regexp.Regexp
	priority int
}{
	{regexp.MustCompile(`(?is)<link\b[^>]*\brel\s*=\s*["']?apple-touch-icon(?:-precomposed)?["']?[^>]*>`), 30},
	{regexp.MustCompile(`(?is)<link\b[^>]*\brel\s*=\s*["']?icon["']?[^>]*>`), 20},
	{regexp.MustCompile(`(?is)<link\b[^>]*\brel\s*=\s*["']?shortcut\s+icon["']?[^>]*>`), 10},
}

var hrefRe = regexp.MustCompile(`(?is)\bhref\s*=\s*(?:"([^"]+)"|'([^']+)'|([^\s>]+))`)

var iconExtByMime = map[string]string{
	"image/x-icon":             ".ico",
	"image/vnd.microsoft.icon": ".ico",
	"image/png":                ".png",
	"image/jpeg":               ".jpg",
	"image/svg+xml":            ".svg",
	"image/webp":               ".webp",
	"image/gif":                ".gif",
}

var iconExtAllowed = map[string]bool{
	".ico": true, ".png": true, ".jpg": true, ".jpeg": true,
	".svg": true, ".webp": true, ".gif": true,
}

// FetchIconFromURL 给定站点 URL,尽力抓取一个 favicon 并存到 data/icon/,返回相对路径。
func FetchIconFromURL(rawURL string) (string, error) {
	rawURL = strings.TrimSpace(rawURL)
	if rawURL == "" {
		return "", errors.New("url 不能为空")
	}
	if !strings.HasPrefix(rawURL, "http://") && !strings.HasPrefix(rawURL, "https://") {
		rawURL = "https://" + rawURL
	}
	u, err := url.Parse(rawURL)
	if err != nil || u.Host == "" {
		return "", fmt.Errorf("无效 URL")
	}

	client := &http.Client{Timeout: fetchHTTPTimeout}

	candidates := collectIconCandidates(client, u)
	// fallback
	candidates = append(candidates, u.Scheme+"://"+u.Host+"/favicon.ico")

	var lastErr error
	for _, c := range candidates {
		if c == "" {
			continue
		}
		path, err := downloadIcon(client, c)
		if err == nil {
			return path, nil
		}
		lastErr = err
	}
	if lastErr == nil {
		lastErr = errors.New("未能抓取到图标")
	}
	return "", lastErr
}

// 在页面 HTML 里找候选,按优先级排序返回绝对 URL 列表
func collectIconCandidates(client *http.Client, u *url.URL) []string {
	req, err := http.NewRequest(http.MethodGet, u.String(), nil)
	if err != nil {
		return nil
	}
	req.Header.Set("User-Agent", fetchUA)
	resp, err := client.Do(req)
	if err != nil {
		return nil
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		return nil
	}
	data, _ := io.ReadAll(io.LimitReader(resp.Body, fetchMaxHTMLSize))

	type scored struct {
		href     string
		priority int
	}
	var out []scored
	seen := map[string]bool{}
	for _, rel := range iconRels {
		for _, m := range rel.pattern.FindAll(data, -1) {
			href := extractHref(m)
			if href == "" {
				continue
			}
			resolved, err := u.Parse(href)
			if err != nil {
				continue
			}
			full := resolved.String()
			if seen[full] {
				continue
			}
			seen[full] = true
			out = append(out, scored{full, rel.priority})
		}
	}
	// 简单冒泡排序按 priority 降序
	for i := 0; i < len(out); i++ {
		for j := i + 1; j < len(out); j++ {
			if out[j].priority > out[i].priority {
				out[i], out[j] = out[j], out[i]
			}
		}
	}
	result := make([]string, 0, len(out))
	for _, s := range out {
		result = append(result, s.href)
	}
	return result
}

func extractHref(linkTag []byte) string {
	m := hrefRe.FindSubmatch(linkTag)
	if m == nil {
		return ""
	}
	for i := 1; i < len(m); i++ {
		if len(m[i]) > 0 {
			return strings.TrimSpace(string(m[i]))
		}
	}
	return ""
}

func downloadIcon(client *http.Client, iconURL string) (string, error) {
	req, err := http.NewRequest(http.MethodGet, iconURL, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("User-Agent", fetchUA)
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		return "", fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, fetchMaxIconSize+1))
	if err != nil {
		return "", err
	}
	if len(data) == 0 {
		return "", errors.New("空图标")
	}
	if len(data) > fetchMaxIconSize {
		return "", errors.New("图标过大")
	}

	// 选扩展名: Content-Type 优先,其次 URL 后缀,最后默认 .png
	mime := strings.ToLower(strings.TrimSpace(strings.SplitN(resp.Header.Get("Content-Type"), ";", 2)[0]))
	ext := iconExtByMime[mime]
	if ext == "" {
		if pu, perr := url.Parse(iconURL); perr == nil {
			candidate := strings.ToLower(filepath.Ext(pu.Path))
			if iconExtAllowed[candidate] {
				ext = candidate
			}
		}
	}
	if ext == "" {
		ext = ".png"
	}

	if err := os.MkdirAll(fetchIconDir, 0755); err != nil {
		return "", err
	}
	name := fmt.Sprintf("%d%s", time.Now().UnixNano(), ext)
	relPath := filepath.ToSlash(filepath.Join(fetchIconDir, name))
	if err := os.WriteFile(relPath, data, 0644); err != nil {
		return "", err
	}
	return relPath, nil
}
