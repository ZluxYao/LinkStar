//go:build !windows

package stun

import "os/exec"

func hideCommandWindow(cmd *exec.Cmd) {}
