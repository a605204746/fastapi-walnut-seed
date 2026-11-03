// Package crudx 提供业务模块共用的 CRUD 基座，对应 Python 端 app/core/base_crud.py。
//
// 覆盖三件事：
//   - 分页请求的解析与校验（PageReq）
//   - order-by 的 SQL 注入防护与列名映射
//   - 审计字段（雪花 ID / create_by / create_dept / *_time）的自动填充
//
// # 为什么不用 GORM 的自动时间戳
//
// GORM 的 CreatedAt/UpdatedAt 钩子与另两端的写入时机不一致（Python 端是
// 应用层显式填充，且 update_time **每次更新都覆盖**、create_time 只在为空时填）。
// 故一律走本包的 FillCreate / FillUpdate。
package crudx

import (
	"context"
	"regexp"
	"strconv"
	"strings"
	"time"
	"unicode"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"walnut-seed-go/internal/pkg/authctx"
	"walnut-seed-go/internal/pkg/errs"
	"walnut-seed-go/internal/pkg/idgen"
	"walnut-seed-go/internal/pkg/modelx"
)

// DefaultUserID 是未登录时审计字段的填充值，对应 Python 端 DEFAULT_USER_ID。
const DefaultUserID int64 = -1

// ---- 查询条件（一律参数化）----

// Cond 是一条参数化的查询条件。
//
// ⚠️ SQL 里只允许出现**调用方硬编码的列名**，值一律进 Args。
// 任何来自请求的数据都不得拼进 SQL 字符串。
type Cond struct {
	SQL  string
	Args []any
}

// Like 生成 `col LIKE %value%`，对应 Python 端 Model.col.like(f"%{v}%")。
func Like(column, value string) Cond {
	return Cond{SQL: column + " LIKE ?", Args: []any{"%" + value + "%"}}
}

// Eq 生成 `col = ?`。
func Eq(column string, value any) Cond {
	return Cond{SQL: column + " = ?", Args: []any{value}}
}

// In 生成 `col IN (...)`。vals 为空时返回恒假条件——
// 空 IN 列表在 SQL 里是语法错误，而 Python 端 `col.in_([])` 会生成 `0 = 1`。
func In(column string, vals []int64) Cond {
	if len(vals) == 0 {
		return Cond{SQL: "1 = 0"}
	}
	args := make([]any, len(vals))
	for i, v := range vals {
		args[i] = v
	}
	return Cond{SQL: column + " IN ?", Args: []any{vals}}
}

// Between 生成 `col BETWEEN ? AND ?`，对应 Python 端 col.between(a, b)。
func Between(column string, from, to any) Cond {
	return Cond{SQL: column + " BETWEEN ? AND ?", Args: []any{from, to}}
}

// Apply 把条件依次作用到查询上。
func Apply(db *gorm.DB, conds ...Cond) *gorm.DB {
	for _, c := range conds {
		if c.SQL == "" {
			continue
		}
		db = db.Where(c.SQL, c.Args...)
	}
	return db
}

// ---- 分页请求 ----

// Page 分页请求，对应 Python 端 app/common/request.py 的 PageReq。
//
// 前端契约字段名为驼峰：pageNum / pageSize / orderByColumn / isAsc。
type Page struct {
	// PageNum 页码，默认 1，最小 1
	PageNum int
	// PageSize 每页数量；nil 表示「查询全部」（Python 端 default=None）
	PageSize *int
	// OrderByColumn 排序列，逗号分隔，驼峰或下划线皆可
	OrderByColumn string
	// IsAsc 排序方向，逗号分隔（asc/desc/ascending/descending）
	IsAsc string
}

// Offset 计算偏移量，对应 PageReq.offset。
//
// ⚠️ Python 端是 `(page_num - 1) * (page_size or 0)`——page_size 为 None 时
// 乘数是 0，故 offset 恒为 0。不要写成 `(page_num-1)*page_size` 然后在 nil 时崩。
func (p Page) Offset() int {
	size := 0
	if p.PageSize != nil {
		size = *p.PageSize
	}
	return (p.PageNum - 1) * size
}

// HasLimit 是否分页，对应 PageReq.has_limit()。
func (p Page) HasLimit() bool { return p.PageSize != nil }

// Querier 是 BindPage 所需的最小查询串读取能力，*gin.Context 直接满足。
//
// 抽象成接口的目的是让 BindPage 能脱离 gin 单测（校验文案是契约，必须可穷举）。
// 用 GetQuery 而非 Query：需要区分「参数不存在」（走默认值）与
// 「参数存在但为空串」（Pydantic 会报解析错误），两者的响应不同。
type Querier interface {
	Query(key string) string
	GetQuery(key string) (string, bool)
}

// BindPage 从查询串解析分页参数。
//
// 不用 gin 的 ShouldBindQuery，因为**校验失败的文案属契约**：
// Python 端由 Pydantic 产生，形如
//
//	Input should be a valid integer, unable to parse string as an integer
//	Input should be greater than or equal to 1
//
// 多个错误按字段声明顺序用 ", " 连接（对齐 validation_exception_handler）。
// gin 的绑定错误是 strconv 的原文，直接返回会与 Python 端不一致。
func BindPage(q Querier) (Page, error) {
	return bindPage(q, pageSizeRule{defaultNil: true})
}

// BindPageQuery 解析 **GET 列表接口**的分页参数，对应 PageQueryParam。
//
// ⚠️ 与 BindPage 的关键差异：PageQueryParam **覆盖**了 page_size 的定义——
//
//	page_size: int | None = Field(default=10, ge=1, le=1000, alias="pageSize")
//
// 即列表接口不传 pageSize 时默认**每页 10 条**（不是查全部），且上限 1000。
// 用错函数会让「不传分页参数」的请求在两端返回不同的行数。
func BindPageQuery(q Querier) (Page, error) {
	return bindPage(q, pageSizeRule{defaultSize: 10, maxSize: 1000})
}

// pageSizeRule 描述 page_size 字段的默认值与上限，两个绑定入口的唯一差异点。
type pageSizeRule struct {
	// defaultNil 为 true 表示缺省时不分页（查全部），对应 PageReq
	defaultNil bool
	// defaultSize 是缺省页大小，对应 PageQueryParam 的 default=10
	defaultSize int
	// maxSize 为 0 表示无上限，否则对应 le=maxSize
	maxSize int
}

func bindPage(q Querier, rule pageSizeRule) (Page, error) {
	page := Page{PageNum: 1}
	if rule.defaultNil {
		page.PageSize = nil
	} else {
		size := rule.defaultSize
		page.PageSize = &size
	}
	var msgs []string

	// page_num: int = Field(default=1, ge=1)
	// ⚠️ 存在即为「显式传参」，空串也算——Pydantic 对 "" 同样报解析错误，
	// 不能当成缺省值处理。
	if raw, ok := q.GetQuery("pageNum"); ok {
		n, err := strconv.Atoi(strings.TrimSpace(raw))
		switch {
		case err != nil:
			msgs = append(msgs, msgInvalidInt)
		case n < 1:
			msgs = append(msgs, msgGeOne)
		default:
			page.PageNum = n
		}
	}

	// page_size: int | None = Field(default=..., ge=1[, le=maxSize])
	// 同上：`?pageSize=` 在 Python 端是校验错误，不是走默认值。
	if raw, ok := q.GetQuery("pageSize"); ok {
		n, err := strconv.Atoi(strings.TrimSpace(raw))
		switch {
		case err != nil:
			msgs = append(msgs, msgInvalidInt)
		case n < 1:
			msgs = append(msgs, msgGeOne)
		case rule.maxSize > 0 && n > rule.maxSize:
			msgs = append(msgs, "Input should be less than or equal to "+strconv.Itoa(rule.maxSize))
		default:
			page.PageSize = &n
		}
	}

	page.OrderByColumn = q.Query("orderByColumn")
	page.IsAsc = q.Query("isAsc")

	if len(msgs) > 0 {
		// 与 Python 端一致：body.code=400，HTTP 传输层仍是 200
		return page, errs.BadRequest(strings.Join(msgs, ", "))
	}
	return page, nil
}

// Pydantic v2 的原文，属契约（前端直接展示）。
const (
	msgInvalidInt = "Input should be a valid integer, unable to parse string as an integer"
	msgGeOne      = "Input should be greater than or equal to 1"
)

// ---- order-by 注入防护 ----

// 允许排序列的字符集，对应 Python 端 SQL_ORDER_PATTERN = ^[a-zA-Z0-9_\ ,\.]+$
//
// ⚠️ 末尾额外允许**一个**换行：Python 的 `$` 在 re.match 下也匹配
// 「字符串末尾的换行之前」，故 "abc\n" 在 Python 端是**合法**的。
// Go 的 `$` 默认只匹配文本末尾，不加这个分支会产生一处细微的契约偏差。
var (
	orderByPattern     = regexp.MustCompile(`^[a-zA-Z0-9_\ ,\.]+$`)
	orderByPatternNL   = regexp.MustCompile(`^[a-zA-Z0-9_\ ,\.]+\n$`)
	MsgOrderByInvalid  = "参数不符合规范，不能进行查询"
	MsgOrderByMismatch = "排序参数有误"
)

// EscapeOrderBySQL 校验排序列，非法则返回 code=400 的业务异常。
// 对应 Python 端 app/utils/sql_util.py 的 escape_order_by_sql。
func EscapeOrderBySQL(orderBy string) error {
	if orderBy == "" {
		return nil
	}
	if orderByPattern.MatchString(orderBy) || orderByPatternNL.MatchString(orderBy) {
		return nil
	}
	return errs.BadRequest(MsgOrderByInvalid)
}

// BuildOrder 把 Page 的排序参数解析成 gorm 的排序子句。
//
// 两道防线：
//  1. 字符集白名单（EscapeOrderBySQL）——挡掉引号、括号、注释等注入载体
//  2. **模型 schema 列名白名单**——只有真实存在的列才会进入 ORDER BY，
//     未知列静默跳过（对齐 Python 端 `getattr(model, attr) is None → continue`）
//
// 最终交给 gorm 的是 clause.Column，由它负责加反引号，不做任何字符串拼接。
func BuildOrder(db *gorm.DB, dest any, page Page) ([]clause.OrderByColumn, error) {
	if strings.TrimSpace(page.OrderByColumn) == "" {
		return nil, nil
	}
	if err := EscapeOrderBySQL(page.OrderByColumn); err != nil {
		return nil, err
	}

	columns := Str2List(page.OrderByColumn)

	// directions 的取法必须照抄 Python：
	//   str2list(is_asc) if is_asc else ["asc"]
	// ⚠️ is_asc 非空但**全是分隔符/空白**时，str2list 得到空列表，
	// 而不是回退到 ["asc"]——随后与 columns 长度不符 → "排序参数有误"。
	// 写成「len==0 就用 asc」会吞掉这个错误分支。
	var directions []string
	if page.IsAsc != "" {
		directions = Str2List(page.IsAsc)
	} else {
		directions = []string{"asc"}
	}
	if len(directions) == 1 && len(columns) > 1 {
		repeated := make([]string, len(columns))
		for i := range repeated {
			repeated[i] = directions[0]
		}
		directions = repeated
	}
	if len(columns) != len(directions) {
		return nil, errs.BadRequest(MsgOrderByMismatch)
	}

	allowed, err := schemaColumns(db, dest)
	if err != nil {
		return nil, err
	}

	out := make([]clause.OrderByColumn, 0, len(columns))
	for i, col := range columns {
		name := ToUnderScoreCase(strings.TrimSpace(col))
		if !allowed[name] {
			continue // 模型无此列 → 跳过（Python 同款行为）
		}
		switch strings.ToLower(strings.TrimSpace(directions[i])) {
		case "ascending", "asc":
			out = append(out, clause.OrderByColumn{Column: clause.Column{Name: name}})
		case "descending", "desc":
			out = append(out, clause.OrderByColumn{Column: clause.Column{Name: name}, Desc: true})
		default:
			continue // 方向无法识别 → 不加排序（Python 的 if/elif 没有 else 分支）
		}
	}
	return out, nil
}

// schemaColumns 取出模型的数据库列名集合。
func schemaColumns(db *gorm.DB, dest any) (map[string]bool, error) {
	stmt := &gorm.Statement{DB: db}
	if err := stmt.Parse(dest); err != nil {
		return nil, errs.Wrap(err, errs.MsgDBFailed)
	}
	out := make(map[string]bool, len(stmt.Schema.Fields))
	for _, f := range stmt.Schema.Fields {
		if f.DBName != "" {
			out[f.DBName] = true
		}
	}
	return out, nil
}

// ---- 分页查询 ----

// Paginate 执行分页查询，对应 Python 端 CRUDBase.page。
//
// 返回的 rows **恒为非 nil**：无数据时是空切片而不是 null，
// 否则前端的 `rows.forEach` 会崩（Python 端列表推导的结果本就是 []）。
//
// ⚠️ 计数与取行必须各自构建一次查询。复用同一个 *gorm.DB 会让 GORM
// 把 Count 用过的 statement 条件带进后续 Find，产生难以察觉的错误 SQL。
func Paginate[T any](ctx context.Context, db *gorm.DB, page Page, conds []Cond) ([]T, int64, error) {
	var zero T
	newQuery := func() *gorm.DB {
		return Apply(db.WithContext(ctx).Model(&zero), conds...)
	}

	var total int64
	if err := newQuery().Count(&total).Error; err != nil {
		return nil, 0, err
	}

	q := newQuery()
	order, err := BuildOrder(db, &zero, page)
	if err != nil {
		return nil, 0, err
	}
	if len(order) > 0 {
		q = q.Clauses(clause.OrderBy{Columns: order})
	}
	if page.HasLimit() {
		q = q.Offset(page.Offset()).Limit(*page.PageSize)
	}

	rows := make([]T, 0)
	if err := q.Find(&rows).Error; err != nil {
		return nil, 0, err
	}
	return rows, total, nil
}

// ListAll 查询全部（不分页），对应 Python 端 CRUDBase.list_all。
// 返回值同样恒为非 nil。
func ListAll[T any](ctx context.Context, db *gorm.DB, conds ...Cond) ([]T, error) {
	var zero T
	rows := make([]T, 0)
	if err := Apply(db.WithContext(ctx).Model(&zero), conds...).Find(&rows).Error; err != nil {
		return nil, err
	}
	return rows, nil
}

// ---- 审计字段填充 ----

// FillCreate 填充新增记录的审计字段，对应 Python 端 CRUDBase._fill_create。
//
// 语义逐条对齐（**只在字段为空时填**，update_time 例外）：
//   - id：为 0 时生成雪花 ID（本项目不用数据库自增）
//   - create_by / update_by：为 nil 时填当前用户 ID，未登录填 -1
//   - create_dept：为 nil 时填当前用户部门（可为 nil）
//   - create_time：为 nil 时填 now
//   - update_time：**无条件**填 now
func FillCreate(bm *modelx.BaseModel, auth authctx.Context, now time.Time) {
	if bm.ID == 0 {
		bm.ID = idgen.NextID()
	}
	uid := currentUserID(auth)
	if bm.CreateBy == nil {
		bm.CreateBy = &uid
	}
	if bm.UpdateBy == nil {
		bm.UpdateBy = &uid
	}
	if bm.CreateDept == nil {
		bm.CreateDept = auth.User.DeptID
	}
	if bm.CreateTime == nil {
		t := now
		bm.CreateTime = &t
	}
	t := now
	bm.UpdateTime = &t
}

// FillUpdate 填充更新记录的审计字段，对应 Python 端 CRUDBase._fill_update。
//
// 两个字段都是**无条件覆盖**（与 FillCreate 的「仅空时填」不同）。
func FillUpdate(bm *modelx.BaseModel, auth authctx.Context, now time.Time) {
	uid := currentUserID(auth)
	bm.UpdateBy = &uid
	t := now
	bm.UpdateTime = &t
}

// currentUserID 对应 Python 端 _current_user_id：
// `getattr(user,'id',None) or DEFAULT_USER_ID`——注意是 `or`，
// 故 ID 为 0（Go 的零值）时也落到 -1，而不是留下 0。
func currentUserID(auth authctx.Context) int64 {
	if auth.User.ID == 0 {
		return DefaultUserID
	}
	return auth.User.ID
}

// ---- 字符串工具 ----

// Separator 是 ID 串/排序列的默认分隔符，对应 Python 端 string_util.SEPARATOR。
const Separator = ","

// IsBlank 对应 Python 端 string_util.is_blank（None 或去空白后为空）。
func IsBlank(s string) bool { return strings.TrimSpace(s) == "" }

// IsNotBlank 对应 string_util.is_not_blank。
func IsNotBlank(s string) bool { return !IsBlank(s) }

// Str2List 按分隔符切分并去空白、过滤空项，对应 string_util.str2list。
//
// ⚠️ 整体为空白时返回**空切片**而不是 nil，且不做「回退默认值」——
// 调用方依赖空/非空来区分参数缺失与参数非法（见 BuildOrder 的注释）。
func Str2List(value string) []string {
	if IsBlank(value) {
		return []string{}
	}
	parts := strings.Split(value, Separator)
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		item := strings.TrimSpace(p)
		if item == "" {
			continue
		}
		out = append(out, item)
	}
	return out
}

// ToUnderScoreCase 驼峰转下划线，对应 common_util.to_under_score_case。
//
// 逐字符规则（必须照抄，别用正则「优化」）：
//   - 遇到大写字母：若**不是首字符**且**前一个字符不是下划线**，先补一个下划线
//   - 然后追加该字母的小写形式
//   - 其余字符原样追加
//
// 例：createTime → create_time；ABC → a_b_c；create_Time → create_time。
func ToUnderScoreCase(camel string) string {
	if camel == "" {
		return camel
	}
	runes := []rune(camel)
	var b strings.Builder
	b.Grow(len(runes) + 4)
	for i, ch := range runes {
		if unicode.IsUpper(ch) {
			if i > 0 && runes[i-1] != '_' {
				b.WriteRune('_')
			}
			b.WriteRune(unicode.ToLower(ch))
		} else {
			b.WriteRune(ch)
		}
	}
	return b.String()
}

// ParseIDs 解析路径里的逗号分隔 ID 串，对应各 controller 里的 _parse_ids。
//
// 两条文案都是契约（前端直接展示）：
//   - 含非数字 → "{label}格式有误"，code=400
//   - 解析后为空 → "{label}不能为空"，code=400
func ParseIDs(raw, label string) ([]int64, error) {
	items := Str2List(raw)
	ids := make([]int64, 0, len(items))
	for _, item := range items {
		n, err := strconv.ParseInt(item, 10, 64)
		if err != nil {
			return nil, errs.BadRequest(label + "格式有误")
		}
		ids = append(ids, n)
	}
	if len(ids) == 0 {
		return nil, errs.BadRequest(label + "不能为空")
	}
	return ids, nil
}
