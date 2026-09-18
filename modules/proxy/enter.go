package proxy

import (
	"sync"

	"linkstar/modules/proxy/model"
)

// ProxyRuntime 反向代理的运行时。
//
// 一个自给自足的 nginx：自己开监听、自己按域名转发。不依赖 STUN，
// 也不知道 STUN 的存在——端口怎么暴露出去是用户在 STUN 页上的事。
//
// Router 是「全部站点」的总表，只给 404 页面和日志用；
// 真正转发用的是 pool 里每个端口各自那张表。
type ProxyRuntime struct {
	mu     sync.RWMutex
	Config model.ProxyConfig
	Router *Router

	pool *serverPool
}

var Runtime = &ProxyRuntime{
	Router: NewRouter(nil),
	pool:   newServerPool(),
}
