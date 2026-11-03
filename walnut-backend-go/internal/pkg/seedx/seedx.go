// Package seedx 负责写入基线种子数据，对应 Python 端 app/seed/initialize.py。
//
// 幂等性有**双重保证**：
//  1. seed/mysql_data.sql 每条语句都是 INSERT IGNORE（主键冲突则跳过）
//  2. 本包执行前先 SELECT COUNT(*) FROM sys_user，非空则整体跳过
//     —— 与 Python 端 InitializeData.init_db 的守卫完全一致
//
// 容器重启不能产生重复数据，因此这两道守卫都不能省。
package seedx

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"strings"

	"gorm.io/gorm"
)

// 守卫用的探测表：与 Python 端一致选 sys_user。
// 它是登录必需表，一旦有数据就说明基线已写入过。
const guardTable = "sys_user"

// Result 种子写入结果。
type Result struct {
	// Skipped 为 true 表示库里已有数据，整体跳过
	Skipped bool
	// Executed 实际执行的 INSERT 条数
	Executed int
	// Existing 跳过时探测到的既有行数
	Existing int64
}

// Run 执行种子数据写入。
//
// 全流程包在一个事务里：任一条失败即整体回滚，避免留下「部门写了一半、
// 菜单没写」这种半初始化状态——那比完全不初始化更难排查。
func Run(ctx context.Context, db *gorm.DB, sqlPath string) (*Result, error) {
	if db == nil {
		return nil, fmt.Errorf("数据库未初始化，无法写入种子数据")
	}

	// ---- 守卫：已有数据则跳过 ----
	var count int64
	if err := db.WithContext(ctx).
		Table(guardTable).
		Count(&count).Error; err != nil {
		return nil, fmt.Errorf("种子前置检查失败（%s 表可能未创建，请先执行 migrate up）: %w", guardTable, err)
	}
	if count > 0 {
		return &Result{Skipped: true, Existing: count}, nil
	}

	// ---- 解析 INSERT 语句 ----
	stmts, err := parseInserts(sqlPath)
	if err != nil {
		return nil, err
	}
	if len(stmts) == 0 {
		return nil, fmt.Errorf("种子文件 %s 中未找到任何 INSERT 语句", sqlPath)
	}

	// ---- 事务内逐条执行 ----
	executed := 0
	err = db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		for i, stmt := range stmts {
			// 不传 args：go-sql-driver 在无参数时走文本协议（COM_QUERY），
			// 数据里若含 '?' 或 ':' 都不会被误当作占位符。
			// ⚠️ 一旦改成 Exec(stmt, args...) 就会切到预处理协议，
			//    种子数据里的 '?' 会导致 "sql: expected 0 arguments, got N"。
			if err := tx.Exec(stmt).Error; err != nil {
				return fmt.Errorf("第 %d 条 INSERT 执行失败: %w\n语句前 120 字符: %s",
					i+1, err, truncate(stmt, 120))
			}
			executed++
		}
		return nil
	})
	if err != nil {
		return nil, err
	}

	return &Result{Executed: executed}, nil
}

// parseInserts 逐行读取并挑出 INSERT 语句。
//
// 与 Python 端同样的约定：**每条 INSERT 独占一行**（种子文件由脚本生成，格式受控）。
// 这样无需引入完整 SQL 解析器，也避免了跨行语句的拼接歧义。
func parseInserts(path string) ([]string, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("打开种子文件失败: %w", err)
	}
	defer f.Close()

	var out []string
	sc := bufio.NewScanner(f)
	// 菜单表的 INSERT 可能很长（含路由元数据 JSON），放大缓冲上限
	sc.Buffer(make([]byte, 0, 256*1024), 16*1024*1024)
	lineNo := 0
	for sc.Scan() {
		lineNo++
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "--") {
			continue
		}
		if !strings.HasPrefix(strings.ToUpper(line), "INSERT") {
			continue
		}
		out = append(out, strings.TrimSuffix(line, ";"))
	}
	if err := sc.Err(); err != nil {
		return nil, fmt.Errorf("读取种子文件失败（第 %d 行附近）: %w", lineNo, err)
	}
	return out, nil
}

func truncate(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n]) + "..."
}
