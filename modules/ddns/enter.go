package ddns

import "linkstar/modules/ddns/model"

type DDNSRuntime struct {
	Config    model.DDNSConfig
	Scheduler *Scheduler
}

var Runtime = &DDNSRuntime{}
