package stun_api

import (
	"fmt"
	"linkstar/middleware"
	"linkstar/modules/home"
	"linkstar/modules/stun"
	stunModel "linkstar/modules/stun/model"
	"linkstar/utils/res"
	"time"

	"github.com/gin-gonic/gin"
)

type StunServiceShowOnHomeRequest struct {
	DeviceID  uint `json:"deviceId"`
	ServiceID uint `json:"serviceId"`
	Show      bool `json:"show"`
}

// 切换某个 stun service 是否显示在 home,内部调 home runtime
// 注意:stun_api 可以同时引用 stun + home(没有循环 import),但 modules/stun 自身不感知 home
func (StunApi) StunServiceShowOnHomeView(c *gin.Context) {
	cr := middleware.GetBindRequest[StunServiceShowOnHomeRequest](c)

	var device *stunModel.Device
	var service *stunModel.Service
	for i := range stun.Runtime.Config.Devices {
		d := &stun.Runtime.Config.Devices[i]
		if d.DeviceID != cr.DeviceID {
			continue
		}
		device = d
		for j := range d.Services {
			s := &d.Services[j]
			if s.ID == cr.ServiceID {
				service = s
				break
			}
		}
		break
	}
	if device == nil || service == nil {
		res.FailWithMsg("设备或服务不存在", c)
		return
	}

	if cr.Show {
		// 已存在则幂等成功
		if home.Runtime.HasStunApp(cr.DeviceID, cr.ServiceID) {
			res.OkWithMsg("已在 home 显示", c)
			return
		}
		err := home.Runtime.WithLock(func(cfg *home.Config) error {
			now := time.Now()
			maxPaged, maxScroll := 0, 0
			for _, a := range cfg.Apps {
				if a.PagedOrder > maxPaged {
					maxPaged = a.PagedOrder
				}
				if a.CategoryID == "" && a.ScrollOrder > maxScroll {
					maxScroll = a.ScrollOrder
				}
			}
			cfg.Apps = append(cfg.Apps, home.App{
				ID:            fmt.Sprintf("stun-%d-%d", cr.DeviceID, cr.ServiceID),
				Name:          service.Name,
				PagedOrder:    maxPaged + 1,
				ScrollOrder:   maxScroll + 1,
				Type:          "stun",
				StunDeviceID:  cr.DeviceID,
				StunServiceID: cr.ServiceID,
				CreatedAt:     now,
				UpdatedAt:     now,
			})
			return nil
		})
		if err != nil {
			res.FailWithMsg("保存配置失败", c)
			return
		}
		res.OkWithMsg("已加入 home", c)
		return
	}

	// show=false → 删除对应 home app（如果存在）
	home.Runtime.RemoveAppByStunService(cr.DeviceID, cr.ServiceID)
	res.OkWithMsg("已从 home 移除", c)
}
