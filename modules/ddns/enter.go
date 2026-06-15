package ddns

import (
	"linkstar/modules/ddns/model"
	"sync"
)

type DDNSRuntime struct {
	mu        sync.RWMutex
	Config    model.DDNSConfig
	Scheduler *Scheduler
}

var Runtime = &DDNSRuntime{}
