package dns

import (
	"fmt"
	"io"
	"linkstar/modules/ddns/model"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// NameCheap 动态 DNS 更新
// 文档: https://www.namecheap.com/support/knowledgebase/article.aspx/29/11/
const namecheapEndpoint = "https://dynamicdns.park-your-domain.com/update"

// NameCheap NameCheap
type NameCheap struct {
	Password   string // Dynamic DNS Password
	httpClient *http.Client
}

// NewNameCheap 创建 NameCheap 客户端
func NewNameCheap(password string) *NameCheap {
	return &NameCheap{
		Password: password,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
			Transport: &http.Transport{
				Proxy: nil,
			},
		},
	}
}

// SetRecord 设置一条记录(NameCheap 仅支持 IPv4/A 记录)
func (nc *NameCheap) SetRecord(domain string, subDomain string, recordType model.DNSRecordType, ipAddr string, ttl int, proxied bool) error {
	if recordType != model.DNSRecordTypeA {
		return fmt.Errorf("NameCheap 仅支持 IPv4(A 记录)")
	}

	host := subDomain
	if host == "" {
		host = "@"
	}

	params := url.Values{}
	params.Set("host", host)
	params.Set("domain", domain)
	params.Set("password", nc.Password)
	params.Set("ip", ipAddr)

	req, err := http.NewRequest(http.MethodGet, namecheapEndpoint+"?"+params.Encode(), http.NoBody)
	if err != nil {
		return err
	}

	resp, err := nc.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}
	// 成功返回的 XML 含 <ErrCount>0</ErrCount>
	if !strings.Contains(string(data), "<ErrCount>0</ErrCount>") {
		return fmt.Errorf("设置记录失败: %s", string(data))
	}
	return nil
}
