//go:build windows

package health

import (
	"syscall"
	"unsafe"
)

var (
	kernel32               = syscall.NewLazyDLL("kernel32.dll")
	procGetDiskFreeSpaceEx = kernel32.NewProc("GetDiskFreeSpaceExW")
)

// diskUsagePercent 返回当前驱动器使用率（百分比，1 位小数）。
//
// 必须与 Python 端 shutil.disk_usage("/") 对齐：
//   - Python 在 Windows 上经 os.statvfs 模拟，"/" 解析为**当前驱动器根**，返回真实百分比
//   - shutil 的算法是 used = (f_blocks - f_bfree) * f_frsize，即用的是「总空闲」而非
//     「非 root 可用」，对应 GetDiskFreeSpaceEx 的 TotalNumberOfFreeBytes（第三个出参），
//     而不是 FreeBytesAvailableToCaller（第二个出参）
//
// 传 "." 而非 "\\"：GetDiskFreeSpaceEx 接受卷上任意路径并返回该卷信息，
// "." 解析到进程 CWD 所在卷，与 Python 的 "/" → 当前驱动器语义一致。
//
// 此前 Windows 分支直接返回 -1.0，导致本机开发时与 Python 端读数不同
// （PY=94.3 / GO=-1）。容器内走 disk_unix.go 不受影响，但契约偏差应当消除而不是容忍。
func diskUsagePercent() float64 {
	path, err := syscall.UTF16PtrFromString(".")
	if err != nil {
		return -1.0
	}

	var (
		freeBytesAvailableToCaller uint64
		totalNumberOfBytes         uint64
		totalNumberOfFreeBytes     uint64
	)

	ret, _, _ := procGetDiskFreeSpaceEx.Call(
		uintptr(unsafe.Pointer(path)),
		uintptr(unsafe.Pointer(&freeBytesAvailableToCaller)),
		uintptr(unsafe.Pointer(&totalNumberOfBytes)),
		uintptr(unsafe.Pointer(&totalNumberOfFreeBytes)),
	)
	if ret == 0 {
		return -1.0
	}
	if totalNumberOfBytes == 0 {
		return -1.0
	}
	// 下溢保护：异常卷上 FreeBytes 可能大于 TotalBytes
	free := totalNumberOfFreeBytes
	if free > totalNumberOfBytes {
		free = totalNumberOfBytes
	}
	used := totalNumberOfBytes - free
	return roundTo(float64(used)/float64(totalNumberOfBytes)*100, 1)
}
