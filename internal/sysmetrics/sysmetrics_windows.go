//go:build windows

package sysmetrics

import (
	"unsafe"

	"golang.org/x/sys/windows"
)

// kernel32 の関数は x/sys/windows の遅延ロード機構経由で呼ぶ（新規依存なし）。
var (
	modkernel32              = windows.NewLazySystemDLL("kernel32.dll")
	procGetSystemTimes       = modkernel32.NewProc("GetSystemTimes")
	procGlobalMemoryStatusEx = modkernel32.NewProc("GlobalMemoryStatusEx")
	procGetDiskFreeSpaceExW  = modkernel32.NewProc("GetDiskFreeSpaceExW")
)

// memoryStatusEx は MEMORYSTATUSEX（GlobalMemoryStatusEx の出力先）。
type memoryStatusEx struct {
	Length               uint32
	MemoryLoad           uint32 // 物理メモリ使用率 0..100（OS 権威値）
	TotalPhys            uint64
	AvailPhys            uint64
	TotalPageFile        uint64
	AvailPageFile        uint64
	TotalVirtual         uint64
	AvailVirtual         uint64
	AvailExtendedVirtual uint64
}

func filetimeToUint64(ft windows.Filetime) uint64 {
	return uint64(ft.HighDateTime)<<32 | uint64(ft.LowDateTime)
}

// readCPUTimes は GetSystemTimes（idle/kernel/user）から busy/total を返す。
// kernel には idle が含まれるため total = kernel + user、busy = total - idle。
func readCPUTimes() (busy, total uint64, err error) {
	var idle, kernel, user windows.Filetime
	r1, _, e := procGetSystemTimes.Call(
		uintptr(unsafe.Pointer(&idle)),
		uintptr(unsafe.Pointer(&kernel)),
		uintptr(unsafe.Pointer(&user)),
	)
	if r1 == 0 {
		return 0, 0, e
	}
	idleT := filetimeToUint64(idle)
	total = filetimeToUint64(kernel) + filetimeToUint64(user)
	busy = total - idleT
	return busy, total, nil
}

// readMem は GlobalMemoryStatusEx から used/total（バイト）と使用率（dwMemoryLoad）を返す。
func readMem() (used, total uint64, percent float64, err error) {
	var m memoryStatusEx
	m.Length = uint32(unsafe.Sizeof(m))
	r1, _, e := procGlobalMemoryStatusEx.Call(uintptr(unsafe.Pointer(&m)))
	if r1 == 0 {
		return 0, 0, 0, e
	}
	total = m.TotalPhys
	used = m.TotalPhys - m.AvailPhys
	percent = float64(m.MemoryLoad)
	return used, total, percent, nil
}

// readDisk は GetDiskFreeSpaceEx から空き（呼び出しユーザー利用可）/総容量を返す。
func readDisk(path string) (free, total uint64, err error) {
	p, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return 0, 0, err
	}
	var freeAvail, totalBytes, totalFree uint64
	r1, _, e := procGetDiskFreeSpaceExW.Call(
		uintptr(unsafe.Pointer(p)),
		uintptr(unsafe.Pointer(&freeAvail)),
		uintptr(unsafe.Pointer(&totalBytes)),
		uintptr(unsafe.Pointer(&totalFree)),
	)
	if r1 == 0 {
		return 0, 0, e
	}
	return freeAvail, totalBytes, nil
}
