package dns

import (
	"fmt"
	"net/url"
	"strings"
)

// Cloudflare 支持维护 SRV 记录
var _ SRVRecordProvider = (*Cloudflare)(nil)

// srvRecordComment 标记这条 SRV 由 LinkStar 维护。删服务时只删带这个标记的。
const srvRecordComment = "linkstar:srv"

// cfSRVData Cloudflare 把 SRV 拆成字段，不收「优先级 权重 端口 目标」那种串
type cfSRVData struct {
	Priority uint16 `json:"priority"`
	Weight   uint16 `json:"weight"`
	Port     uint16 `json:"port"`
	Target   string `json:"target"`
}

type cfSRVRecord struct {
	ID      string    `json:"id,omitempty"`
	Type    string    `json:"type"`
	Name    string    `json:"name"`
	TTL     int       `json:"ttl"`
	Data    cfSRVData `json:"data"`
	Comment string    `json:"comment"`
}

type cfSRVRecordsResp struct {
	CloudflareStatus
	Result []cfSRVRecord
}

// SyncSRVRecord 见 SRVRecordProvider。
//
// 优先级 0、权重 5、TTL 自动：只有一个目标，前两个随便填，MC 不看。
func (cf *Cloudflare) SyncSRVRecord(zoneDomain, name, target string, port uint16) (bool, error) {
	name = trimHost(name)
	target = trimHost(target)
	if name == "" || target == "" || port == 0 {
		return false, fmt.Errorf("SRV 记录缺参数：name=%q target=%q port=%d", name, target, port)
	}
	zoneID, err := cf.zoneID(zoneDomain)
	if err != nil {
		return false, err
	}

	want := cfSRVRecord{
		Type:    "SRV",
		Name:    name,
		TTL:     1,
		Data:    cfSRVData{Priority: 0, Weight: 5, Port: port, Target: target},
		Comment: srvRecordComment,
	}

	existing, err := cf.findSRVRecord(zoneID, name)
	if err != nil {
		return false, err
	}
	if existing == nil {
		var status CloudflareStatus
		if err := cf.request("POST", fmt.Sprintf("%s/%s/dns_records", zonesAPI, zoneID), want, &status); err != nil {
			return false, srvWriteError("新建", err)
		}
		if !status.Success {
			return false, fmt.Errorf("新建 SRV 记录返回失败: %v", status.Messages)
		}
		return true, nil
	}

	// 端口、目标、标记都对上就不打 API；只差标记也改一次，以后删服务时才认得出来
	if existing.Data.Port == port && trimHost(existing.Data.Target) == target && existing.Comment == srvRecordComment {
		return false, nil
	}
	want.ID = existing.ID
	want.Data.Priority, want.Data.Weight = existing.Data.Priority, existing.Data.Weight // 用户手调过的不动
	var status CloudflareStatus
	if err := cf.request("PUT", fmt.Sprintf("%s/%s/dns_records/%s", zonesAPI, zoneID, existing.ID), want, &status); err != nil {
		return false, srvWriteError("更新", err)
	}
	if !status.Success {
		return false, fmt.Errorf("更新 SRV 记录返回失败: %v", status.Messages)
	}
	return true, nil
}

// RemoveSRVRecord 见 SRVRecordProvider。没有标记的是用户自己的，不动。
func (cf *Cloudflare) RemoveSRVRecord(zoneDomain, name string) (bool, error) {
	zoneID, err := cf.zoneID(zoneDomain)
	if err != nil {
		return false, err
	}
	rec, err := cf.findSRVRecord(zoneID, trimHost(name))
	if err != nil || rec == nil || rec.Comment != srvRecordComment {
		return false, err
	}
	var status CloudflareStatus
	if err := cf.request("DELETE", fmt.Sprintf("%s/%s/dns_records/%s", zonesAPI, zoneID, rec.ID), nil, &status); err != nil {
		return false, fmt.Errorf("删除 SRV 记录失败: %w", err)
	}
	return status.Success, nil
}

func (cf *Cloudflare) findSRVRecord(zoneID, name string) (*cfSRVRecord, error) {
	params := url.Values{}
	params.Set("type", "SRV")
	params.Set("name", name)
	var resp cfSRVRecordsResp
	err := cf.request("GET", fmt.Sprintf("%s/%s/dns_records?%s", zonesAPI, zoneID, params.Encode()), nil, &resp)
	if isForbidden(err) {
		return nil, errSRVPermission
	}
	if err != nil {
		return nil, fmt.Errorf("查询 SRV 记录失败: %w", err)
	}
	if !resp.Success {
		return nil, fmt.Errorf("查询 SRV 记录返回失败: %v", resp.Messages)
	}
	if len(resp.Result) == 0 {
		return nil, nil
	}
	return &resp.Result[0], nil
}

// errSRVPermission SRV 就是普通 DNS 记录，只要 Zone → DNS → 编辑
var errSRVPermission = fmt.Errorf(
	"Cloudflare 令牌没有 DNS 权限：建 SRV 记录要「区域 → DNS → 编辑」，用「编辑区域 DNS」模板建的令牌就够",
)

func srvWriteError(action string, err error) error {
	if isForbidden(err) {
		return errSRVPermission
	}
	return fmt.Errorf("%s SRV 记录失败: %w", action, err)
}

// trimHost 去掉空白和末尾的点，统一小写比较
func trimHost(s string) string {
	return strings.ToLower(strings.TrimSuffix(strings.TrimSpace(s), "."))
}
