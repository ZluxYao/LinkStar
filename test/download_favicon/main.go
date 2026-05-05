package main

import (
	"bufio"
	"bytes"
	"errors"
	"flag"
	"fmt"
	"html"
	"io"
	"mime"
	"net/http"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"
)

const (
	outputDir = "favicons"
	userAgent = "Mozilla/5.0 (compatible; favicon-downloader/1.0)"
)

var (
	linkTagRE = regexp.MustCompile(`(?is)<link\b[^>]*>`)
	attrRE    = regexp.MustCompile(`(?is)([a-zA-Z_:][-a-zA-Z0-9_:.]*)\s*=\s*(?:"([^"]*)"|'([^']*)'|([^\s"'=<>]+))`)
)

type iconCandidate struct {
	url   *url.URL
	score int
	typ   string
}

func main() {
	file := flag.String("file", "", "file containing one URL per line")
	flag.Parse()

	sites, err := readSites(*file, flag.Args())
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	if len(sites) == 0 {
		fmt.Fprintln(os.Stderr, "usage: go run . [-file urls.txt] github.com https://example.com")
		os.Exit(1)
	}
	if err := os.MkdirAll(outputDir, 0o755); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	client := &http.Client{Timeout: 12 * time.Second}
	failed := 0
	for _, site := range sites {
		if err := downloadFavicon(client, site); err != nil {
			failed++
			fmt.Printf("[fail] %s: %v\n", site, err)
		}
	}
	if failed > 0 {
		os.Exit(1)
	}
}

func readSites(file string, args []string) ([]string, error) {
	var sites []string
	sites = append(sites, args...)

	if file != "" {
		f, err := os.Open(file)
		if err != nil {
			return nil, err
		}
		defer f.Close()

		scanner := bufio.NewScanner(f)
		for scanner.Scan() {
			line := strings.TrimSpace(strings.TrimPrefix(scanner.Text(), "\ufeff"))
			if line != "" && !strings.HasPrefix(line, "#") {
				sites = append(sites, line)
			}
		}
		if err := scanner.Err(); err != nil {
			return nil, err
		}
	}

	seen := make(map[string]bool)
	out := sites[:0]
	for _, site := range sites {
		site = strings.TrimSpace(site)
		if site != "" && !seen[site] {
			seen[site] = true
			out = append(out, site)
		}
	}
	return out, nil
}

func downloadFavicon(client *http.Client, rawSite string) error {
	siteURL, inferredHTTPS, err := normalizeSiteURL(rawSite)
	if err != nil {
		return err
	}

	baseURLs := []*url.URL{siteURL}
	if inferredHTTPS {
		httpURL := *siteURL
		httpURL.Scheme = "http"
		baseURLs = append(baseURLs, &httpURL)
	}

	var attempts []string
	for _, baseURL := range baseURLs {
		candidates, err := findFavicons(client, baseURL)
		if err != nil {
			attempts = append(attempts, err.Error())
		}

		for _, candidate := range candidates {
			body, contentType, finalURL, err := get(client, candidate.url, "image/avif,image/webp,image/apng,image/svg+xml,image/*,*/*;q=0.8")
			if err != nil {
				attempts = append(attempts, fmt.Sprintf("%s: %v", candidate.url, err))
				continue
			}
			if !looksLikeImage(contentType, finalURL, body) {
				attempts = append(attempts, fmt.Sprintf("%s: not an image", candidate.url))
				continue
			}

			ext := iconExt(contentType, finalURL, body, candidate.typ)
			if ext == "" {
				ext = ".ico"
			}
			outPath := filepath.Join(outputDir, safeName(baseURL.Hostname())+ext)
			if err := os.WriteFile(outPath, body, 0o644); err != nil {
				return err
			}
			fmt.Printf("[ok] %s -> %s (from %s)\n", baseURL, outPath, finalURL)
			return nil
		}
	}

	if len(attempts) > 3 {
		attempts = append(attempts[:3], fmt.Sprintf("... %d more", len(attempts)-3))
	}
	return errors.New(strings.Join(attempts, "; "))
}

func normalizeSiteURL(raw string) (*url.URL, bool, error) {
	raw = strings.TrimSpace(raw)
	inferredHTTPS := false
	if !strings.Contains(raw, "://") {
		raw = "https://" + raw
		inferredHTTPS = true
	}

	u, err := url.Parse(raw)
	if err != nil {
		return nil, inferredHTTPS, err
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return nil, inferredHTTPS, fmt.Errorf("unsupported scheme: %s", u.Scheme)
	}
	if u.Host == "" {
		return nil, inferredHTTPS, errors.New("missing host")
	}
	if u.Path == "" {
		u.Path = "/"
	}
	return u, inferredHTTPS, nil
}

func findFavicons(client *http.Client, siteURL *url.URL) ([]iconCandidate, error) {
	body, _, finalURL, err := get(client, siteURL, "text/html,application/xhtml+xml;q=0.9,*/*;q=0.8")
	if err != nil {
		return []iconCandidate{defaultFavicon(siteURL)}, err
	}

	candidates := parseIconLinks(string(body), finalURL)
	candidates = append(candidates, defaultFavicon(finalURL))
	sort.SliceStable(candidates, func(i, j int) bool {
		return candidates[i].score > candidates[j].score
	})
	return dedupeCandidates(candidates), nil
}

func parseIconLinks(pageHTML string, baseURL *url.URL) []iconCandidate {
	var candidates []iconCandidate
	for _, tag := range linkTagRE.FindAllString(pageHTML, -1) {
		attrs := parseAttrs(tag)
		rel := strings.ToLower(attrs["rel"])
		if attrs["href"] == "" || !isIconRel(rel) {
			continue
		}

		iconURL, err := url.Parse(attrs["href"])
		if err != nil {
			continue
		}
		iconURL = baseURL.ResolveReference(iconURL)
		if iconURL.Scheme != "http" && iconURL.Scheme != "https" {
			continue
		}

		candidates = append(candidates, iconCandidate{
			url:   iconURL,
			score: iconScore(attrs["href"], rel, attrs["sizes"], attrs["type"]),
			typ:   attrs["type"],
		})
	}
	return candidates
}

func parseAttrs(tag string) map[string]string {
	attrs := make(map[string]string)
	for _, m := range attrRE.FindAllStringSubmatch(tag, -1) {
		value := ""
		for i := 2; i < len(m); i++ {
			if m[i] != "" {
				value = m[i]
				break
			}
		}
		attrs[strings.ToLower(m[1])] = strings.TrimSpace(html.UnescapeString(value))
	}
	return attrs
}

func isIconRel(rel string) bool {
	tokens := relTokens(rel)
	return tokens["icon"] &&
		!tokens["apple-touch-icon"] &&
		!tokens["apple-touch-icon-precomposed"] &&
		!tokens["mask-icon"]
}

func relTokens(rel string) map[string]bool {
	tokens := make(map[string]bool)
	for _, token := range strings.Fields(strings.ToLower(rel)) {
		tokens[token] = true
	}
	return tokens
}

func iconScore(href, rel, sizes, contentType string) int {
	score := 1000
	hrefLower := strings.ToLower(href)
	contentType = strings.ToLower(contentType)
	ext := strings.ToLower(path.Ext(hrefLower))

	if strings.Contains(hrefLower, "favicon.ico") || ext == ".ico" || strings.Contains(contentType, "icon") {
		score += 300
	}
	if strings.Contains(contentType, "svg") || ext == ".svg" || strings.Contains(strings.ToLower(sizes), "any") {
		score += 250
	}
	if strings.Contains(rel, "shortcut") {
		score += 100
	}

	bestSize := 0
	for _, size := range strings.Fields(strings.ToLower(sizes)) {
		parts := strings.Split(size, "x")
		if len(parts) != 2 {
			continue
		}
		w := atoi(parts[0])
		h := atoi(parts[1])
		if w > 0 && h > 0 {
			bestSize = max(bestSize, min(w, h))
		}
	}

	switch {
	case bestSize == 16 || bestSize == 32:
		score += 200
	case bestSize > 0 && bestSize <= 64:
		score += 120
	case bestSize > 64:
		score -= min(bestSize, 512)
	}

	return score
}

func defaultFavicon(siteURL *url.URL) iconCandidate {
	u := *siteURL
	u.Path = "/favicon.ico"
	u.RawQuery = ""
	u.Fragment = ""
	return iconCandidate{url: &u, score: 900, typ: "image/x-icon"}
}

func dedupeCandidates(candidates []iconCandidate) []iconCandidate {
	seen := make(map[string]bool)
	out := candidates[:0]
	for _, candidate := range candidates {
		key := candidate.url.String()
		if !seen[key] {
			seen[key] = true
			out = append(out, candidate)
		}
	}
	return out
}

func get(client *http.Client, u *url.URL, accept string) ([]byte, string, *url.URL, error) {
	req, err := http.NewRequest(http.MethodGet, u.String(), nil)
	if err != nil {
		return nil, "", u, err
	}
	req.Header.Set("User-Agent", userAgent)
	req.Header.Set("Accept", accept)

	resp, err := client.Do(req)
	if err != nil {
		return nil, "", u, err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 400 {
		return nil, "", resp.Request.URL, fmt.Errorf("HTTP %d", resp.StatusCode)
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, 5<<20))
	if err != nil {
		return nil, "", resp.Request.URL, err
	}
	return body, resp.Header.Get("Content-Type"), resp.Request.URL, nil
}

func looksLikeImage(contentType string, finalURL *url.URL, body []byte) bool {
	if extFromContentType(contentType) != "" || extFromBytes(body) != "" {
		return true
	}

	mediaType, _, _ := mime.ParseMediaType(contentType)
	if mediaType == "" || mediaType == "application/octet-stream" || mediaType == "text/plain" {
		return extFromPath(finalURL.Path) != ""
	}
	return false
}

func iconExt(contentType string, finalURL *url.URL, body []byte, candidateType string) string {
	if ext := extFromContentType(contentType); ext != "" {
		return ext
	}
	if ext := extFromBytes(body); ext != "" {
		return ext
	}
	if ext := extFromContentType(candidateType); ext != "" {
		return ext
	}
	return extFromPath(finalURL.Path)
}

func extFromContentType(contentType string) string {
	mediaType, _, _ := mime.ParseMediaType(contentType)
	switch strings.ToLower(mediaType) {
	case "image/x-icon", "image/vnd.microsoft.icon", "image/ico", "image/icon":
		return ".ico"
	case "image/png":
		return ".png"
	case "image/jpeg", "image/jpg":
		return ".jpg"
	case "image/gif":
		return ".gif"
	case "image/svg+xml":
		return ".svg"
	case "image/webp":
		return ".webp"
	case "image/avif":
		return ".avif"
	default:
		return ""
	}
}

func extFromPath(urlPath string) string {
	switch strings.ToLower(path.Ext(urlPath)) {
	case ".ico", ".png", ".jpg", ".jpeg", ".gif", ".svg", ".webp", ".avif":
		if strings.EqualFold(path.Ext(urlPath), ".jpeg") {
			return ".jpg"
		}
		return strings.ToLower(path.Ext(urlPath))
	default:
		return ""
	}
}

func extFromBytes(body []byte) string {
	if len(body) >= 4 && body[0] == 0x00 && body[1] == 0x00 && body[2] == 0x01 && body[3] == 0x00 {
		return ".ico"
	}
	if len(body) >= 8 && bytes.Equal(body[:8], []byte{0x89, 'P', 'N', 'G', '\r', '\n', 0x1a, '\n'}) {
		return ".png"
	}
	if len(body) >= 3 && body[0] == 0xff && body[1] == 0xd8 && body[2] == 0xff {
		return ".jpg"
	}
	if len(body) >= 6 && (bytes.Equal(body[:6], []byte("GIF87a")) || bytes.Equal(body[:6], []byte("GIF89a"))) {
		return ".gif"
	}
	if len(body) >= 12 && bytes.Equal(body[:4], []byte("RIFF")) && bytes.Equal(body[8:12], []byte("WEBP")) {
		return ".webp"
	}
	trimmed := bytes.TrimSpace(body)
	if len(trimmed) > 4 && bytes.HasPrefix(bytes.ToLower(trimmed[:min(len(trimmed), 256)]), []byte("<svg")) {
		return ".svg"
	}
	return ""
}

func safeName(host string) string {
	host = strings.ToLower(strings.TrimSpace(host))
	host = strings.TrimPrefix(host, "www.")
	var b strings.Builder
	for _, r := range host {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '.' || r == '-' {
			b.WriteRune(r)
		} else {
			b.WriteByte('_')
		}
	}
	if b.Len() == 0 {
		return "site"
	}
	return b.String()
}

func atoi(s string) int {
	n := 0
	for _, r := range s {
		if r < '0' || r > '9' {
			return n
		}
		n = n*10 + int(r-'0')
	}
	return n
}
