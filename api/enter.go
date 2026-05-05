package api

import (
	"linkstar/api/home_api"
	"linkstar/api/stun_api"
)

type Api struct {
	StunApi stun_api.StunApi
	HomeApi home_api.HomeApi
}

var App = new(Api)
