package proxy_api

// ProxyApi 反向代理相关接口。
//
// 这一层只和 modules/proxy 打交道。反向代理就在本机监听一个端口按域名转发，
// 和 STUN 没有任何关系——那个端口怎么暴露到公网是用户在 STUN 页上的事。
type ProxyApi struct{}
