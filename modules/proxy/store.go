package proxy

import (
	"linkstar/modules/proxy/model"
)

// Snapshot 返回配置的深拷贝快照，可安全脱离锁使用
func (r *ProxyRuntime) Snapshot() model.ProxyConfig {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return cloneConfig(r.Config)
}

// Update 在写锁内执行 mutator，落盘、重建站点表、调整监听。
// mutator 返回错误时什么都不做。
func (r *ProxyRuntime) Update(fn func(cfg *model.ProxyConfig) error) error {
	r.mu.Lock()
	if err := fn(&r.Config); err != nil {
		r.mu.Unlock()
		return err
	}
	snapshot := cloneConfig(r.Config)
	// 站点表改了要立刻对新连接生效，所以在锁内换掉 Router 指针；
	// 请求侧读到的永远是某个完整版本，不会读到改了一半的表。
	r.Router = NewRouter(snapshot.Sites)
	r.mu.Unlock()

	if err := SaveConfig(snapshot); err != nil {
		return err
	}
	// 监听参数没变的端口只换转发表——改站点不会把正在传的连接断掉
	return r.pool.Apply(snapshot)
}

// CurrentRouter 取当前站点表快照，供请求路径使用
func (r *ProxyRuntime) CurrentRouter() *Router {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.Router
}

// FindSite 按 ID 取一个站点的拷贝
func (r *ProxyRuntime) FindSite(id uint) (model.Site, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	for _, s := range r.Config.Sites {
		if s.ID == id {
			return s, true
		}
	}
	return model.Site{}, false
}

// cloneConfig 切片复制一份，防止调用方拿着快照改到运行时配置
func cloneConfig(in model.ProxyConfig) model.ProxyConfig {
	out := in
	out.Sites = append([]model.Site(nil), in.Sites...)
	return out
}

// nextSiteID 返回下一个可用的站点 ID
func nextSiteID(cfg *model.ProxyConfig) uint {
	var max uint
	for _, s := range cfg.Sites {
		if s.ID > max {
			max = s.ID
		}
	}
	return max + 1
}
