// Package idempotentx 实现防重提交，对应 Python 端 app/core/idempotent.py 的 RepeatSubmit。
//
// 机制：以「token 头 + 请求体 + 排序后的查询参数」算 MD5 作为请求指纹，
// 用 Redis SETNX + TTL 占位。窗口内相同指纹的第二次请求被拒绝。
//
// # 键结构
//
//	{REDIS_KEY_PREFIX}global:repeat_submit:{请求路径}{指纹MD5}
//
// 与 Python 端逐段一致。⚠️ 业务代码**禁止自己拼 Redis 键**，一律走
// cachex.Client.Key()——三端前缀不同（walnut-seed-go / -python / -java），
// 写错会与另两端串键，导致一端的提交把另一端也锁住。
package idempotentx

import (
	"bytes"
	"crypto/md5"
	"encoding/hex"
	"io"
	"math"
	"net/url"
	"sort"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"walnut-seed-go/internal/pkg/cachex"
	"walnut-seed-go/internal/pkg/errs"
	"walnut-seed-go/internal/pkg/i18nx"
)

const (
	// DefaultInterval 默认防重窗口（毫秒），对应 Python 端 RepeatSubmit(interval=5000)。
	DefaultInterval = 5000
	// MinInterval 最小窗口。低于此值 Python 端在**请求时**抛异常，此处保持同样时机。
	MinInterval = 1000
	// msgMinInterval 是 Python 端的字面量，属契约文案。
	msgMinInterval = "重复提交间隔时间不能小于'1'秒"
	// keyPrefix 对应 CacheNames.REPEAT_SUBMIT_KEY。
	keyPrefix = "global:repeat_submit:"
	// msgKey 是 i18n 消息键，对应 MessageUtils.message("repeat.submit.message")。
	msgKey = "repeat.submit.message"
	// DefaultTokenHeader 对应 settings.TOKEN_NAME。
	DefaultTokenHeader = "Authorization"
)

// Guard 是防重提交中间件的工厂。
type Guard struct {
	cache       *cachex.Client
	errs        *errs.Handler
	tokenHeader string
	localeOf    func(*gin.Context) string
}

// New 构造工厂。localeOf 用于按请求语言解析提示文案，nil 时用 zh_CN 兜底。
func New(cache *cachex.Client, eh *errs.Handler, localeOf func(*gin.Context) string) *Guard {
	return &Guard{cache: cache, errs: eh, tokenHeader: DefaultTokenHeader, localeOf: localeOf}
}

// Check 构造防重窗口为 intervalMS 毫秒的中间件（提示文案走 i18n）。
func (g *Guard) Check(intervalMS int) gin.HandlerFunc {
	return g.CheckMsg(intervalMS, "")
}

// Default 是 5 秒窗口的便捷写法，覆盖绝大多数写端点。
func (g *Guard) Default() gin.HandlerFunc { return g.Check(DefaultInterval) }

// CheckMsg 构造防重中间件，message 非空时用它替代 i18n 文案。
//
// 对应 Python 端 RepeatSubmit(interval=..., message=...)。
func (g *Guard) CheckMsg(intervalMS int, message string) gin.HandlerFunc {
	return func(c *gin.Context) {
		// 与 Python 一致：这是**请求时**检查，不是构造时。
		// 配置错误的间隔要让调用方看到明确文案，而不是静默放行。
		if intervalMS < MinInterval {
			g.abort(c, errs.New(msgMinInterval))
			return
		}
		if g.cache == nil {
			// Redis 未就绪。fail-closed：宁可拒绝写入，不可放过重复提交。
			// （实际上此时登录也已不可用，拿不到 token 走不到这里。）
			g.abort(c, errs.New(errs.MsgSystemError))
			return
		}

		fingerprint, err := g.fingerprint(c)
		if err != nil {
			g.abort(c, errs.Wrap(err, errs.MsgSystemError))
			return
		}

		key := keyPrefix + c.Request.URL.Path + fingerprint
		// expire = max(1, ceil(interval/1000)) 秒，与 Python 端同式
		expire := time.Duration(max64(1, int64(math.Ceil(float64(intervalMS)/1000.0)))) * time.Second

		if !g.cache.SetIfAbsent(c.Request.Context(), key, "", expire) {
			g.abort(c, errs.New(g.message(c, message)))
			return
		}
		c.Next()
	}
}

// message 取提示文案：显式 message 优先，否则按请求语言解析 i18n。
func (g *Guard) message(c *gin.Context, explicit string) string {
	if explicit != "" {
		return explicit
	}
	locale := i18nx.DefaultLocale
	if g.localeOf != nil {
		if l := g.localeOf(c); l != "" {
			locale = l
		}
	}
	if msg := i18nx.MessageIn(locale, msgKey); msg != "" && msg != msgKey {
		return msg
	}
	return "不允许重复提交，请稍候再试"
}

// fingerprint 计算请求指纹（MD5 十六进制）。
//
// 对应 Python 端：
//
//	token_header = (headers.get(TOKEN_NAME) or "").strip()
//	args_string  = body + str(sorted(query_params.items()))
//	submit_key   = md5(f"{token_header}:{args_string}")
//
// ⚠️ 读 body 会消费掉流，**必须重新塞回去**，否则后续 handler 拿到空请求体。
func (g *Guard) fingerprint(c *gin.Context) (string, error) {
	tokenHeader := strings.TrimSpace(c.GetHeader(g.tokenHeader))

	var body string
	if c.Request.Body != nil {
		buf, err := io.ReadAll(c.Request.Body)
		if err != nil {
			return "", err
		}
		// 还原请求体，供后续 handler 绑定
		c.Request.Body = io.NopCloser(bytes.NewReader(buf))
		body = string(buf)
	}

	raw := tokenHeader + ":" + body + canonicalQuery(c.Request.URL.Query())
	sum := md5.Sum([]byte(raw))
	return hex.EncodeToString(sum[:]), nil
}

// canonicalQuery 把查询参数序列化成稳定字符串。
//
// 对应 Python 的 str(sorted(query_params.items()))。**不追求与 Python 的
// 字面输出一致**——Redis 键按端隔离（前缀不同），两端各自成对即可；
// 需要保证的是同一端的**稳定性**：同样的参数必须得到同样的指纹，
// 故这里同样「展开全部键值对（含重复键）后按 (键, 值) 排序」。
func canonicalQuery(values url.Values) string {
	type pair struct{ k, v string }
	pairs := make([]pair, 0, len(values))
	for k, vs := range values {
		for _, v := range vs {
			pairs = append(pairs, pair{k, v})
		}
	}
	sort.Slice(pairs, func(i, j int) bool {
		if pairs[i].k != pairs[j].k {
			return pairs[i].k < pairs[j].k
		}
		return pairs[i].v < pairs[j].v
	})
	var b strings.Builder
	b.WriteByte('[')
	for i, p := range pairs {
		if i > 0 {
			b.WriteByte(',')
		}
		b.WriteString(p.k)
		b.WriteByte('=')
		b.WriteString(p.v)
	}
	b.WriteByte(']')
	return b.String()
}

// abort 渲染业务异常并中止。
func (g *Guard) abort(c *gin.Context, se *errs.ServiceError) {
	if g.errs != nil {
		g.errs.Abort(c, se)
		return
	}
	c.Abort()
	c.JSON(se.Status(), gin.H{"code": se.Code, "msg": se.Msg, "data": nil})
}

func max64(a, b int64) int64 {
	if a > b {
		return a
	}
	return b
}
