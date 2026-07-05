package core

import (
	"os"
	"os/signal"
	"sync"
	"syscall"

	"github.com/sirupsen/logrus"
)

var (
	shutdownMu    sync.Mutex
	shutdownFuncs []func()
	shutdownRan   bool
)

// OnShutdown 注册一个退出时执行的保存函数，由各模块调用。
func OnShutdown(fn func()) {
	if fn == nil {
		return
	}
	shutdownMu.Lock()
	shutdownFuncs = append(shutdownFuncs, fn)
	shutdownMu.Unlock()
}

// RunShutdown 按注册顺序执行所有保存函数，保证只执行一次。
func RunShutdown() {
	shutdownMu.Lock()
	if shutdownRan {
		shutdownMu.Unlock()
		return
	}
	shutdownRan = true
	funcs := append([]func(){}, shutdownFuncs...)
	shutdownMu.Unlock()

	for _, fn := range funcs {
		fn()
	}
}

// ListenShutdown 统一监听退出信号，按注册顺序执行所有保存函数后退出。
func ListenShutdown() {
	signalChan := make(chan os.Signal, 1)
	signal.Notify(signalChan, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		sig := <-signalChan
		logrus.Infof("收到退出信号：%v ,正在保存配置文件", sig)

		RunShutdown()

		logrus.Info("配置保存完成，程序退出")
		os.Exit(0)
	}()
}
