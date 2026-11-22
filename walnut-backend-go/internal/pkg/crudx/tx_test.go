package crudx

import (
	"context"
	"testing"

	"gorm.io/gorm"
)

func TestDBFallbackWithoutTx(t *testing.T) {
	base := &gorm.DB{}
	if got := DB(context.Background(), base); got != base {
		t.Error("无事务应回退基础 DB")
	}
}

func TestDBReturnsTxWhenPresent(t *testing.T) {
	base := &gorm.DB{}
	tx := &gorm.DB{}
	ctx := WithTx(context.Background(), tx)
	if got := DB(ctx, base); got != tx {
		t.Error("有事务应返回 tx 句柄")
	}
}

func TestWithTxNilFallsBack(t *testing.T) {
	base := &gorm.DB{}
	ctx := WithTx(context.Background(), nil)
	if got := DB(ctx, base); got != base {
		t.Error("nil tx 应回退基础 DB")
	}
}
