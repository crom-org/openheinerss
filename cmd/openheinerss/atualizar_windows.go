//go:build windows

package main

import "syscall"

func atributosDesacoplado() *syscall.SysProcAttr { return &syscall.SysProcAttr{} }

func donoSouEu(path string) bool { return false }

func sinalizarTerm(pid int) error { return syscall.EWINDOWS }
