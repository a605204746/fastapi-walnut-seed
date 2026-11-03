// Package databasex 封装 GORM 初始化、连接池调优与建库引导。
//
// 与 Python（SQLAlchemy async）/ Java（HikariCP）的等价职责，但有一处 Go 特有差异：
// **go-sql-driver/mysql 不支持 JDBC 的 createDatabaseIfNotExist**，
// 而中间件编排的 MYSQL_DATABASE 只能建一个库。因此库 walnut_seed_go
// 必须由本包的 EnsureDatabase 显式引导创建（对应 P1 任务 1.1）。
package databasex

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"net/url"
	"os"
	"time"

	_ "github.com/go-sql-driver/mysql"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
	"gorm.io/gorm/schema"
)

// Params 连接参数，由 config.Config 映射而来。
// 不直接依赖 config 包，避免基础设施层反向依赖配置层，也便于测试注入。
type Params struct {
	Host     string
	Port     int
	User     string
	Password string
	Name     string

	// PoolSize 常驻连接数（对应 SQLAlchemy 的 POOL_SIZE）
	PoolSize int
	// MaxOverflow 峰值额外连接数（对应 MAX_OVERFLOW）
	MaxOverflow int
	// PoolTimeout 获取连接的最长等待秒数
	PoolTimeout int
	// PoolRecycle 连接最长存活秒数，**必须小于 MySQL 的 wait_timeout**，
	// 否则池中会积累被服务端单方面关闭的连接，表现为大量 invalid connection
	PoolRecycle int
	// Echo 打印 SQL（对应 DATABASE_ECHO）
	Echo bool
}

// DSN 构造业务库 DSN。
//
// 参数对齐三端约定：
//   - charset=utf8mb4：与 Python 端 ASYNC_DB_URI 一致
//   - parseTime=true：让 time.Time 自动映射，否则 GORM 拿到的是 []byte
//   - loc=Local：配合容器 TZ=Asia/Shanghai，与 Java 端 serverTimezone=GMT+8 等价
func (p Params) DSN() string {
	return fmt.Sprintf("%s:%s@tcp(%s:%d)/%s?charset=utf8mb4&parseTime=true&loc=Local",
		p.User, url.QueryEscape(p.Password), p.Host, p.Port, p.Name)
}

// serverDSN 构造不带库名的 DSN，仅用于建库引导。
func (p Params) serverDSN() string {
	return fmt.Sprintf("%s:%s@tcp(%s:%d)/?charset=utf8mb4&parseTime=true&loc=Local",
		p.User, url.QueryEscape(p.Password), p.Host, p.Port)
}

// EnsureDatabase 在目标库不存在时创建它。
//
// 库名不能由调用方任意拼接进 SQL（注入风险），故先做标识符白名单校验，
// 再用反引号包裹。字符集与排序规则对齐 Java 端 Flyway 建库结果。
func EnsureDatabase(ctx context.Context, p Params) error {
	if !isSafeIdent(p.Name) {
		return fmt.Errorf("非法数据库名 %q：仅允许字母、数字、下划线", p.Name)
	}

	conn, err := sql.Open("mysql", p.serverDSN())
	if err != nil {
		return fmt.Errorf("连接 MySQL 服务器失败: %w", err)
	}
	defer conn.Close()

	conn.SetConnMaxLifetime(30 * time.Second)
	if err := conn.PingContext(ctx); err != nil {
		return fmt.Errorf("MySQL 服务器不可达 (%s:%d): %w", p.Host, p.Port, err)
	}

	// 排序规则必须是 utf8mb4_unicode_ci，与另两端一致：
	//   - 中间件编排设了 --collation-server=utf8mb4_unicode_ci，
	//     Python 库经 MYSQL_DATABASE 建库时继承该默认值（实测 default_collation_name=utf8mb4_unicode_ci）
	//   - 表级 DDL 也显式声明 COLLATE=utf8mb4_unicode_ci
	// 曾误写为 utf8mb4_general_ci，导致库默认排序规则与 Python 库不同（计划 R13 那类漂移）。
	// 表虽然显式指定了 collation 不受影响，但**后续新增表若省略 COLLATE 就会继承错的默认值**，
	// 且 JOIN 两端 collation 不一致时 MySQL 会报 illegal mix of collations。
	stmt := fmt.Sprintf(
		"CREATE DATABASE IF NOT EXISTS `%s` CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci",
		p.Name,
	)
	if _, err := conn.ExecContext(ctx, stmt); err != nil {
		return fmt.Errorf("创建数据库 %s 失败: %w", p.Name, err)
	}
	return nil
}

// isSafeIdent 校验 SQL 标识符是否只含安全字符。
func isSafeIdent(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '_':
		default:
			return false
		}
	}
	return true
}

// Open 建立 GORM 连接并完成连接池调优。调用方负责 Close。
func Open(ctx context.Context, p Params) (*gorm.DB, error) {
	logLevel := logger.Warn
	if p.Echo {
		logLevel = logger.Info
	}

	db, err := gorm.Open(mysql.New(mysql.Config{
		DSN:                       p.DSN(),
		SkipInitializeWithVersion: false,
	}), &gorm.Config{
		// 命名策略对齐 Python 端 base_model.py 的 NAMING_CONVENTION（ix_* / uq_* / fk_* / ck_*）
		NamingStrategy: schema.NamingStrategy{
			TablePrefix:   "",
			SingularTable: true, // 表名不自动复数化，模型显式声明 TableName()
		},
		Logger: newLogger(logLevel),
		// 由本函数显式 PingContext，不要让 gorm.Open 内部再 Ping 一次
		DisableAutomaticPing: true,
		// 禁用自动建外键约束：Schema 的唯一事实源是 migrations/ 下的版本化 SQL。
		// 同理，**全项目禁止调用 AutoMigrate**——它会绕过迁移历史，造成三端 schema 漂移
		DisableForeignKeyConstraintWhenMigrating: true,
		// 把 MySQL 的重复键/外键错误翻译成 gorm.ErrDuplicatedKey / ErrForeignKeyViolated，
		// 便于 errs 层统一映射成 409 文案（对齐 Python 端的 IntegrityError 分支）
		TranslateError: true,
		PrepareStmt:    true, // 复用预处理语句，降低 MySQL 解析开销
	})
	if err != nil {
		return nil, fmt.Errorf("GORM 初始化失败: %w", err)
	}

	sqlDB, err := db.DB()
	if err != nil {
		return nil, fmt.Errorf("获取底层 *sql.DB 失败: %w", err)
	}

	// ---- 连接池调优 ----
	// 语义映射：SQLAlchemy 的 pool_size 是常驻连接、max_overflow 是峰值额外连接，
	// Go 的 MaxOpenConns 是总上限，故总上限 = PoolSize + MaxOverflow
	sqlDB.SetMaxOpenConns(positive(p.PoolSize, 10) + positive(p.MaxOverflow, 20))
	sqlDB.SetMaxIdleConns(positive(p.PoolSize, 10))
	sqlDB.SetConnMaxLifetime(time.Duration(positive(p.PoolRecycle, 1800)) * time.Second)
	sqlDB.SetConnMaxIdleTime(time.Duration(positive(p.PoolRecycle, 1800)) * time.Second / 2)

	if err := sqlDB.PingContext(ctx); err != nil {
		return nil, fmt.Errorf("数据库 %s 连接失败: %w", p.Name, err)
	}
	return db, nil
}

func positive(v, def int) int {
	if v <= 0 {
		return def
	}
	return v
}

// newLogger 构造 GORM 日志器。
//
// ⚠️ 必须开 IgnoreRecordNotFoundError，不能直接用 logger.Default：
// `gorm.ErrRecordNotFound` 在本项目里是**正常业务结果**——repo 层大量使用
// First() 后判断「记录不存在则返回 (nil, nil)」（如按 clientId 查客户端、
// 按用户名查用户），调用方据此走"未知客户端""用户不存在"等业务分支。
//
// GORM 默认会把它按 error 级别打到 stdout：
//
//	.../auth/repo.go:69 record not found
//	[1.784ms] [rows:0] SELECT * FROM sys_client WHERE client_id = 'nonexistent-client' ...
//
// 生产环境每个未知 clientId / 不存在用户的请求都会刷一条，属于纯噪音，
// 且会把真正的 SQL 错误淹没。Python 端 SQLAlchemy 的 scalars().first()
// 返回 None 是静默的，故忽略该错误也是为了两端日志行为一致。
//
// 其余 SQL 错误仍然照常记录（含慢查询），排障能力不受影响。
func newLogger(level logger.LogLevel) logger.Interface {
	return logger.New(
		log.New(os.Stdout, "", log.LstdFlags),
		logger.Config{
			SlowThreshold:             200 * time.Millisecond,
			LogLevel:                  level,
			IgnoreRecordNotFoundError: true,
			Colorful:                  false,
			// 不打印调用方行号：GORM 的 CallerSkip 计算在有封装层时经常指错位置，
			// 反而误导排查。SQL 文本本身已足够定位。
			ParameterizedQueries: false,
		},
	)
}

// OpenForMigration 建立**迁移专用**连接（原生 *sql.DB，不走 GORM）。
//
// ⚠️ 安全边界：这条连接开了 `multiStatements=true`，因为单个迁移文件含多条 SQL
// （0001_init_schema.up.sql 有 17 条 CREATE TABLE），不开的话驱动只执行第一条、
// 其余静默丢弃。而 multiStatements 一旦用于**应用运行期**连接，SQL 注入就能
// 拼接执行任意多条语句——所以必须隔离成独立连接，且只在迁移期间短暂存在。
//
// 应用运行期的连接（Open）保持 multiStatements 关闭。
// 连接池刻意收窄：迁移是串行短任务，不需要应用级的池容量。
func OpenForMigration(ctx context.Context, p Params) (*sql.DB, error) {
	dsn := fmt.Sprintf("%s:%s@tcp(%s:%d)/%s?charset=utf8mb4&parseTime=true&loc=Local&multiStatements=true",
		p.User, url.QueryEscape(p.Password), p.Host, p.Port, p.Name)

	db, err := sql.Open("mysql", dsn)
	if err != nil {
		return nil, fmt.Errorf("打开迁移连接失败: %w", err)
	}
	db.SetMaxOpenConns(2)
	db.SetMaxIdleConns(1)
	db.SetConnMaxLifetime(5 * time.Minute)

	if err := db.PingContext(ctx); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("迁移连接不可用 (%s:%d/%s): %w", p.Host, p.Port, p.Name, err)
	}
	return db, nil
}

// Ping 健康探测，供 /common/health/ready 使用。
func Ping(ctx context.Context, db *gorm.DB) error {
	if db == nil {
		return fmt.Errorf("数据库未初始化")
	}
	sqlDB, err := db.DB()
	if err != nil {
		return err
	}
	return sqlDB.PingContext(ctx)
}

// Close 关闭连接池。
func Close(db *gorm.DB) error {
	if db == nil {
		return nil
	}
	sqlDB, err := db.DB()
	if err != nil {
		return err
	}
	return sqlDB.Close()
}
