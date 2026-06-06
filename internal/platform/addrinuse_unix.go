//go:build !windows

package platform

import (
	"errors"
	"syscall"
)

// IsAddrInUse は net.Listen のエラーが「アドレス使用中」かを判定する。
func IsAddrInUse(err error) bool { return errors.Is(err, syscall.EADDRINUSE) }
