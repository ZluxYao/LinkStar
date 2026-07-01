package api

import (
	"linkstar/api/ddns_api"
	"linkstar/api/home_api"
	"linkstar/api/stun_api"
	"linkstar/api/webhook_api"
)

type Api struct {
	StunApi    stun_api.StunApi
	HomeApi    home_api.HomeApi
	DdnsApi    ddns_api.DdnsApi
	WebhookApi webhook_api.WebhookApi
}

var App = new(Api)
