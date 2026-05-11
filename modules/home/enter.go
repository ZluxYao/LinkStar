package home

import "sync"

type HomeRuntime struct {
	mu     sync.RWMutex
	Config Config
}

var Runtime = &HomeRuntime{}
