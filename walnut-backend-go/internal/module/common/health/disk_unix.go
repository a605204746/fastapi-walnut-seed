//go:build linux || darwin || freebsd

package health

import "syscall"

// diskUsagePercent 返回根文件系统使用率（百分比，1 位小数）。
//
// 对齐 Python 端 shutil.disk_usage("/")：
//
//	total = f_blocks * f_frsize
//	used  = (f_blocks - f_bfree) * f_frsize
//
// 失败时返回 -1.0，与 Python 端 except 分支的 `return -1.0` 一致。
func diskUsagePercent() float64 {
	var stat syscall.Statfs_t
	if err := syscall.Statfs("/", &stat); err != nil {
		return -1.0
	}
	blockSize := uint64(stat.Bsize)
	if blockSize == 0 || stat.Blocks == 0 {
		return -1.0
	}
	total := stat.Blocks * blockSize
	// Bfree 可能大于 Blocks（异常文件系统），做下溢保护
	free := stat.Bfree
	if free > stat.Blocks {
		free = stat.Blocks
	}
	used := (stat.Blocks - free) * blockSize
	return roundTo(float64(used)/float64(total)*100, 1)
}
