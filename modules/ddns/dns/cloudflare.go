package dns

const zonesAPI = "https://api.cloudflare.com/client/v4/zones"

// Cloudflare
type Cloudflare struct {
}

// 获取域名记录列表
func (cf *Cloudflare) getZone()

// request 统一请求接口
func (cf *Cloudflare) request(method string, url string, data interface{}, result interface{}) (err error) {

}
