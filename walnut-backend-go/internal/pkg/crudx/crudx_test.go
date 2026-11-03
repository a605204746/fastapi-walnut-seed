package crudx

import (
	"net/url"
	"strings"
	"testing"
	"time"

	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	"walnut-seed-go/internal/pkg/authctx"
	"walnut-seed-go/internal/pkg/errs"
	"walnut-seed-go/internal/pkg/modelx"
)

// tPost 是测试用的模型，字段覆盖排序白名单与审计填充两条路径。
type tPost struct {
	modelx.BaseModel
	PostCode *string `gorm:"column:post_code"`
	PostSort *int    `gorm:"column:post_sort"`
	DeptID   *int64  `gorm:"column:dept_id"`
	Status   *string `gorm:"column:status"`
}

func (tPost) TableName() string { return "sys_post" }

func sp(s string) *string { return &s }
func ip(i int64) *int64   { return &i }

// dryDB 构造不连库、只生成 SQL 的 *gorm.DB（两个开关缺一不可，见 permx 的同名注释）。
func dryDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(mysql.New(mysql.Config{
		SkipInitializeWithVersion: true,
		DSN:                       "u:p@tcp(127.0.0.1:1)/d",
	}), &gorm.Config{DryRun: true, DisableAutomaticPing: true, Logger: logger.Discard})
	if err != nil {
		t.Fatalf("构造 DryRun DB 失败: %v", err)
	}
	return db
}

// fakeQuery 用真实 URL 解析结果实现 Querier，避免手写 map 与 gin 语义不符。
type fakeQuery struct{ v url.Values }

func newQuery(t *testing.T, raw string) fakeQuery {
	t.Helper()
	parsed, err := url.ParseQuery(raw)
	if err != nil {
		t.Fatalf("解析查询串 %q 失败: %v", raw, err)
	}
	return fakeQuery{v: parsed}
}

func (f fakeQuery) Query(key string) string { return f.v.Get(key) }

func (f fakeQuery) GetQuery(key string) (string, bool) {
	vals, ok := f.v[key]
	if !ok || len(vals) == 0 {
		return "", false
	}
	return vals[0], true
}

// ---- ToUnderScoreCase ----

func TestToUnderScoreCase(t *testing.T) {
	cases := map[string]string{
		"":              "",
		"id":            "id",
		"createTime":    "create_time",
		"postSort":      "post_sort",
		"ABC":           "a_b_c",       // 连续大写逐个加下划线
		"A":             "a",           // 首字符大写不补下划线
		"create_Time":   "create_time", // 前一个字符已是下划线则不重复补
		"already_snake": "already_snake",
		"_leading":      "_leading",
		"trailing_":     "trailing_",
		"deptId":        "dept_id",
		"a1B2":          "a1_b2", // 数字不触发下划线
	}
	for in, want := range cases {
		if got := ToUnderScoreCase(in); got != want {
			t.Errorf("ToUnderScoreCase(%q) = %q, want %q", in, got, want)
		}
	}
}

// ---- Str2List / IsBlank ----

func TestStr2List(t *testing.T) {
	cases := []struct {
		in   string
		want []string
	}{
		{"", []string{}},
		{"   ", []string{}},
		{",,,", []string{}}, // 全部分隔符 → 空列表（**不是**回退默认值）
		{"a,b", []string{"a", "b"}},
		{"a, ,b", []string{"a", "b"}},
		{" a , b ", []string{"a", "b"}},
		{"createTime,postSort", []string{"createTime", "postSort"}},
	}
	for _, c := range cases {
		got := Str2List(c.in)
		if got == nil {
			t.Errorf("Str2List(%q) 返回 nil，应返回空切片（nil 会序列化成 null）", c.in)
			continue
		}
		if strings.Join(got, "|") != strings.Join(c.want, "|") {
			t.Errorf("Str2List(%q) = %v, want %v", c.in, got, c.want)
		}
	}
}

func TestIsBlank(t *testing.T) {
	for _, s := range []string{"", " ", "\t", "\n", " \t\n "} {
		if !IsBlank(s) || IsNotBlank(s) {
			t.Errorf("IsBlank(%q) 应为 true", s)
		}
	}
	for _, s := range []string{"a", " a ", "0"} {
		if IsBlank(s) || !IsNotBlank(s) {
			t.Errorf("IsBlank(%q) 应为 false", s)
		}
	}
}

// ---- EscapeOrderBySQL：注入防线 ----

func TestEscapeOrderBySQL(t *testing.T) {
	allowed := []string{
		"", "createTime", "post_sort", "a.b", "a b", "a,b", "a1", "A",
		"create_time desc", "a,b desc",
		// Python 的 $ 在 re.match 下也匹配「末尾换行之前」，故这是合法的
		"createTime\n",
	}
	for _, s := range allowed {
		if err := EscapeOrderBySQL(s); err != nil {
			t.Errorf("EscapeOrderBySQL(%q) 不应报错，实际: %v", s, err)
		}
	}

	blocked := []string{
		"id; DROP TABLE sys_post",
		"id'", `"id`, "id`",
		"1=1", "id--", "id/*x*/",
		"(id)", "id)", "count(*)",
		"createTime\n\n", // 两个换行：Python 的 $ 只允许一个
		"id\t",
		"中文字段",
	}
	for _, s := range blocked {
		err := EscapeOrderBySQL(s)
		if err == nil {
			t.Errorf("EscapeOrderBySQL(%q) 应拒绝（注入载体），实际放行了", s)
			continue
		}
		se := errs.As(err)
		if se == nil {
			t.Errorf("EscapeOrderBySQL(%q) 应返回 *errs.ServiceError", s)
			continue
		}
		if se.Code != errs.CodeBadRequest {
			t.Errorf("code got=%d want=%d", se.Code, errs.CodeBadRequest)
		}
		if se.Msg != MsgOrderByInvalid {
			t.Errorf("msg got=%q want=%q", se.Msg, MsgOrderByInvalid)
		}
	}
}

// ---- BuildOrder ----

func TestBuildOrder(t *testing.T) {
	db := dryDB(t)

	t.Run("空排序列不产生子句", func(t *testing.T) {
		got, err := BuildOrder(db, &tPost{}, Page{PageNum: 1, OrderByColumn: "  "})
		if err != nil || got != nil {
			t.Fatalf("got=%v err=%v", got, err)
		}
	})

	t.Run("驼峰列名转下划线并默认asc", func(t *testing.T) {
		got, err := BuildOrder(db, &tPost{}, Page{PageNum: 1, OrderByColumn: "createTime"})
		if err != nil {
			t.Fatal(err)
		}
		if len(got) != 1 || got[0].Column.Name != "create_time" || got[0].Desc {
			t.Fatalf("got=%+v", got)
		}
	})

	t.Run("单方向自动复制到多列", func(t *testing.T) {
		got, err := BuildOrder(db, &tPost{}, Page{PageNum: 1, OrderByColumn: "createTime,postSort", IsAsc: "desc"})
		if err != nil {
			t.Fatal(err)
		}
		if len(got) != 2 || !got[0].Desc || !got[1].Desc {
			t.Fatalf("got=%+v", got)
		}
		if got[0].Column.Name != "create_time" || got[1].Column.Name != "post_sort" {
			t.Fatalf("列名映射错误: %+v", got)
		}
	})

	t.Run("多方向逐列对应", func(t *testing.T) {
		got, err := BuildOrder(db, &tPost{}, Page{PageNum: 1, OrderByColumn: "createTime,postSort", IsAsc: "desc,asc"})
		if err != nil {
			t.Fatal(err)
		}
		if len(got) != 2 || !got[0].Desc || got[1].Desc {
			t.Fatalf("got=%+v", got)
		}
	})

	t.Run("ascending/descending全称等价", func(t *testing.T) {
		got, err := BuildOrder(db, &tPost{}, Page{PageNum: 1, OrderByColumn: "createTime,postSort", IsAsc: "DESCENDING,Ascending"})
		if err != nil {
			t.Fatal(err)
		}
		if len(got) != 2 || !got[0].Desc || got[1].Desc {
			t.Fatalf("大小写应不敏感: %+v", got)
		}
	})

	// ⚠️ 这是最容易写错的分支：is_asc 非空但全是分隔符时，
	// str2list 得到**空列表**而不是回退 ["asc"]，随后长度不符要报错。
	t.Run("isAsc全为分隔符时报排序参数有误", func(t *testing.T) {
		_, err := BuildOrder(db, &tPost{}, Page{PageNum: 1, OrderByColumn: "createTime", IsAsc: ","})
		if err == nil {
			t.Fatal("应报错：方向列表为空与列数不符")
		}
		se := errs.As(err)
		if se == nil || se.Msg != MsgOrderByMismatch || se.Code != errs.CodeBadRequest {
			t.Fatalf("期望 code=400 msg=%q，实际 %v", MsgOrderByMismatch, err)
		}
	})

	t.Run("列数与方向数不符时报错", func(t *testing.T) {
		_, err := BuildOrder(db, &tPost{}, Page{PageNum: 1, OrderByColumn: "createTime,postSort", IsAsc: "asc,desc,asc"})
		se := errs.As(err)
		if se == nil || se.Msg != MsgOrderByMismatch {
			t.Fatalf("期望 %q，实际 %v", MsgOrderByMismatch, err)
		}
	})

	t.Run("模型不存在的列被静默跳过", func(t *testing.T) {
		got, err := BuildOrder(db, &tPost{}, Page{PageNum: 1, OrderByColumn: "nonExistentColumn"})
		if err != nil {
			t.Fatal(err)
		}
		if len(got) != 0 {
			t.Fatalf("未知列应被跳过，实际 %+v", got)
		}
	})

	t.Run("无法识别的方向被跳过而非报错", func(t *testing.T) {
		got, err := BuildOrder(db, &tPost{}, Page{PageNum: 1, OrderByColumn: "createTime,postSort", IsAsc: "sideways,asc"})
		if err != nil {
			t.Fatal(err)
		}
		if len(got) != 1 || got[0].Column.Name != "post_sort" {
			t.Fatalf("got=%+v", got)
		}
	})

	t.Run("注入载体在字符集校验阶段就被拦下", func(t *testing.T) {
		_, err := BuildOrder(db, &tPost{}, Page{PageNum: 1, OrderByColumn: "id; DROP TABLE sys_post"})
		se := errs.As(err)
		if se == nil || se.Msg != MsgOrderByInvalid {
			t.Fatalf("期望 %q，实际 %v", MsgOrderByInvalid, err)
		}
	})
}

// ---- BindPage：校验文案是契约 ----

func TestBindPage(t *testing.T) {
	t.Run("无参数走默认值", func(t *testing.T) {
		p, err := BindPage(newQuery(t, ""))
		if err != nil {
			t.Fatal(err)
		}
		if p.PageNum != 1 || p.PageSize != nil || p.HasLimit() {
			t.Fatalf("got=%+v", p)
		}
		if p.Offset() != 0 {
			t.Errorf("PageSize 为 nil 时 offset 必须为 0，got=%d", p.Offset())
		}
	})

	t.Run("正常分页", func(t *testing.T) {
		p, err := BindPage(newQuery(t, "pageNum=3&pageSize=10"))
		if err != nil {
			t.Fatal(err)
		}
		if p.PageNum != 3 || p.PageSize == nil || *p.PageSize != 10 {
			t.Fatalf("got=%+v", p)
		}
		if p.Offset() != 20 {
			t.Errorf("offset got=%d want=20", p.Offset())
		}
	})

	t.Run("pageNum为0报ge校验", func(t *testing.T) {
		_, err := BindPage(newQuery(t, "pageNum=0"))
		assertMsg(t, err, msgGeOne)
	})

	t.Run("非整数报解析错误", func(t *testing.T) {
		_, err := BindPage(newQuery(t, "pageNum=abc"))
		assertMsg(t, err, msgInvalidInt)
	})

	// ⚠️ `?pageSize=` 在 Python 端是校验错误，不是「查全部」
	t.Run("pageSize为空串报解析错误", func(t *testing.T) {
		_, err := BindPage(newQuery(t, "pageSize="))
		assertMsg(t, err, msgInvalidInt)
	})

	t.Run("pageNum为空串同样报错", func(t *testing.T) {
		_, err := BindPage(newQuery(t, "pageNum="))
		assertMsg(t, err, msgInvalidInt)
	})

	t.Run("多个错误按字段顺序用逗号空格连接", func(t *testing.T) {
		_, err := BindPage(newQuery(t, "pageNum=0&pageSize=abc"))
		assertMsg(t, err, msgGeOne+", "+msgInvalidInt)
	})

	t.Run("负数报ge校验", func(t *testing.T) {
		_, err := BindPage(newQuery(t, "pageSize=-5"))
		assertMsg(t, err, msgGeOne)
	})

	t.Run("排序参数原样透传", func(t *testing.T) {
		p, err := BindPage(newQuery(t, "orderByColumn=createTime&isAsc=desc"))
		if err != nil {
			t.Fatal(err)
		}
		if p.OrderByColumn != "createTime" || p.IsAsc != "desc" {
			t.Fatalf("got=%+v", p)
		}
	})
}

func assertMsg(t *testing.T, err error, want string) {
	t.Helper()
	se := errs.As(err)
	if se == nil {
		t.Fatalf("期望 *errs.ServiceError(msg=%q)，实际 %v", want, err)
	}
	if se.Msg != want {
		t.Errorf("msg got=%q want=%q", se.Msg, want)
	}
	if se.Code != errs.CodeBadRequest {
		t.Errorf("code got=%d want=400", se.Code)
	}
}

// ---- ParseIDs ----

func TestParseIDs(t *testing.T) {
	t.Run("正常解析", func(t *testing.T) {
		got, err := ParseIDs("1,2,3", "岗位ID")
		if err != nil {
			t.Fatal(err)
		}
		if len(got) != 3 || got[0] != 1 || got[2] != 3 {
			t.Fatalf("got=%v", got)
		}
	})

	t.Run("含空白项被过滤", func(t *testing.T) {
		got, err := ParseIDs(" 1 , ,2 ", "岗位ID")
		if err != nil || len(got) != 2 {
			t.Fatalf("got=%v err=%v", got, err)
		}
	})

	t.Run("非数字报格式有误", func(t *testing.T) {
		_, err := ParseIDs("1,abc", "岗位ID")
		assertMsg(t, err, "岗位ID格式有误")
	})

	t.Run("空串报不能为空", func(t *testing.T) {
		_, err := ParseIDs("", "岗位ID")
		assertMsg(t, err, "岗位ID不能为空")
	})

	t.Run("全是分隔符报不能为空", func(t *testing.T) {
		_, err := ParseIDs(",,,", "岗位ID")
		assertMsg(t, err, "岗位ID不能为空")
	})
}

// ---- 审计填充 ----

func authCtx(userID int64, deptID *int64) authctx.Context {
	return authctx.Context{User: authctx.User{ID: userID, DeptID: deptID}}
}

func TestFillCreate(t *testing.T) {
	now := time.Date(2026, 11, 3, 10, 0, 0, 0, time.Local)

	t.Run("全部字段为空时按当前用户填充", func(t *testing.T) {
		var bm modelx.BaseModel
		FillCreate(&bm, authCtx(7, ip(100)), now)
		if bm.ID == 0 {
			t.Error("应生成雪花 ID")
		}
		if bm.CreateBy == nil || *bm.CreateBy != 7 {
			t.Errorf("CreateBy got=%v want=7", bm.CreateBy)
		}
		if bm.UpdateBy == nil || *bm.UpdateBy != 7 {
			t.Errorf("UpdateBy got=%v want=7", bm.UpdateBy)
		}
		if bm.CreateDept == nil || *bm.CreateDept != 100 {
			t.Errorf("CreateDept got=%v want=100", bm.CreateDept)
		}
		if bm.CreateTime == nil || !bm.CreateTime.Equal(now) {
			t.Errorf("CreateTime got=%v want=%v", bm.CreateTime, now)
		}
		if bm.UpdateTime == nil || !bm.UpdateTime.Equal(now) {
			t.Errorf("UpdateTime got=%v want=%v", bm.UpdateTime, now)
		}
	})

	t.Run("已有值不被覆盖，但UpdateTime总是覆盖", func(t *testing.T) {
		old := now.Add(-48 * time.Hour)
		bm := modelx.BaseModel{
			ID:         999,
			CreateBy:   ip(3),
			UpdateBy:   ip(3),
			CreateDept: ip(50),
			CreateTime: &old,
			UpdateTime: &old,
		}
		FillCreate(&bm, authCtx(7, ip(100)), now)
		if bm.ID != 999 {
			t.Errorf("已有 ID 不应被覆盖，got=%d", bm.ID)
		}
		if *bm.CreateBy != 3 || *bm.UpdateBy != 3 || *bm.CreateDept != 50 {
			t.Error("已有审计字段不应被覆盖")
		}
		if !bm.CreateTime.Equal(old) {
			t.Error("已有 CreateTime 不应被覆盖")
		}
		if !bm.UpdateTime.Equal(now) {
			t.Errorf("UpdateTime 必须无条件覆盖，got=%v want=%v", bm.UpdateTime, now)
		}
	})

	// Python 端是 `user.id or DEFAULT_USER_ID`，故 ID 为 0 时落到 -1
	t.Run("未登录用户ID填-1", func(t *testing.T) {
		var bm modelx.BaseModel
		FillCreate(&bm, authCtx(0, nil), now)
		if bm.CreateBy == nil || *bm.CreateBy != DefaultUserID {
			t.Errorf("CreateBy got=%v want=%d", bm.CreateBy, DefaultUserID)
		}
		if bm.CreateDept != nil {
			t.Errorf("用户无部门时 CreateDept 应为 nil，got=%v", *bm.CreateDept)
		}
	})

	t.Run("两次调用生成不同雪花ID", func(t *testing.T) {
		var a, b modelx.BaseModel
		FillCreate(&a, authCtx(7, nil), now)
		FillCreate(&b, authCtx(7, nil), now)
		if a.ID == b.ID {
			t.Errorf("雪花 ID 重复: %d", a.ID)
		}
	})
}

func TestFillUpdate(t *testing.T) {
	now := time.Date(2026, 11, 3, 10, 0, 0, 0, time.Local)
	old := now.Add(-48 * time.Hour)

	t.Run("无条件覆盖update_by与update_time", func(t *testing.T) {
		bm := modelx.BaseModel{ID: 5, UpdateBy: ip(3), UpdateTime: &old, CreateBy: ip(3), CreateTime: &old}
		FillUpdate(&bm, authCtx(7, ip(100)), now)
		if *bm.UpdateBy != 7 {
			t.Errorf("UpdateBy got=%d want=7", *bm.UpdateBy)
		}
		if !bm.UpdateTime.Equal(now) {
			t.Error("UpdateTime 应被覆盖为 now")
		}
		// 更新不得触碰创建审计字段
		if *bm.CreateBy != 3 || !bm.CreateTime.Equal(old) {
			t.Error("FillUpdate 不应改动 create_by / create_time")
		}
	})

	t.Run("未登录时update_by为-1", func(t *testing.T) {
		var bm modelx.BaseModel
		FillUpdate(&bm, authCtx(0, nil), now)
		if *bm.UpdateBy != DefaultUserID {
			t.Errorf("UpdateBy got=%d want=%d", *bm.UpdateBy, DefaultUserID)
		}
	})
}

// ---- Cond ----

func TestInEmptyIsAlwaysFalse(t *testing.T) {
	c := In("id", nil)
	if c.SQL != "1 = 0" || len(c.Args) != 0 {
		t.Fatalf("空 IN 列表必须退化为恒假条件，got=%+v", c)
	}
}

func TestApplySkipsEmptyCond(t *testing.T) {
	db := dryDB(t)
	stmt := Apply(db.Model(&tPost{}), Cond{}, Like("post_code", "abc")).Find(&[]tPost{}).Statement
	sql := stmt.SQL.String()
	if !strings.Contains(sql, "post_code LIKE ?") {
		t.Fatalf("条件未生效: %s", sql)
	}
	// LIKE 的值必须走绑定参数
	if strings.Contains(sql, "abc") {
		t.Fatalf("值被拼进 SQL，存在注入风险: %s", sql)
	}
}
