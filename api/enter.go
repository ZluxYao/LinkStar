package api

import (
	"linkstar/api/auth_api"
	"linkstar/api/ddns_api"
	"linkstar/api/home_api"
	"linkstar/api/stun_api"
	"linkstar/api/system_api"
	"linkstar/api/webhook_api"
)

type Api struct {
	StunApi    stun_api.StunApi
	HomeApi    home_api.HomeApi
	DdnsApi    ddns_api.DdnsApi
	WebhookApi webhook_api.WebhookApi
	AuthApi    auth_api.AuthApi
	SystemApi  system_api.SystemApi
}

var App = new(Api)
