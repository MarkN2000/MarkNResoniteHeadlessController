//go:build linux

package sysmetrics

import (
	"bufio"
	"os"
	"strconv"
	"strings"
	"syscall"
)

// readCPUTimes は /proc/stat の先頭 "cpu" 行から busy/total の累積ティックを返す。
// フィールド: user nice system idle iowait irq softirq steal [guest guest_nice]。
// guest/guest_nice は user/nice に二重計上されているため先頭8項目までで打ち切る。
// idle = idle + iowait、busy = total - idle。
func readCPUTimes() (busy, total uint64, err error) {
	f, err := os.Open("/proc/stat")
	if err != nil {
		return 0, 0, err
	}
	defer f.Close()

	sc := bufio.NewScanner(f)
	if !sc.Scan() {
		if e := sc.Err(); e != nil {
			return 0, 0, e
		}
		return 0, 0, ErrUnsupported
	}
	fields := strings.Fields(sc.Text())
	if len(fields) < 5 || fields[0] != "cpu" {
		return 0, 0, ErrUnsupported
	}
	vals := make([]uint64, 0, 8)
	for _, fld := range fields[1:] {
		v, e := strconv.ParseUint(fld, 10, 64)
		if e != nil {
			break
		}
		vals = append(vals, v)
		if len(vals) == 8 { // user..steal（guest 以降は二重計上のため除外）
			break
		}
	}
	if len(vals) < 5 {
		return 0, 0, ErrUnsupported
	}
	for _, v := range vals {
		total += v
	}
	idle := vals[3] + vals[4] // idle + iowait
	busy = total - idle
	return busy, total, nil
}

// readMem は /proc/meminfo から used/total（バイト）と使用率を返す。
// MemAvailable（カーネル 3.14+）を優先し、無ければ MemFree+Buffers+Cached で近似する。
func readMem() (used, total uint64, percent float64, err error) {
	f, err := os.Open("/proc/meminfo")
	if err != nil {
		return 0, 0, 0, err
	}
	defer f.Close()

	var memTotal, memAvail, memFree, buffers, cached uint64
	var haveAvail bool
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		key, val, ok := strings.Cut(sc.Text(), ":")
		if !ok {
			continue
		}
		kb := parseMeminfoKB(val)
		switch key {
		case "MemTotal":
			memTotal = kb
		case "MemAvailable":
			memAvail = kb
			haveAvail = true
		case "MemFree":
			memFree = kb
		case "Buffers":
			buffers = kb
		case "Cached":
			cached = kb
		}
	}
	if e := sc.Err(); e != nil {
		return 0, 0, 0, e
	}
	if memTotal == 0 {
		return 0, 0, 0, ErrUnsupported
	}
	avail := memAvail
	if !haveAvail {
		avail = memFree + buffers + cached
	}
	if avail > memTotal {
		avail = memTotal
	}
	used = memTotal - avail
	percent = float64(used) / float64(memTotal) * 100
	return used, memTotal, percent, nil
}

// parseMeminfoKB は "  16384000 kB" のような値部分を bytes に変換する（kB→×1024）。
func parseMeminfoKB(s string) uint64 {
	s = strings.TrimSpace(strings.TrimSuffix(strings.TrimSpace(s), "kB"))
	n, _ := strconv.ParseUint(strings.TrimSpace(s), 10, 64)
	return n * 1024
}

// readDisk は path が属するファイルシステムの空き（非特権ユーザー利用可）/総容量を返す。
func readDisk(path string) (free, total uint64, err error) {
	var st syscall.Statfs_t
	if err := syscall.Statfs(path, &st); err != nil {
		return 0, 0, err
	}
	bs := uint64(st.Bsize) // arch により int32/int64 のため明示変換
	free = uint64(st.Bavail) * bs
	total = uint64(st.Blocks) * bs
	return free, total, nil
}
