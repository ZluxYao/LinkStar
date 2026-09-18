package cert

import (
	"linkstar/modules/cert/model"
	"sync"
)

type CertRuntime struct {
	mu        sync.RWMutex
	Config    model.CertConfig
	Manager   *Manager
	Scheduler *Scheduler
}

var Runtime = &CertRuntime{
	Manager: NewManager(),
}
