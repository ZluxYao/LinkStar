package cf

import (
	"encoding/json"
	"fmt"
	"net/url"
	"strings"

	"ddns/webhook"
)

// ============ CF DDNS 业务模块 ============
//
// 对外只暴露一个高层函数：UpdateRecord
// 它内部封装了"查 → 比较 → 改/建"三步业务流
//
// HTTP 调用统一用 webhook.Do 这个工具函数（省 boilerplate）
// JSON 解析用本包 types.go 里定义的 typed struct（编译期类型安全，不用 JSONPath）

const cfAPI = "https://api.cloudflare.com/client/v4"

// UpdateRecord 把 zone 下某个域名的 A 记录更新为给定 IP
//
// 行为：
//  0. 如果 zoneID 为空 → 自动从 recordName 反查（需要 token 有 Zone:Read 权限）
//  1. 用 name 查找已存在的 A 记录
//  2. 如果存在且 IP 相同 → ActionSkipped
//  3. 如果存在但 IP 不同 → PUT 改它 → ActionUpdated
//  4. 如果不存在        → POST 新建 → ActionCreated
//
// 参数：
//
//	token      - CF API Token（至少 Zone → DNS → Edit；要自动反查 zone 还需 Zone → Zone → Read）
//	zoneID     - 可选；留空会自动从 recordName 反查
//	recordName - 完整域名，如 "home.example.com" 或根域名 "example.com"
//	ip         - 要写入的 IPv4
func UpdateRecord(token, zoneID, recordName, ip string) (Result, error) {
	// 0) 没传 zoneID 就自动反查
	if zoneID == "" {
		var err error
		zoneID, err = FindZoneID(token, recordName)
		if err != nil {
			return Result{}, fmt.Errorf("自动反查 zoneID 失败: %w", err)
		}
	}

	// 1) 查现有记录
	records, err := listRecords(token, zoneID, recordName)
	if err != nil {
		return Result{}, fmt.Errorf("查记录失败: %w", err)
	}

	// 2) 不存在 → 新建
	if len(records) == 0 {
		newRec, err := createRecord(token, zoneID, recordName, ip)
		if err != nil {
			return Result{}, fmt.Errorf("新建记录失败: %w", err)
		}
		return Result{
			Action:   ActionCreated,
			RecordID: newRec.ID,
			NewIP:    newRec.Content,
		}, nil
	}

	rec := records[0]

	// 3) IP 没变 → 跳过
	if rec.Content == ip {
		return Result{
			Action:   ActionSkipped,
			RecordID: rec.ID,
			PrevIP:   rec.Content,
			NewIP:    rec.Content,
		}, nil
	}

	// 4) IP 变了 → PUT 更新
	updated, err := updateRecordByID(token, zoneID, rec.ID, recordName, ip)
	if err != nil {
		return Result{}, fmt.Errorf("更新记录失败: %w", err)
	}
	return Result{
		Action:   ActionUpdated,
		RecordID: updated.ID,
		PrevIP:   rec.Content,
		NewIP:    updated.Content,
	}, nil
}

// ============ 私有：三个 HTTP 调用 ============

// listRecords 查 zone 下指定 name 的 A 记录
func listRecords(token, zoneID, name string) ([]Record, error) {
	// 拼 URL，name 要 URL 编码
	q := url.Values{}
	q.Set("type", "A")
	q.Set("name", name)
	u := fmt.Sprintf("%s/zones/%s/dns_records?%s", cfAPI, zoneID, q.Encode())

	body, status, err := webhook.Do("GET", u, authHeaders(token), nil)
	if err != nil {
		return nil, err
	}
	if status >= 400 {
		return nil, fmt.Errorf("http %d: %s", status, string(body))
	}

	var resp listResp
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("解析响应失败: %w", err)
	}
	if !resp.Success {
		return nil, fmt.Errorf("CF API 失败: %v", resp.Errors)
	}
	return resp.Result, nil
}

// createRecord 新建一条 A 记录
func createRecord(token, zoneID, name, ip string) (Record, error) {
	u := fmt.Sprintf("%s/zones/%s/dns_records", cfAPI, zoneID)

	payload := Record{
		Type:    "A",
		Name:    name,
		Content: ip,
		TTL:     120, // 短 TTL 适合 DDNS
		Proxied: false,
	}

	body, status, err := webhook.Do("POST", u, authHeaders(token), payload)
	if err != nil {
		return Record{}, err
	}
	if status >= 400 {
		return Record{}, fmt.Errorf("http %d: %s", status, string(body))
	}

	var resp mutateResp
	if err := json.Unmarshal(body, &resp); err != nil {
		return Record{}, fmt.Errorf("解析响应失败: %w", err)
	}
	if !resp.Success {
		return Record{}, fmt.Errorf("CF API 失败: %v", resp.Errors)
	}
	return resp.Result, nil
}

// updateRecordByID PUT 改一条已存在记录的 IP
func updateRecordByID(token, zoneID, recordID, name, ip string) (Record, error) {
	u := fmt.Sprintf("%s/zones/%s/dns_records/%s", cfAPI, zoneID, recordID)

	payload := Record{
		Type:    "A",
		Name:    name,
		Content: ip,
		TTL:     120,
		Proxied: false,
	}

	body, status, err := webhook.Do("PUT", u, authHeaders(token), payload)
	if err != nil {
		return Record{}, err
	}
	if status >= 400 {
		return Record{}, fmt.Errorf("http %d: %s", status, string(body))
	}

	var resp mutateResp
	if err := json.Unmarshal(body, &resp); err != nil {
		return Record{}, fmt.Errorf("解析响应失败: %w", err)
	}
	if !resp.Success {
		return Record{}, fmt.Errorf("CF API 失败: %v", resp.Errors)
	}
	return resp.Result, nil
}

// authHeaders 统一生成 CF 鉴权头
func authHeaders(token string) map[string]string {
	return map[string]string{
		"Authorization": "Bearer " + token,
	}
}

// ============ Zone 反查 ============

// FindZoneID 给定一个完整域名，反查它属于哪个 CF zone
//
// 策略：从最长开始逐段缩短，第一个命中的就是
//   home.example.com → 试 home.example.com → 试 example.com (命中) → 返回
//
// 注意：需要 token 有 "Zone → Zone → Read" 权限；
// 如果用户的 token 只有特定 zone 的权限，应该手动传 zoneID 跳过反查
func FindZoneID(token, fullName string) (string, error) {
	parts := strings.Split(fullName, ".")
	if len(parts) < 2 {
		return "", fmt.Errorf("域名格式不对: %q", fullName)
	}

	// 至少要剩 2 段（顶级域 + 一级域，如 example.com）
	for i := 0; i <= len(parts)-2; i++ {
		candidate := strings.Join(parts[i:], ".")
		id, err := getZoneIDByName(token, candidate)
		if err != nil {
			return "", err
		}
		if id != "" {
			return id, nil
		}
	}
	return "", fmt.Errorf("找不到包含 %q 的 zone", fullName)
}

// getZoneIDByName 给一个候选 zone name 去 CF 查，命中返回 ID，未命中返回空串
func getZoneIDByName(token, name string) (string, error) {
	q := url.Values{}
	q.Set("name", name)
	u := fmt.Sprintf("%s/zones?%s", cfAPI, q.Encode())

	body, status, err := webhook.Do("GET", u, authHeaders(token), nil)
	if err != nil {
		return "", err
	}
	if status >= 400 {
		return "", fmt.Errorf("http %d: %s", status, string(body))
	}

	var resp zoneListResp
	if err := json.Unmarshal(body, &resp); err != nil {
		return "", fmt.Errorf("解析响应失败: %w", err)
	}
	if !resp.Success {
		return "", fmt.Errorf("CF API 失败: %v", resp.Errors)
	}
	if len(resp.Result) == 0 {
		return "", nil
	}
	return resp.Result[0].ID, nil
}
