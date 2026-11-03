package cryptox

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"
)

// ---- bcrypt ----

// TestHashPasswordUsesCost12 锁住 R9：必须用 cost 12。
//
// Python 端三处新建密码都调 bcrypt.hashpw(pwd, bcrypt.gensalt())，
// 而 bcrypt 库的 gensalt() 无参默认 rounds=12。
// Go 的 bcrypt.DefaultCost 是 10 —— 若有人"顺手"改回 DefaultCost，
// 新建账号的哈希强度就比另两端弱一档，且不会有任何测试失败提示。
func TestHashPasswordUsesCost12(t *testing.T) {
	if BcryptCost != 12 {
		t.Errorf("BcryptCost = %d，必须为 12（对齐 Python bcrypt.gensalt() 默认值）", BcryptCost)
	}

	hash, err := HashPassword("admin123")
	if err != nil {
		t.Fatalf("HashPassword 失败: %v", err)
	}
	if got := HashCost(hash); got != 12 {
		t.Errorf("生成的哈希 cost = %d，期望 12（哈希串: %.10s...）", got, hash)
	}
	if len(hash) != 60 {
		t.Errorf("bcrypt 哈希长度 = %d，期望 60", len(hash))
	}
	// x/crypto/bcrypt 生成 $2a$ 前缀（Python 的 bcrypt 库生成 $2b$）
	if !strings.HasPrefix(hash, prefix2a) {
		t.Errorf("哈希前缀 = %.4s，x/crypto/bcrypt 应生成 %s", hash, prefix2a)
	}
}

func TestCheckPassword(t *testing.T) {
	hash, err := HashPassword("admin123")
	if err != nil {
		t.Fatalf("HashPassword 失败: %v", err)
	}

	if !CheckPassword(hash, "admin123") {
		t.Error("正确密码应校验通过")
	}
	if CheckPassword(hash, "admin124") {
		t.Error("错误密码不应校验通过")
	}
	if CheckPassword(hash, "") {
		t.Error("空密码不应校验通过")
	}
}

// TestCheckPasswordAcceptsSeedHash 用**种子数据里的真实哈希**验证。
//
// 该哈希取自 seed/mysql_data.sql 写入的 admin 账号（与 Python / Java 端逐字节相同），
// 前缀 $2a$10$ 即 cost 10，与 Go 新建时的 cost 12 不同。cost 编码在哈希串内部，
// 故校验必须同时接受两种——这是跨端登录的基础：admin 是种子写入的，
// 用户可能通过 Python、Java 或 Go 任一端登录。
//
// ⚠️ 这条用例是 **P2 登录链路的前置条件**：它通过才说明 Go 能给真实种子账号验密。
const seedAdminHash = "$2a$10$7JB720yubVSZvUI0rEqK/.VqGOZTH.ulu33dHOiBE8ByOhJIrdAu2"

func TestCheckPasswordAcceptsSeedHash(t *testing.T) {
	if got := HashCost(seedAdminHash); got != 10 {
		t.Errorf("种子哈希 cost = %d，期望 10", got)
	}
	if !CheckPassword(seedAdminHash, "admin123") {
		t.Error("种子哈希 + admin123 应校验通过（否则三端无法用同一份种子数据登录）")
	}
	if CheckPassword(seedAdminHash, "wrong-password") {
		t.Error("种子哈希 + 错误密码不应通过")
	}
}

// TestCheckPasswordAccepts2bPrefix 验证能校验 Python bcrypt 库生成的 $2b$ 哈希。
//
// Python 端新建用户产生 $2b$ 前缀，Go 产生 $2a$。若 Go 拒绝 $2b$，
// 那么"用 Python 建的账号无法在 Go 端登录"——这是必须排除的跨端故障。
func TestCheckPasswordAccepts2bPrefix(t *testing.T) {
	// $2b$ 与 $2a$ 对同一 (password, salt) 产生相同的哈希体，仅版本字符不同。
	// 故把上面那个已知可用的 $2a$ 哈希换成 $2b$ 前缀，仍应校验通过。
	hash2b := prefix2b + strings.TrimPrefix(seedAdminHash, prefix2a)
	if !strings.HasPrefix(hash2b, "$2b$10$") {
		t.Fatalf("构造 $2b$ 哈希失败: %s", hash2b)
	}
	if !CheckPassword(hash2b, "admin123") {
		t.Error("$2b$ 前缀的哈希应能被校验通过（Python 端新建账号就是这个前缀）")
	}
	// $2y$（PHP crypt 风格）同样应接受
	hash2y := prefix2y + strings.TrimPrefix(seedAdminHash, prefix2a)
	if !CheckPassword(hash2y, "admin123") {
		t.Error("$2y$ 前缀的哈希应能被校验通过")
	}
}

// TestCheckPasswordStrictDistinguishes 验证 Strict 版本能区分"密码错"与"哈希坏"。
//
// 修改密码接口需要这个区分：密码错 → 业务提示；哈希坏 → 数据损坏，应记日志。
func TestCheckPasswordStrictDistinguishes(t *testing.T) {
	hash, _ := HashPassword("admin123")

	ok, err := CheckPasswordStrict(hash, "admin123")
	if !ok || err != nil {
		t.Errorf("正确密码: ok=%v err=%v，期望 true/nil", ok, err)
	}

	// 密码错误是**正常业务结果**，err 必须为 nil
	ok, err = CheckPasswordStrict(hash, "wrong")
	if ok || err != nil {
		t.Errorf("错误密码: ok=%v err=%v，期望 false/nil（err 非 nil 会让调用方误判为数据损坏）", ok, err)
	}

	// 哈希为空
	if _, err = CheckPasswordStrict("", "x"); err == nil {
		t.Error("空哈希应返回 error")
	}
	// 哈希前缀非法
	if _, err = CheckPasswordStrict("$1$abcdef", "x"); err == nil {
		t.Error("非 bcrypt 前缀应返回 error")
	}
	// 哈希长度不足
	if _, err = CheckPasswordStrict("$2a$10$short", "x"); err == nil {
		t.Error("畸形 bcrypt 哈希应返回 error")
	}
}

// TestBcryptTruncatesAt72Bytes 记录 bcrypt 的 72 字节截断行为。
//
// 两端都会截断，故不是互通问题；但调用方不应依赖第 72 字节之后的熵，
// 这个测试把该行为固化下来，避免日后有人误以为长密码全长参与哈希。
func TestBcryptTruncatesAt72Bytes(t *testing.T) {
	base := strings.Repeat("a", 72)
	hash, err := HashPassword(base)
	if err != nil {
		t.Fatalf("HashPassword 失败: %v", err)
	}
	// 第 73 字节起被截断，故两个不同的长密码会校验通过同一个哈希
	if !CheckPassword(hash, base+"SUFFIX-SHOULD-BE-IGNORED") {
		t.Error("bcrypt 应截断 72 字节之后的内容（两端行为一致，此处固化该事实）")
	}
}

// ---- JWT ----

// TestSignTokenProducesVerifiableToken 基础往返。
func TestSignTokenProducesVerifiableToken(t *testing.T) {
	const secret = "test-secret-key-that-is-long-enough-32bytes"
	uid := int64(904941282285273088)
	name := "admin"
	clientID := "e5cd7e4891bf95d1d19206ce24a7b32e"

	claims := Claims{
		Sub:      "session-uuid-1234",
		UserID:   &uid,
		UserName: &name,
		ClientID: &clientID,
		Exp:      time.Now().Add(time.Hour).Unix(),
	}

	token, err := SignToken(claims, secret)
	if err != nil {
		t.Fatalf("SignToken 失败: %v", err)
	}
	if strings.Count(token, ".") != 2 {
		t.Errorf("JWT 应有 3 段（2 个点），实际 %q", token)
	}

	got, err := ParseToken(token, secret, true)
	if err != nil {
		t.Fatalf("ParseToken 失败: %v", err)
	}
	if got.Sub != claims.Sub {
		t.Errorf("sub = %q，期望 %q", got.Sub, claims.Sub)
	}
	if got.UserID == nil || *got.UserID != uid {
		t.Errorf("user_id = %v，期望 %d", got.UserID, uid)
	}
	if got.UserName == nil || *got.UserName != name {
		t.Errorf("user_name = %v，期望 %q", got.UserName, name)
	}
	if got.ClientID == nil || *got.ClientID != clientID {
		t.Errorf("clientid = %v，期望 %q", got.ClientID, clientID)
	}
	if got.IsRefresh {
		t.Error("is_refresh 默认应为 false")
	}
}

// TestClaimsJSONShape 是 JWT 契约的核心断言。
//
// 三条必须守住的规则（对应 Python 端 JWTPayloadSchema + model_dump(exclude_none=False)）：
//  1. 未设置的可选字段必须以 **null** 出现，不能整个消失
//  2. **不得**出现 Python 没有的注册声明（iat / nbf / iss / aud / jti）
//  3. 字段顺序与 Pydantic 模型声明顺序一致（影响 claims 的 JSON 字节序）
func TestClaimsJSONShape(t *testing.T) {
	const secret = "test-secret-key-that-is-long-enough-32bytes"
	uid := int64(1)

	token, err := SignToken(Claims{
		Sub:    "sess-1",
		UserID: &uid,
		Exp:    1893456000, // 2030-01-01
	}, secret)
	if err != nil {
		t.Fatalf("SignToken 失败: %v", err)
	}

	payloadB64 := strings.Split(token, ".")[1]
	// JWT 用 base64url 无填充编码
	if m := len(payloadB64) % 4; m != 0 {
		payloadB64 += strings.Repeat("=", 4-m)
	}
	raw, err := base64.StdEncoding.DecodeString(strings.ReplaceAll(strings.ReplaceAll(payloadB64, "-", "+"), "_", "/"))
	if err != nil {
		t.Fatalf("解码 payload 失败: %v", err)
	}

	// ---- 规则 1：可选字段以 null 出现 ----
	var m map[string]any
	if err := json.Unmarshal(raw, &m); err != nil {
		t.Fatalf("payload 不是合法 JSON: %v (%s)", err, raw)
	}
	for _, field := range []string{"sub", "user_id", "user_name", "dept_id", "dept_name",
		"dept_category", "clientid", "is_refresh", "exp"} {
		if _, exists := m[field]; !exists {
			t.Errorf("claims 缺少字段 %q —— Python 端 model_dump(exclude_none=False) 会让它以 null 出现", field)
		}
	}
	for _, field := range []string{"user_name", "dept_id", "dept_name", "dept_category", "clientid"} {
		if v, ok := m[field]; ok && v != nil {
			t.Errorf("未设置的 %q 应为 null，实际为 %#v", field, v)
		}
	}

	// ---- 规则 2：不得有 Python 没有的注册声明 ----
	for _, forbidden := range []string{"iat", "nbf", "iss", "aud", "jti"} {
		if _, exists := m[forbidden]; exists {
			t.Errorf("claims 含 %q —— PyJWT 的 jwt.encode 不会自动添加该声明，多出来会破坏契约", forbidden)
		}
	}
	if len(m) != 9 {
		t.Errorf("claims 字段数 = %d，期望恰好 9 个（%v）", len(m), keysOf(m))
	}

	// ---- 规则 3：字段顺序 ----
	wantOrder := `{"sub":"sess-1","user_id":1,"user_name":null,"dept_id":null,"dept_name":null,` +
		`"dept_category":null,"clientid":null,"is_refresh":false,"exp":1893456000}`
	if string(raw) != wantOrder {
		t.Errorf("claims JSON 不符\n  got  = %s\n  want = %s", raw, wantOrder)
	}

	// exp 必须是数字而非字符串
	if expNum, ok := m["exp"].(float64); !ok || int64(expNum) != 1893456000 {
		t.Errorf("exp = %#v，期望数字 1893456000", m["exp"])
	}
}

func keysOf(m map[string]any) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}

// rawToken 拼一个 header/payload 可自定义的 token（签名段是占位值，不验签）。
//
// 用于构造「结构合法但 header 有问题」的 token —— 例如 alg=none 的伪造令牌。
// 这类输入无法用 SignToken 生成（它恒定写 HS256），只能手工拼。
func rawToken(t *testing.T, header, payload map[string]any) string {
	t.Helper()
	enc := func(v map[string]any) string {
		blob, err := json.Marshal(v)
		if err != nil {
			t.Fatalf("序列化失败: %v", err)
		}
		return base64.RawURLEncoding.EncodeToString(blob)
	}
	return enc(header) + "." + enc(payload) + "." + base64.RawURLEncoding.EncodeToString([]byte("sig"))
}

// TestParseTokenErrorKinds 验证错误分类与 Python 端四个 except 分支一一对应。
//
// 分类必须准确：前端按文案区分"过期需重新登录"与"token 非法"，
// 混为一谈会让用户在 token 过期时看到莫名其妙的提示。
func TestParseTokenErrorKinds(t *testing.T) {
	const secret = "test-secret-key-that-is-long-enough-32bytes"

	valid, _ := SignToken(Claims{Sub: "s1", Exp: time.Now().Add(time.Hour).Unix()}, secret)
	expired, _ := SignToken(Claims{Sub: "s2", Exp: time.Now().Add(-time.Hour).Unix()}, secret)

	cases := []struct {
		name      string
		token     string
		secret    string
		verifyExp bool
		wantKind  TokenErrorKind
		wantMsg   string
	}{
		{"空 token", "", secret, true, TokenErrEmpty, "认证不存在，请重新登录"},
		{"签名错误", valid, "another-secret-key-also-32-bytes-long", true, TokenErrInvalid, "无效认证，请重新登录"},
		{"已过期", expired, secret, true, TokenErrExpired, "认证已过期，请重新登录"},
		// ⚠️ 结构非法属 PyJWT 的 **DecodeError**（被第一个 except 捕获），
		// 文案是"无效认证"而不是"token已失效"。四种畸形形态已逐一实测比对
		// （见 test/contract/diff_system.py 用例 1），Python 端全部返回"无效认证"。
		{"结构非法：三段但解不开", "not.a.jwt", secret, true, TokenErrInvalid, "无效认证，请重新登录"},
		{"结构非法：单段", "abc", secret, true, TokenErrInvalid, "无效认证，请重新登录"},
		{"结构非法：两段", "a.b", secret, true, TokenErrInvalid, "无效认证，请重新登录"},
		{"结构非法：非法 base64", "!!!.@@@.###", secret, true, TokenErrInvalid, "无效认证，请重新登录"},
		// 算法不被允许属 PyJWT 的 **InvalidAlgorithmError**（不是 DecodeError），
		// 落到第三个 except 分支 → "token已失效"。golang-jwt 把它和"签名错误"
		// 报成同一个 ErrTokenSignatureInvalid，故 ParseToken 里做了前置判定。
		{"alg=none 伪造", rawToken(t, map[string]any{"alg": "none", "typ": "JWT"},
			map[string]any{"sub": "x", "exp": 9999999999}), secret, true,
			TokenErrMalformed, "token已失效，请重新登录"},
		{"alg 未知（HS999）", rawToken(t, map[string]any{"alg": "HS999", "typ": "JWT"},
			map[string]any{"sub": "x", "exp": 9999999999}), secret, true,
			TokenErrMalformed, "token已失效，请重新登录"},
		{"header 缺 alg", rawToken(t, map[string]any{"typ": "JWT"},
			map[string]any{"sub": "x", "exp": 9999999999}), secret, true,
			TokenErrMalformed, "token已失效，请重新登录"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := ParseToken(tc.token, tc.secret, tc.verifyExp)
			if err == nil {
				t.Fatal("应当返回错误")
			}
			var te *TokenError
			if !errors.As(err, &te) {
				t.Fatalf("错误类型应为 *TokenError，实际 %T", err)
			}
			if te.Kind != tc.wantKind {
				t.Errorf("Kind = %d，期望 %d", te.Kind, tc.wantKind)
			}
			if te.Message() != tc.wantMsg {
				t.Errorf("Message = %q，期望 %q（文案是契约的一部分）", te.Message(), tc.wantMsg)
			}
		})
	}

	// 滑动过期：verifyExp=false 时过期 token 仍应解析成功
	got, err := ParseToken(expired, secret, false)
	if err != nil {
		t.Errorf("verifyExp=false 时过期 token 应能解析（滑动过期由 Redis TTL 决定），却失败: %v", err)
	} else if got.Sub != "s2" {
		t.Errorf("sub = %q，期望 s2", got.Sub)
	}
}

// TestSignTokenRejectsBadInput 验证前置校验。
func TestSignTokenRejectsBadInput(t *testing.T) {
	if _, err := SignToken(Claims{Sub: "s", Exp: 1}, ""); err == nil {
		t.Error("空 SECRET_KEY 应拒绝签发")
	}
	if _, err := SignToken(Claims{Sub: "", Exp: 1}, "secret"); err == nil {
		t.Error("空 sub 应拒绝签发（Python 端 model_validator 同样拒绝）")
	}
	if _, err := SignToken(Claims{Sub: "   ", Exp: 1}, "secret"); err == nil {
		t.Error("纯空白 sub 应拒绝签发")
	}
}

// TestAlgorithmIsHS256 锁住算法，防止被改成 none 或 RS256（算法替换攻击面）。
func TestAlgorithmIsHS256(t *testing.T) {
	if Algorithm != "HS256" {
		t.Errorf("Algorithm = %q，必须为 HS256（与 Python settings.ALGORITHM 一致）", Algorithm)
	}
	// alg=none 的 token 必须被拒绝
	const secret = "test-secret-key-that-is-long-enough-32bytes"
	noneToken := "eyJhbGciOiJub25lIiwidHlwIjoiSldUIn0." +
		"eyJzdWIiOiJoYWNrZXIiLCJleHAiOjk5OTk5OTk5OTl9."
	if _, err := ParseToken(noneToken, secret, true); err == nil {
		t.Error("alg=none 的 token 必须被拒绝（WithValidMethods 应已拦截）")
	}
}
