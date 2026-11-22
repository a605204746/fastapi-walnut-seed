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
├── routes.go           # 路由声明（Mount + 权限/防重守卫）
├── handler.go          # HTTP 端点处理（bind → 调 service → envelope）
├── service.go          # 业务逻辑 + consumer-side repo 接口
├── service_test.go     # mock repo 单测
├── req.go              # 入参/查询绑定（解码辅助走 bindx）
├── resp.go             # 出参形状与映射
├── dto_decode_test.go  # 特征测试（钉住契约文案与错误序）
├── model.go            # GORM 实体
└── repo.go             # 数据访问
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

## 3. req.go / resp.go

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
- 出参放 resp.go、入参/查询放 req.go；出参 camelCase，指针 + 无 omitempty（null 恒定出现）
- 整数字段走 `idtype.JSInt`（条件序列化）
- **JSON body 入参用 tag 范式**：请求结构体 `validate` + `label` tag，`Decode*(ctx, body)` 内 `bindx.BindJSON` 绑定后映射到 service 消费的 In 结构；自定义规则 `bindx.RegisterTag`。活样板：`internal/module/system/client`
- query/form 绑定仍走 crudx/bindx 查询 cluster（BindQueryParam 形态）
- **特征测试先行**：dto_decode_test.go 钉住 Decode* 输出（结构语义冻结，文案按 i18n 策略）

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
- 跨模块模型引用直接 import 模块根（如 dept.SysDept）；包名冲突用别名（如 sysconfig）

## 5. service.go

```go
package article

// articleRepo consumer-side 接口：只列 service 实际调用的方法
type articleRepo interface {
    Page(ctx context.Context, page crudx.Page, conds []crudx.Cond) ([]Article, int64, error)
    Create(ctx context.Context, row *Article) error
}

type Service struct {
    repo articleRepo
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
- repo 依赖是 consumer-side 接口；mock 单测用「嵌入接口 + 覆盖被测方法」模式（见 user/service_test.go）
- 跨模块能力经 Provider 工厂注入（见 user.Deps 与 bootstrap 接线），**禁止直接 NewService 其他模块**
- `crudx.FillCreate` 填充审计字段
- 删除前校验关联数据

## 6. handler.go 与 routes.go

路由声明与请求处理分两个文件：routes.go 放 Mount 与守卫工厂，handler.go 只放端点处理函数。

```go
// routes.go
package article

func (h *Handler) Mount(b routex.Binder) {
    require, repeat := h.guards()
    b.GET("/list", require("blog:article:list"), h.List)
    b.POST("", require("blog:article:add"), repeat(), h.Add)
    b.PUT("", require("blog:article:edit"), repeat(), h.Update)
    b.DELETE("/:ids", require("blog:article:remove"), h.Delete)
}
```

```go
// handler.go
package article

type Handler struct {
    db   *gorm.DB
    errs *errs.Handler
    perm *permguard.Guard
    idem *idempotentx.Guard
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
go run . routes                                     # 重新生成全局路由清单
```

## 9. 检查清单

- [ ] model.go 有显式 `TableName()`
- [ ] resp.go 出参字段顺序与 Python Pydantic 声明序一致
- [ ] req.go 入参解码复刻 Pydantic 校验消息（bindx 变体选择正确）
- [ ] dto_decode_test.go 特征测试钉住 Decode* 行为
- [ ] repo.go count 和 fetch 各建 query
- [ ] service.go 定义 consumer-side repo 接口 + mock 单测；使用 `crudx.FillCreate` / `FillUpdate`
- [ ] 跨模块能力经 Provider 注入，无直接 NewService 其他模块
- [ ] routes.go 每个端点都挂了 `permguard.Require`
- [ ] 写操作挂了 `repeat()`（防重提交）
- [ ] router.go 挂载在 `b.Protected` 分组下
