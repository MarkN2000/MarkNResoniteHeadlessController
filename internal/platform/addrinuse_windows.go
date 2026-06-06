//go:build windows

package platform

import (
	"errors"

	"golang.org/x/sys/windows"
)

// IsAddrInUse は net.Listen のエラーが「アドレス使用中」かを判定する。
// Windows で net.Listen が実際に返すのは WSAEADDRINUSE(10048)。
// syscall.EADDRINUSE は APPLICATION_ERROR ベースの擬似値で一致しない
// （syscall.Errno.Is も EADDRINUSE をマップしない）ため、windows 定数で判定する。
func IsAddrInUse(err error) bool { return errors.Is(err, windows.WSAEADDRINUSE) }
