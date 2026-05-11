package home

import (
	"linkstar/modules/stun"

	"github.com/sirupsen/logrus"
)

// registerStunHooks 让 home 订阅 stun 单向通知，stun 不感知 home 的存在
func registerStunHooks() {
	stun.RegisterOnServiceDeleted(func(deviceID, serviceID uint) {
		Runtime.RemoveAppByStunService(deviceID, serviceID)
		logrus.Debugf("[home] 同步删除 stun app device=%d service=%d", deviceID, serviceID)
	})
}
