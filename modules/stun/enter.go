package stun

import "linkstar/modules/stun/model"

type STUNRuntime struct {
	Config      model.Config
	Network     model.NetworkState
	STUNService *STUNService
	UpnpGateway *model.UpnpGateway
	Scheduler   *Scheduler
}

var Runtime = &STUNRuntime{}
