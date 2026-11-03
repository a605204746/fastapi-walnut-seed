//go:build !linux && !darwin && !freebsd && !windows

package health

// diskUsagePercent 在既非类 Unix 也非 Windows 的平台恒返回 -1.0。
//
// 三个实现的分工：
//   - disk_unix.go    (linux/darwin/freebsd)：syscall.Statfs("/")，生产容器走这条
//   - disk_windows.go (windows)：GetDiskFreeSpaceEx(".")，本机开发走这条
//   - disk_other.go   (其余)：-1.0 兜底
//
// ⚠️ 本文件的构建标签**必须显式排除 windows**，否则会与 disk_windows.go 重复定义。
// 曾一度写成 `!linux && !darwin && !freebsd`（隐含包含 windows）导致编译冲突。
//
// 返回值 -1.0 与 Python 端 shutil.disk_usage 失败时的 `return -1.0` 分支一致。
func diskUsagePercent() float64 { return -1.0 }
