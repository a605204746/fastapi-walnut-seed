package envelope

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

func init() { gin.SetMode(gin.TestMode) }

// serve 用一个临时 gin 引擎执行 handler，返回状态码、原始响应体与响应头。
func serve(t *testing.T, h gin.HandlerFunc) (int, string, http.Header) {
	t.Helper()
	e := gin.New()
	e.GET("/probe", h)

	w := httptest.NewRecorder()
	req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/probe", nil)
	e.ServeHTTP(w, req)

	resp := w.Result()
	defer resp.Body.Close()
	return w.Code, w.Body.String(), resp.Header
}

// TestSuccessShape 验证信封的字节级形状。
//
// 三个关键点：
//  1. data 为 nil 时字段**必须存在**且为 null（Python 端 ApiResponse.data 默认 None）
//  2. 响应体**末尾不能有换行**——json.Encoder.Encode 会追加 '\n'，
//     而 Python 的 JSONResponse 不会，契约 diff 是逐字节比对
//  3. 字段顺序为 code / msg / data
func TestSuccessShape(t *testing.T) {
	_, body, _ := serve(t, func(c *gin.Context) { OK(c, nil) })

	want := `{"code":200,"msg":"操作成功","data":null}`
	if body != want {
		t.Errorf("响应体不符\n  got  = %q\n  want = %q", body, want)
	}
}

func TestSuccessWithData(t *testing.T) {
	_, body, _ := serve(t, func(c *gin.Context) {
		OK(c, map[string]any{"userName": "admin", "status": 0})
	})
	// status: 0 必须出现，不能被 omitempty 吞掉
	want := `{"code":200,"msg":"操作成功","data":{"status":0,"userName":"admin"}}`
	if body != want {
		t.Errorf("响应体不符\n  got  = %q\n  want = %q", body, want)
	}
}

func TestOKMsg(t *testing.T) {
	code, body, _ := serve(t, func(c *gin.Context) {
		OKMsg(c, gin.H{"status": 1}, "系统健康")
	})
	if code != http.StatusOK {
		t.Errorf("HTTP 状态码 = %d，期望 200", code)
	}
	want := `{"code":200,"msg":"系统健康","data":{"status":1}}`
	if body != want {
		t.Errorf("响应体不符\n  got  = %q\n  want = %q", body, want)
	}
}

// TestFailUsesHTTP200 验证业务失败的关键契约：HTTP 200 + body.code=500。
// 前端据 body.code 判断，而不是 HTTP 状态码。
func TestFailUsesHTTP200(t *testing.T) {
	code, body, _ := serve(t, func(c *gin.Context) { Fail(c, "用户名已存在") })
	if code != http.StatusOK {
		t.Errorf("业务失败的 HTTP 状态码 = %d，期望 200（真实结果由 body.code 表达）", code)
	}
	want := `{"code":500,"msg":"用户名已存在","data":null}`
	if body != want {
		t.Errorf("响应体不符\n  got  = %q\n  want = %q", body, want)
	}
}

// TestWarnCode601 验证警告码。
func TestWarnCode601(t *testing.T) {
	_, body, _ := serve(t, func(c *gin.Context) { Warn(c, nil, "操作警告") })
	want := `{"code":601,"msg":"操作警告","data":null}`
	if body != want {
		t.Errorf("响应体不符\n  got  = %q\n  want = %q", body, want)
	}
}

// TestFailWithRealStatus 验证 401/403/404/405/503 走**真实 HTTP 状态码**但仍带信封体。
func TestFailWithRealStatus(t *testing.T) {
	cases := []struct {
		httpStatus int
		bodyCode   int
		msg        string
	}{
		{http.StatusNotFound, CodeNotFound, "请求地址不存在"},
		{http.StatusMethodNotAllowed, CodeBadMethod, "Method Not Allowed"},
		{http.StatusServiceUnavailable, 503, "依赖未就绪"},
	}
	for _, tc := range cases {
		code, body, _ := serve(t, func(c *gin.Context) {
			FailWith(c, tc.httpStatus, tc.bodyCode, tc.msg, nil)
		})
		if code != tc.httpStatus {
			t.Errorf("HTTP 状态码 = %d，期望 %d", code, tc.httpStatus)
		}
		var env Response
		if err := json.Unmarshal([]byte(body), &env); err != nil {
			t.Fatalf("响应体不是合法 JSON: %v (%s)", err, body)
		}
		if env.Code != tc.bodyCode || env.Msg != tc.msg {
			t.Errorf("信封体 = %+v，期望 code=%d msg=%s", env, tc.bodyCode, tc.msg)
		}
	}
}

// TestFailWithZeroStatusFallsBackTo200 验证 httpStatus 传 0 时沿用 200。
func TestFailWithZeroStatusFallsBackTo200(t *testing.T) {
	code, _, _ := serve(t, func(c *gin.Context) {
		FailWith(c, 0, CodeBadRequest, "参数错误", nil)
	})
	if code != http.StatusOK {
		t.Errorf("httpStatus=0 时 HTTP 状态码 = %d，期望 200", code)
	}
}

// TestPageEmptyRowsIsArray 是分页最易踩的坑：
// Go 的 nil slice 会序列化成 null，而 Python 端 PageResult.rows 用
// default_factory=list 恒为 []。前端拿到 null 后 v-for 会崩。
func TestPageEmptyRowsIsArray(t *testing.T) {
	type row struct {
		ID int64 `json:"id"`
	}

	// 显式传 nil —— 最常见的「查无数据」写法
	_, body, _ := serve(t, func(c *gin.Context) {
		Page[row](c, nil, 0)
	})
	want := `{"code":200,"msg":"操作成功","data":{"rows":[],"total":0}}`
	if body != want {
		t.Errorf("空分页响应不符\n  got  = %q\n  want = %q", body, want)
	}

	// 空切片同样要输出 []
	_, body, _ = serve(t, func(c *gin.Context) {
		Page[row](c, []row{}, 0)
	})
	if body != want {
		t.Errorf("空切片分页响应不符\n  got  = %q", body)
	}

	// 有数据时形状正确
	_, body, _ = serve(t, func(c *gin.Context) {
		Page[row](c, []row{{ID: 1}, {ID: 2}}, 2)
	})
	want = `{"code":200,"msg":"操作成功","data":{"rows":[{"id":1},{"id":2}],"total":2}}`
	if body != want {
		t.Errorf("分页响应不符\n  got  = %q\n  want = %q", body, want)
	}
}

func TestNewPageNormalizesNil(t *testing.T) {
	p := NewPage[int](nil, 0)
	if p.Rows == nil {
		t.Error("NewPage(nil) 应把 Rows 归一化为空切片，否则序列化成 null")
	}
	if len(p.Rows) != 0 {
		t.Errorf("Rows 长度 = %d，期望 0", len(p.Rows))
	}
}

// TestContentType 验证 Content-Type 与 Python 端 EnvelopeResponse 一致。
// Python 显式设置 "application/json; charset=utf-8"。
func TestContentType(t *testing.T) {
	_, _, header := serve(t, func(c *gin.Context) { OK(c, nil) })
	ct := header.Get("Content-Type")
	if ct != "application/json" && ct != "application/json; charset=utf-8" {
		t.Errorf("Content-Type = %q", ct)
	}
}
