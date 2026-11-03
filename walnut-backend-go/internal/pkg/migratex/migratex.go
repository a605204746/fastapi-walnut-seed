// Package migratex 封装 golang-migrate，提供与 Python 端 Alembic / Java 端 Flyway
// 等价的迁移能力。
//
// ⚠️ **全项目禁止使用 GORM 的 AutoMigrate**：Schema 的唯一事实源是 migrations/ 下的
// 版本化 SQL。AutoMigrate 会绕过迁移历史直接改表，造成三端 schema 漂移且无法回溯。
//
// 与另两端的职责对应：
//
//	Python  uv run main.py upgrade        ↔  go run . migrate up
//	Python  uv run main.py downgrade -r-1 ↔  go run . migrate down -r 1
//	Python  uv run main.py current        ↔  go run . migrate current
//	Java    Flyway 启动时自动执行          ↔  docker-entrypoint.sh 先 migrate up 再 run
package migratex

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/golang-migrate/migrate/v4"
	migratemysql "github.com/golang-migrate/migrate/v4/database/mysql"
	_ "github.com/golang-migrate/migrate/v4/source/file" // 注册 file:// 源驱动

	"walnut-seed-go/internal/pkg/databasex"
)

// ErrNilVersion 表示数据库尚未执行过任何迁移。
var ErrNilVersion = migrate.ErrNilVersion

// MigrationsTable 版本记录表名。
// golang-migrate 的默认值就是 schema_migrations，此处显式声明以免依赖默认值。
const MigrationsTable = "schema_migrations"

// Runner 持有一个已连接的迁移器。用完须 Close。
type Runner struct {
	m *migrate.Migrate
	// ownedDB 非 nil 表示连接是本 Runner 自己开的，Close 时需一并关闭；
	// 为 nil 表示连接由调用方传入，生命周期归调用方（不能替它关，
	// 否则会把应用正在用的连接池一起关掉）
	ownedDB *sql.DB
}

// Open 打开**迁移专用连接**并构造 Runner，Close 时一并释放连接。
//
// 这是 CLI 与应用启动期的常规入口：调用方不需要自己管理 *sql.DB。
// 连接由 databasex.OpenForMigration 建立（带 multiStatements=true，
// 与应用运行期连接隔离，详见该函数注释）。
func Open(ctx context.Context, p databasex.Params, dir string) (*Runner, error) {
	db, err := databasex.OpenForMigration(ctx, p)
	if err != nil {
		return nil, err
	}
	r, err := New(db, dir)
	if err != nil {
		_ = db.Close()
		return nil, err
	}
	r.ownedDB = db
	return r, nil
}

// New 用调用方提供的 *sql.DB 建立迁移器。
//
// **刻意不用 `migrate.New(sourceURL, databaseURL)` 的 URL 形式**，原因有二：
//
//  1. golang-migrate 的 mysql URL 解析对 host:port 的处理与 go-sql-driver 的 DSN
//     语法不完全对齐，实测报 `default addr for network 'localhost:3307' unknown`
//     ——DSN 里地址必须包成 `tcp(host:port)`，URL 形式不保证这点。
//  2. 更重要的是**安全**：迁移文件含多条语句，必须开 `multiStatements=true`；
//     而这个参数一旦加到应用主 DSN 上，SQL 注入就能拼接执行多条语句。
//     所以迁移必须走**独立连接**（见 databasex.OpenForMigration），
//     应用运行期的连接保持 multiStatements 关闭。
//
// 目标库必须已存在（由 databasex.EnsureDatabase 负责创建）——
// golang-migrate 不会建库，连不上就直接失败。
//
// 传入的 *sql.DB 生命周期由调用方管理，Close 不会关闭它——
// 否则会把应用正在用的连接池一起关掉。
func New(db *sql.DB, dir string) (*Runner, error) {
	if db == nil {
		return nil, errors.New("迁移需要一个有效的 *sql.DB")
	}
	absDir, err := resolveDir(dir)
	if err != nil {
		return nil, err
	}

	driver, err := migratemysql.WithInstance(db, &migratemysql.Config{
		MigrationsTable: MigrationsTable,
		// 不设 ForceLockHandler：单机开发够用；多副本并发迁移由
		// entrypoint 前置执行 + 滚动发布流程规避（与 Python 端同一策略）
	})
	if err != nil {
		return nil, fmt.Errorf("初始化 mysql 迁移驱动失败: %w", err)
	}

	// 用 NewWithDatabaseInstance：source 走 file:// URL，database 用我们自建的实例。
	//
	// ⚠️ 不要用 migrate.New(sourceURL, databaseURL)：它要自己解析数据库 URL，
	//    而其 mysql URL 方言与 go-sql-driver 的 DSN 语法不完全对齐，
	//    实测报 `default addr for network 'localhost:3307' unknown`。
	// ⚠️ 也不要用 NewWithInstance / NewWithSourceInstance：前者要求 source.Driver
	//    实例，后者第一个参数是 sourceName 而非 URL，都更容易用错。
	m, err := migrate.NewWithDatabaseInstance("file://"+filepath.ToSlash(absDir), "mysql", driver)
	if err != nil {
		return nil, fmt.Errorf("初始化迁移器失败（目录 %s）: %w", absDir, err)
	}
	return &Runner{m: m}, nil
}

// resolveDir 把迁移目录解析为绝对路径并校验存在。
//
// file:// 源对相对路径的解析依赖工作目录，而容器内 WORKDIR 与本机 CWD 不同，
// 故统一转绝对路径，避免「本机正常、容器里找不到脚本」这类难查的问题。
func resolveDir(dir string) (string, error) {
	if dir == "" {
		dir = "migrations"
	}
	abs, err := filepath.Abs(dir)
	if err != nil {
		return "", fmt.Errorf("解析迁移目录 %q 失败: %w", dir, err)
	}
	info, err := os.Stat(abs)
	if err != nil {
		return "", fmt.Errorf("迁移目录不可用 %s: %w", abs, err)
	}
	if !info.IsDir() {
		return "", fmt.Errorf("迁移路径不是目录: %s", abs)
	}
	// 目录存在但没有 .sql 时 golang-migrate 会报 "no migration found"，
	// 提前给出更明确的提示
	entries, err := os.ReadDir(abs)
	if err != nil {
		return "", err
	}
	hasSQL := false
	for _, e := range entries {
		if !e.IsDir() && filepath.Ext(e.Name()) == ".sql" {
			hasSQL = true
			break
		}
	}
	if !hasSQL {
		return "", fmt.Errorf("迁移目录 %s 下没有任何 .sql 文件", abs)
	}
	return abs, nil
}

// Up 应用全部未执行的迁移。
func (r *Runner) Up() error {
	if err := r.m.Up(); err != nil && !errors.Is(err, migrate.ErrNoChange) {
		return err
	}
	return nil
}

// Down 回滚全部迁移（**会删表**）。生产环境慎用。
func (r *Runner) Down() error {
	if err := r.m.Down(); err != nil && !errors.Is(err, migrate.ErrNoChange) {
		return err
	}
	return nil
}

// Steps 按步数迁移。n 为正数向上，负数向下（与 Alembic 的 -r -1 语义一致）。
func (r *Runner) Steps(n int) error {
	if n == 0 {
		return nil
	}
	if err := r.m.Steps(n); err != nil && !errors.Is(err, migrate.ErrNoChange) {
		return err
	}
	return nil
}

// Version 返回当前版本与 dirty 状态。
//
// dirty=true 意味着某次迁移执行到一半失败了。**MySQL 的 DDL 非事务性**，
// 失败后可能留下部分表，必须先人工核对实际结构，再用 Force 标记到正确版本，
// 否则后续所有迁移操作都会被拒绝。
func (r *Runner) Version() (version uint, dirty bool, err error) {
	v, d, err := r.m.Version()
	if errors.Is(err, migrate.ErrNilVersion) {
		return 0, false, ErrNilVersion
	}
	if err != nil {
		return 0, false, err
	}
	return v, d, nil
}

// Force 把版本表强制标记为指定版本（不执行任何 SQL）。
// 用于清理 dirty 状态：人工确认实际结构等同于某个版本后调用。
func (r *Runner) Force(version int) error {
	return r.m.Force(version)
}

// Close 释放迁移器持有的 source/database 驱动资源，并关闭自建的连接。
func (r *Runner) Close() error {
	if r == nil || r.m == nil {
		return nil
	}
	// golang-migrate 同时持有 source 与 database 两个驱动，都要关
	srcErr, dbErr := r.m.Close()
	var ownErr error
	if r.ownedDB != nil {
		ownErr = r.ownedDB.Close()
		r.ownedDB = nil
	}
	return errors.Join(srcErr, dbErr, ownErr)
}
