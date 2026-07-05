//go:build windows

package stun

import (
	"os/exec"
	"syscall"
)

func hideCommandWindow(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{
		HideWindow: true,
	}
}
