// Package logx 提供结构化日志：slog（标准库）+ lumberjack 文件轮转 + 敏感字段脱敏。
//
// 对齐 Python 端 app/core/logger.py（Loguru）与 app/core/router_class.py 的 _scrub：
// 写日志前剔除 password / oldPassword / newPassword / confirmPassword。
//
// 选用 slog 而非 zap：本项目日志需求是「结构化 + 轮转 + 脱敏」，slog 作为标准库
// 零依赖即可满足，少一层 API 学习成本。所有调用都收敛在本包，日后换实现只改这里。
package logx

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/natefinch/lumberjack.v2"
)

// ExcludeProperties 日志脱敏需剔除的字段名。
// 与 Python 端 SystemConstants.EXCLUDE_PROPERTIES 逐字一致；
// 键比较**不区分大小写**，以覆盖 snake_case 与 camelCase 两种写法。
var ExcludeProperties = map[string]struct{}{
	"password":        {},
	"oldpassword":     {},
	"newpassword":     {},
	"confirmpassword": {},
}

// RedactedValue 敏感字段被替换后的占位值。
const RedactedValue = "***"

// Options 日志初始化参数。
type Options struct {
	// Level 日志级别：DEBUG / INFO / WARNING / ERROR（不区分大小写）。
	Level string
	// Dir 日志目录；为空则只输出到 stdout（容器场景由 stdout 采集）。
	Dir string
	// Filename 日志文件名，如 walnut-seed-go.log。
	Filename string
	// MaxSizeMB 单文件大小上限（MB），超过即轮转。
	MaxSizeMB int
	// MaxBackups 保留的历史文件数。
	MaxBackups int
	// MaxAgeDays 历史文件保留天数。
	MaxAgeDays int
	// ConsoleJSON 为 true 时 stdout 输出 JSON；默认输出人类可读的 text。
	// Dir 非空时文件侧恒为 JSON（便于采集），不受本项影响。
	ConsoleJSON bool
	// ServiceName 注入到每条日志的固定字段，便于三端日志混排时区分来源。
	ServiceName string
}

// Setup 构造 logger 并设为 slog 默认值。
//
// 返回的 io.Closer 用于关闭日志文件句柄（lumberjack），**必须在停机时调用**：
// 否则文件句柄一直持有，Windows 上会导致目录无法删除（测试的 t.TempDir 清理失败），
// Linux 上则会让日志轮转后的旧文件无法被回收。无文件输出时返回 nil。
//
// 输出拓扑：
//   - Dir 为空：单一 handler 写 stdout（格式由 ConsoleJSON 决定）
//   - Dir 非空：fan-out 到「stdout text（人类可读）」+「文件 JSON（可采集）」
//     slog 单个 handler 只能一种格式，故用 splitHandler 分流
func Setup(opts Options) (*slog.Logger, io.Closer, error) {
	level, err := parseLevel(opts.Level)
	if err != nil {
		return nil, nil, err
	}
	handlerOpts := &slog.HandlerOptions{Level: level, ReplaceAttr: redactAttr}

	var h slog.Handler
	var closer io.Closer

	fileWriter, err := openFileWriter(opts)
	if err != nil {
		return nil, nil, err
	}

	switch {
	case fileWriter != nil:
		closer = fileWriter
		// stdout 的格式由 ConsoleJSON 决定（容器内建议 JSON 供采集器直接解析），
		// 文件侧恒为 JSON。此前 stdout 固定 text 会让 ConsoleJSON 在有文件输出时
		// 变成死配置，故在此显式分流。
		var stdoutHandler slog.Handler
		if opts.ConsoleJSON {
			stdoutHandler = slog.NewJSONHandler(os.Stdout, handlerOpts)
		} else {
			stdoutHandler = slog.NewTextHandler(os.Stdout, handlerOpts)
		}
		h = &splitHandler{
			primary:   stdoutHandler,
			secondary: slog.NewJSONHandler(fileWriter, handlerOpts),
		}
	case opts.ConsoleJSON:
		h = slog.NewJSONHandler(os.Stdout, handlerOpts)
	default:
		h = slog.NewTextHandler(os.Stdout, handlerOpts)
	}

	logger := slog.New(h)
	if opts.ServiceName != "" {
		logger = logger.With(slog.String("service", opts.ServiceName))
	}
	slog.SetDefault(logger)
	return logger, closer, nil
}

// openFileWriter 按需构造轮转文件写入器；Dir 为空返回 nil。
// 返回 *lumberjack.Logger（实现 io.WriteCloser），以便调用方停机时关闭句柄。
func openFileWriter(opts Options) (*lumberjack.Logger, error) {
	if opts.Dir == "" {
		return nil, nil
	}
	if err := os.MkdirAll(opts.Dir, 0o755); err != nil {
		return nil, fmt.Errorf("创建日志目录 %s 失败: %w", opts.Dir, err)
	}
	filename := opts.Filename
	if filename == "" {
		filename = "walnut-seed-go.log"
	}
	return &lumberjack.Logger{
		Filename:   filepath.Join(opts.Dir, filename),
		MaxSize:    orDefault(opts.MaxSizeMB, 50),
		MaxBackups: orDefault(opts.MaxBackups, 10),
		MaxAge:     orDefault(opts.MaxAgeDays, 30),
		LocalTime:  true,
		Compress:   false,
	}, nil
}

func orDefault(v, def int) int {
	if v <= 0 {
		return def
	}
	return v
}

func parseLevel(s string) (slog.Level, error) {
	switch strings.ToUpper(strings.TrimSpace(s)) {
	case "", "DEBUG":
		return slog.LevelDebug, nil
	case "INFO":
		return slog.LevelInfo, nil
	case "WARN", "WARNING":
		return slog.LevelWarn, nil
	case "ERROR":
		return slog.LevelError, nil
	default:
		return slog.LevelInfo, fmt.Errorf("未知日志级别 %q（接受 DEBUG/INFO/WARNING/ERROR）", s)
	}
}

// redactAttr 在属性写入前做脱敏。
//
// 两层处理：
//  1. 键名命中 ExcludeProperties → 整个值替换为 ***
//  2. 值是 map/slice/url.Values → 递归脱敏（对应 Python 端 _scrub 对 dict/list 的递归）
//
// slog 的 ReplaceAttr 对 group 内属性也会回调，故嵌套 group 能被覆盖；
// 但被 slog.Any 整体塞进去的 map 不会逐键回调，因此这里显式递归。
func redactAttr(_ []string, a slog.Attr) slog.Attr {
	if _, hit := ExcludeProperties[strings.ToLower(a.Key)]; hit {
		a.Value = slog.StringValue(RedactedValue)
		return a
	}
	// 字符串值不做内容扫描：查询串里可能出现 "password=" 形态，
	// 但那属于 URL 脱敏职责（在 middleware/requestlog 层处理），此处误伤正常文案代价更大
	if a.Value.Kind() == slog.KindAny {
		if scrubbed, changed := scrubAny(a.Value.Any()); changed {
			a.Value = slog.AnyValue(scrubbed)
		}
	}
	return a
}

// scrubAny 递归脱敏任意值，返回 (结果, 是否发生改动)。
// 未改动时返回原对象，避免无谓拷贝。
func scrubAny(v any) (any, bool) {
	switch val := v.(type) {
	case map[string]any:
		out := make(map[string]any, len(val))
		changed := false
		for k, item := range val {
			if _, hit := ExcludeProperties[strings.ToLower(k)]; hit {
				out[k] = RedactedValue
				changed = true
				continue
			}
			if scrubbed, c := scrubAny(item); c {
				out[k] = scrubbed
				changed = true
			} else {
				out[k] = item
			}
		}
		return decide(v, out, changed)

	case []any:
		out := make([]any, len(val))
		changed := false
		for i, item := range val {
			if scrubbed, c := scrubAny(item); c {
				out[i] = scrubbed
				changed = true
			} else {
				out[i] = item
			}
		}
		return decide(v, out, changed)

	case url.Values:
		out := make(url.Values, len(val))
		changed := false
		for k, items := range val {
			if _, hit := ExcludeProperties[strings.ToLower(k)]; hit {
				out[k] = []string{RedactedValue}
				changed = true
				continue
			}
			out[k] = items
		}
		return decide(v, out, changed)

	default:
		return v, false
	}
}

func decide(orig, scrubbed any, changed bool) (any, bool) {
	if !changed {
		return orig, false
	}
	return scrubbed, true
}

// Scrub 是 scrubAny 的导出入口，供请求日志 / 操作日志在序列化前主动脱敏。
func Scrub(v any) any {
	out, _ := scrubAny(v)
	return out
}

// splitHandler 把同一条日志分流到两个 handler（stdout text + 文件 JSON）。
//
// secondary 的错误被吞掉：文件写入失败（磁盘满、容器退出时句柄已关）
// 不应中断业务，也不应让 stdout 那一路丢失。
type splitHandler struct {
	primary   slog.Handler
	secondary slog.Handler
}

func (s *splitHandler) Enabled(ctx context.Context, level slog.Level) bool {
	return s.primary.Enabled(ctx, level) || s.secondary.Enabled(ctx, level)
}

func (s *splitHandler) Handle(ctx context.Context, r slog.Record) error {
	// 两路都要投递；primary（stdout）的错误向上返回，secondary 忽略
	err := s.primary.Handle(ctx, r.Clone())
	_ = s.secondary.Handle(ctx, r.Clone())
	return err
}

func (s *splitHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	return &splitHandler{primary: s.primary.WithAttrs(attrs), secondary: s.secondary.WithAttrs(attrs)}
}

func (s *splitHandler) WithGroup(name string) slog.Handler {
	return &splitHandler{primary: s.primary.WithGroup(name), secondary: s.secondary.WithGroup(name)}
}

var _ slog.Handler = (*splitHandler)(nil)
