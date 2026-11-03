package cryptox

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

// Algorithm HS256 —— 与 Python 端 settings.ALGORITHM 默认值一致。
// 三端共用同一个 SECRET_KEY，算法必须相同，否则互相解不开。
const Algorithm = "HS256"

// Claims 是 JWT 载荷，字段与顺序严格对齐 Python 端 JWTPayloadSchema。
//
// ⚠️ 三个必须守住的细节：
//
//  1. **可选字段用指针且不加 omitempty**。Python 端签发时调
//     `payload.model_dump(exclude_none=False)`，即 None 字段会以 `null` 出现在
//     claims 里。Go 若用值类型 + omitempty，未设置的字段会整个消失，
//     claims 形状就与另两端不同。
//
//  2. **不得添加 Python 没有的注册声明**。PyJWT 的 `jwt.encode` 只写调用方给的
//     payload，不会自动补 iat / nbf / iss / aud。Go 侧若用 jwt.RegisteredClaims
//     或手动塞 iat，token 就会多出字段。
//
//  3. **字段顺序影响 token 字符串**。Go 的 encoding/json 按结构体字段序输出，
//     此处顺序照抄 JWTPayloadSchema 的声明顺序，使 claims 的 JSON 字节序一致。
//
// 已知且已接受的差异：JWT **header** 的键序不同——PyJWT 输出
// `{"typ":"JWT","alg":"HS256"}`，golang-jwt 输出 `{"alg":"HS256","typ":"JWT"}`。
// 因此同一 payload 两端签出的 token 字符串不相同，但**互相可验证**
// （header 是自描述的，解析方按 alg 字段选算法）。三端的 token 本就不要求可互换
// （各自有独立的 Redis 会话），故不为此牺牲 golang-jwt 的标准实现。
type Claims struct {
	// Sub 登录ID/会话编号，必填（Python 端 decode 时会校验非空）
	Sub string `json:"sub"`
	// UserID 用户ID
	UserID *int64 `json:"user_id"`
	// UserName 用户名
	UserName *string `json:"user_name"`
	// DeptID 部门ID
	DeptID *int64 `json:"dept_id"`
	// DeptName 部门名称
	DeptName *string `json:"dept_name"`
	// DeptCategory 部门类别
	DeptCategory *string `json:"dept_category"`
	// ClientID 客户端ID。与请求头 clientid 需一致，否则视为跨客户端盗用
	ClientID *string `json:"clientid"`
	// IsRefresh 是否刷新 token
	IsRefresh bool `json:"is_refresh"`
	// Exp 过期时间（Unix 秒）。Python 端把 datetime 转成 int(timestamp())
	Exp int64 `json:"exp"`
}

// ---- 实现 jwt.Claims 接口，完全掌控序列化形状 ----

func (c Claims) GetExpirationTime() (*jwt.NumericDate, error) {
	if c.Exp == 0 {
		return nil, nil
	}
	return jwt.NewNumericDate(time.Unix(c.Exp, 0)), nil
}

func (c Claims) GetIssuedAt() (*jwt.NumericDate, error) { return nil, nil }

func (c Claims) GetNotBefore() (*jwt.NumericDate, error) { return nil, nil }

func (c Claims) GetIssuer() (string, error) { return "", nil }

func (c Claims) GetSubject() (string, error) { return c.Sub, nil }

func (c Claims) GetAudience() (jwt.ClaimStrings, error) { return nil, nil }

var _ jwt.Claims = Claims{}

// 认证失败的分类。对应 Python 端 decode_access_token 的四个 except 分支，
// 每个分支的提示文案不同，故必须能区分——不能笼统返回一个 error。
type TokenErrorKind int

const (
	// TokenOK 无错误
	TokenOK TokenErrorKind = iota
	// TokenErrEmpty token 为空 → "认证不存在，请重新登录"
	TokenErrEmpty
	// TokenErrInvalid 签名错误或无法解析 → "无效认证，请重新登录"
	TokenErrInvalid
	// TokenErrExpired 已过期 → "认证已过期，请重新登录"
	TokenErrExpired
	// TokenErrNoSubject 能解析但 sub 为空 → "无效认证，请重新登录"
	TokenErrNoSubject
	// TokenErrMalformed 其他 token 结构问题 → "token已失效，请重新登录"
	TokenErrMalformed
)

// TokenError 携带分类的认证错误。
type TokenError struct {
	Kind TokenErrorKind
	Err  error
}

func (e *TokenError) Error() string {
	if e.Err != nil {
		return e.Err.Error()
	}
	return "token 无效"
}

func (e *TokenError) Unwrap() error { return e.Err }

// Message 返回与 Python 端逐字一致的提示文案。
//
// ⚠️ 这些字符串是契约的一部分（前端会直接展示），改动需同步 Python / Java 端。
// P1 阶段先硬编码中文；接入 i18nx 后应改为消息键解析。
func (e *TokenError) Message() string {
	switch e.Kind {
	case TokenErrEmpty:
		return "认证不存在，请重新登录"
	case TokenErrInvalid, TokenErrNoSubject:
		return "无效认证，请重新登录"
	case TokenErrExpired:
		return "认证已过期，请重新登录"
	default:
		return "token已失效，请重新登录"
	}
}

// SignToken 签发 JWT。对应 Python 端 create_access_token。
func SignToken(claims Claims, secret string) (string, error) {
	if strings.TrimSpace(secret) == "" {
		return "", errors.New("SECRET_KEY 为空，无法签发 token")
	}
	if strings.TrimSpace(claims.Sub) == "" {
		// Python 端 JWTPayloadSchema 的 model_validator 会拒绝空 sub，
		// 这里同样前置拦截，避免签出一个解不开的 token
		return "", errors.New("会话编号不能为空")
	}
	return jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString([]byte(secret))
}

// ParseToken 解析并校验 JWT。对应 Python 端 decode_access_token。
//
// verifyExp=false 用于**滑动过期**场景：token 的 exp 只是名义值，
// 实际有效期由 Redis 会话 TTL 决定（Python 端同名参数、同一语义）。
func ParseToken(tokenString, secret string, verifyExp bool) (*Claims, error) {
	if strings.TrimSpace(tokenString) == "" {
		return nil, &TokenError{Kind: TokenErrEmpty}
	}

	// ⚠️ 必须在 jwt.ParseWithClaims **之前**自行判定算法，原因见 checkAlgAllowed。
	if !checkAlgAllowed(tokenString) {
		return nil, &TokenError{
			Kind: TokenErrMalformed,
			Err:  errors.New("token 的 alg 缺失或不在允许列表内"),
		}
	}

	parserOpts := []jwt.ParserOption{
		jwt.WithValidMethods([]string{Algorithm}), // 拒绝 alg=none 与算法替换攻击
	}
	if !verifyExp {
		parserOpts = append(parserOpts, jwt.WithoutClaimsValidation())
	}

	claims := &Claims{}
	_, err := jwt.ParseWithClaims(tokenString, claims,
		func(t *jwt.Token) (any, error) { return []byte(secret), nil },
		parserOpts...)
	if err != nil {
		return nil, classify(err)
	}
	if strings.TrimSpace(claims.Sub) == "" {
		return nil, &TokenError{Kind: TokenErrNoSubject}
	}
	return claims, nil
}

// checkAlgAllowed 判定 token header 的 alg 是否为本端允许的算法。
//
// 为什么不能只靠 jwt.WithValidMethods：**两端对「算法不被允许」报的错撞车了**。
// PyJWT 抛 InvalidAlgorithmError（InvalidTokenError 的子类，但不是 DecodeError），
// 落在第三个 except 分支 → "token已失效，请重新登录"；
// 而 golang-jwt 把它归到 ErrTokenSignatureInvalid，与「签名不对」
// （PyJWT 的 InvalidSignatureError → 第二个分支 → "无效认证，请重新登录"）
// 是同一个错误值，事后无法区分。故在解析前先自行判定。
//
// 返回 true 表示「放行给 golang-jwt 继续解析」，**包括 header 根本解不开的情况**：
// 那属于格式错误，对应 PyJWT 的 DecodeError → "无效认证"，交给 ErrTokenMalformed 分支。
// 只有「header 能正常解析、但 alg 缺失或不在允许列表内」才返回 false。
//
// 实测对齐（diff_system.py 用例，Python 端为基准）：
//
//	alg=none 的伪造 token  → py "token已失效"  ✓
//	alg=HS999（未知算法）  → py "token已失效"  ✓
//	not.a.jwt（header 解不开）→ py "无效认证"  ✓
func checkAlgAllowed(tokenString string) bool {
	head, _, found := strings.Cut(tokenString, ".")
	if !found {
		return true // 段数不足，交给 ParseWithClaims 报 ErrTokenMalformed
	}
	raw, err := base64.RawURLEncoding.DecodeString(head)
	if err != nil {
		// JWT 段通常不带 '=' 填充，但 PyJWT 两种都收，这里同样兼容
		if raw, err = base64.URLEncoding.DecodeString(head); err != nil {
			return true
		}
	}
	var header struct {
		// 用指针区分「alg 缺失」与「alg 为空串」：
		// 缺失时 PyJWT 抛 KeyError → InvalidTokenError → "token已失效"
		Alg *string `json:"alg"`
	}
	if err := json.Unmarshal(raw, &header); err != nil {
		return true // header 不是合法 JSON，属格式错误
	}
	if header.Alg == nil {
		return false
	}
	return *header.Alg == Algorithm
}

// classify 把 golang-jwt 的错误映射到与 Python 端四个 except 分支等价的分类。
//
// Python 端 decode_access_token 的分支顺序：
//
//	(jwt.InvalidSignatureError, jwt.DecodeError)  → "无效认证，请重新登录"
//	jwt.ExpiredSignatureError                     → "认证已过期，请重新登录"
//	jwt.InvalidTokenError                         → "token已失效，请重新登录"
//
// ⚠️ **格式错误的 token 属第二个分支**：PyJWT 对「段数不足」「base64 解不开」
// 「payload 不是合法 JSON」抛的都是 DecodeError（InvalidTokenError 的子类，
// 但被第一个 except 先捕获），文案是"无效认证"。
// 早期版本把 ErrTokenMalformed 归到 TokenErrMalformed（"token已失效"），
// 与 Python 端差了一档——P2 的契约比对只覆盖了「不带 token」的路径，没暴露出来。
func classify(err error) *TokenError {
	switch {
	case errors.Is(err, jwt.ErrTokenSignatureInvalid),
		errors.Is(err, jwt.ErrTokenMalformed):
		return &TokenError{Kind: TokenErrInvalid, Err: err}
	case errors.Is(err, jwt.ErrTokenExpired):
		return &TokenError{Kind: TokenErrExpired, Err: err}
	case errors.Is(err, jwt.ErrTokenUnverifiable),
		errors.Is(err, jwt.ErrTokenInvalidIssuer),
		errors.Is(err, jwt.ErrTokenNotValidYet):
		return &TokenError{Kind: TokenErrMalformed, Err: err}
	default:
		// 其余（含签名解析失败）按 Python 端 DecodeError 分支处理
		return &TokenError{Kind: TokenErrInvalid, Err: fmt.Errorf("token 解析失败: %w", err)}
	}
}

// ExpiresAt 由当前时间 + TTL 计算 exp（Unix 秒）。
// 与 Python 端 `datetime.now() + timedelta(seconds=...)` 再 `int(ts)` 等价。
func ExpiresAt(ttl time.Duration) int64 {
	return time.Now().Add(ttl).Unix()
}
