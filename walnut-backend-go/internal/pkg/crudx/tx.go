package crudx

import (
	"context"

	"gorm.io/gorm"
)

// txKey 是事务句柄的私有 context key。
type txKey struct{}

// WithTx 把事务句柄注入 ctx。service 的多写操作在 db.Transaction 闭包内调用，
// 使闭包内所有 repo 写操作参与同一事务（对齐 Python session.begin 的请求级回滚语义）。
func WithTx(ctx context.Context, tx *gorm.DB) context.Context {
	return context.WithValue(ctx, txKey{}, tx)
}

// DB 返回 ctx 中的事务句柄；无事务时回退 fallback（基础 DB）。
// repo 方法统一以 DB(ctx, r.db) 取连接：读操作与未包裹事务的写操作行为不变，
// 被 service 事务包裹的写操作自动改用 tx，失败整体回滚。
func DB(ctx context.Context, fallback *gorm.DB) *gorm.DB {
	if tx, ok := ctx.Value(txKey{}).(*gorm.DB); ok && tx != nil {
		return tx
	}
	return fallback
}
