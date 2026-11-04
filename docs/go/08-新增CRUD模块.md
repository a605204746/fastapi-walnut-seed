# 新增 CRUD 模块

> 更新日期：2026-11-04 · 适用版本：WalnutSeed v1.0

本文以新增一个「博客管理 → 文章管理」模块为例，走完 Go 后端的标准开发流程。参考原型：`internal/module/system/post`（岗位管理）。

## 0. 前置

```bash
docker compose -f docker/docker-compose.middleware.yml up -d
cd walnut-backend-go && go run . run --env dev
```

## 1. 创建模块目录

```
walnut-backend-go/internal/module/blog/article/
├── model.go      # GORM 实体
├── dto.go        # 入参/出参
├── repo.go       # 数据访问
├── service.go    # 业务逻辑
└── handler.go    # HTTP 端点
```

## 2. model.go

```go
package article

import "walnut-seed-go/internal/pkg/modelx"

type Article struct {
    modelx.BaseModel
    Title   string  `gorm:"column:title;not null"`
    Content *string `gorm:"column:content"`
    Status  string  `gorm:"column:status;not null"` // "0" 正常 / "1" 停用
    DelFlag string  `gorm:"column:del_flag;not null"`
}

func (Article) TableName() string { return "blog_article" }
```

要点：
- `modelx.BaseModel` 提供雪花主键 + 审计字段
- 可空列用指针
- NOT NULL 列用值类型
- 显式 `TableName()`

## 3. dto.go

```go
package article

type Out struct {
    ID         *idtype.JSInt   `json:"id"`
    Title      *string         `json:"title"`
    Content    *string         `json:"content"`
    Status     *string         `json:"status"`
    CreateTime *timex.JSONTime `json:"createTime"`
}

type QueryParam struct {
    Page   crudx.Page
    Title  string
    Status string
}

func BindQueryParam(q crudx.Querier) (QueryParam, error) {
    page, err := crudx.BindPageQuery(q)
    // ...
}

type CreateIn struct {
    Title   *string
    Content *string
    Status  *string
}

func DecodeCreate(body []byte) (CreateIn, error) {
    // Pydantic 风格的逐字段解码
}
```

要点：
- 出参 camelCase，指针 + 无 omitempty（null 恒定出现）
- 整数字段走 `idtype.JSInt`（条件序列化）
- 入参解码复刻 Pydantic 校验消息（`Field required` 等原文）

## 4. repo.go

```go
package article

type Repo struct{ db *gorm.DB }

func NewRepo(db *gorm.DB) *Repo { return &Repo{db: db} }

func (r *Repo) Page(ctx context.Context, page crudx.Page, conds []crudx.Cond) ([]Article, int64, error) {
    newQuery := func() *gorm.DB {
        q := r.db.WithContext(ctx).Model(&Article{}).Where("del_flag = ?", modelx.Normal)
        return crudx.Apply(q, conds...)
    }
    var total int64
    newQuery().Count(&total)
    q := newQuery().Order("id DESC")
    if page.HasLimit() { q = q.Offset(page.Offset()).Limit(*page.PageSize) }
    rows := make([]Article, 0)
    q.Find(&rows)
    return rows, total, nil
}
```

要点：
- count 和 fetch 各建一个 query（GORM 语句复用陷阱）
- `crudx.Apply` 追加条件
- `HasLimit()` 控制分页

## 5. service.go

```go
package article

type Service struct {
    repo *Repo
    auth authctx.Context
}

func NewService(db *gorm.DB, ac authctx.Context) *Service {
    return &Service{repo: NewRepo(db), auth: ac}
}

func (s *Service) PageList(ctx context.Context, q QueryParam) (*PageOut, error) {
    conds := buildConditions(q)
    rows, total, err := s.repo.Page(ctx, q.Page, conds)
    // toOuts + fillDeptName...
}

func (s *Service) Insert(ctx context.Context, in CreateIn) (bool, error) {
    row := Article{Status: modelx.Normal, DelFlag: modelx.Normal}
    applyCreateFields(&row, in)
    crudx.FillCreate(&row.BaseModel, s.auth, nowFunc())
    return s.repo.Create(ctx, &row) == nil, nil
}
```

要点：
- Service 持有请求级 `authctx.Context`
- `crudx.FillCreate` 填充审计字段
- 删除前校验关联数据

## 6. handler.go

```go
package article

type Handler struct {
    db   *gorm.DB
    errs *errs.Handler
    perm *permguard.Guard
    idem *idempotentx.Guard
}

func (h *Handler) Mount(b routex.Binder) {
    require, repeat := h.guards()
    b.GET("/list", require("blog:article:list"), h.List)
    b.POST("", require("blog:article:add"), repeat(), h.Add)
    b.PUT("", require("blog:article:edit"), repeat(), h.Update)
    b.DELETE("/:ids", require("blog:article:remove"), h.Delete)
}
```

## 7. 挂载路由

`internal/bootstrap/router.go`：

```go
import "walnut-seed-go/internal/module/blog/article"

article.NewHandler(a.db, a.errHandler, perm, idem).Mount(system.Group("/article").Binder())
```

## 8. 验证

```bash
go build ./...
go vet ./...
go test ./test/contract/ -run TestNoInventedRoutes  # 不应有 Python 没有的路由
```

## 9. 检查清单

- [ ] model.go 有显式 `TableName()`
- [ ] dto.go 出参字段顺序与 Python Pydantic 声明序一致
- [ ] dto.go 入参解码复刻 Pydantic 校验消息
- [ ] repo.go count 和 fetch 各建 query
- [ ] service.go 使用 `crudx.FillCreate` / `FillUpdate`
- [ ] handler.go 每个端点都挂了 `permguard.Require`
- [ ] 写操作挂了 `repeat()`（防重提交）
- [ ] router.go 挂载在 `b.Protected` 分组下
