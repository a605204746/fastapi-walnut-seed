package config

import (
	"os"
	"strings"
	"testing"
)

func TestDefaultsMatchPython(t *testing.T) {
	c := defaults()

	// 逐项对齐 app/config/setting.py 的默认值
	checks := []struct {
		name string
		got  any
		want any
	}{
		{"SERVER_PORT", c.ServerPort, 8011},
		{"SERVER_HOST", c.ServerHost, "0.0.0.0"},
		{"ENVIRONMENT", string(c.Environment), "dev"},
		{"ALGORITHM", c.Algorithm, "HS256"},
		{"TOKEN_NAME", c.TokenName, "Authorization"},
		{"TOKEN_PREFIX", c.TokenPrefix, "Bearer"},
		{"CLIENT_ID_HEADER", c.ClientIDHeader, "clientid"},
		{"ACCESS_TOKEN_EXPIRE_SECONDS", c.AccessTokenExpireSeconds, 2592000},
		{"PASSWORD_MAX_RETRY_COUNT", c.PasswordMaxRetryCount, 5},
		{"PASSWORD_LOCK_TIME", c.PasswordLockTime, 10},
		{"CAPTCHA_TYPE", c.CaptchaType, "math"},
		{"CAPTCHA_EXPIRE_SECONDS", c.CaptchaExpireSeconds, 120},
		{"DATABASE_NAME", c.DatabaseName, "walnut_seed_go"},
		{"DATABASE_PORT", c.DatabasePort, 3306},
		{"POOL_SIZE", c.PoolSize, 10},
		{"MAX_OVERFLOW", c.MaxOverflow, 20},
		{"POOL_RECYCLE", c.PoolRecycle, 1800},
		// ⚠️ 三端隔离的关键项
		{"REDIS_KEY_PREFIX", c.RedisKeyPrefix, "walnut-seed-go"},
		{"REDIS_DEFAULT_CACHE_TTL", c.RedisDefaultCacheTTL, 86400},
		{"API_DECRYPT_HEADER_FLAG", c.APIDecryptHeaderFlag, "encrypt-key"},
		{"SSE_PATH", c.SSEPath, "/resource/sse"},
		{"WEBSOCKET_PATH", c.WebSocketPath, "/resource/websocket"},
		{"OSS_S3_URL_PREFIX", c.OSSS3URLPrefix, "/upload"},
		{"GZIP_MIN_SIZE", c.GzipMinSize, 1000},
		{"GZIP_COMPRESS_LEVEL", c.GzipCompressLevel, 9},
		{"MAX_FILE_SIZE", c.MaxFileSize, int64(10 * 1024 * 1024)},
		{"MAX_REQUEST_SIZE", c.MaxRequestSize, int64(20 * 1024 * 1024)},
		{"OPERATION_LOG_RETENTION_DAYS", c.OperationLogRetentionD, 90},
	}
	for _, tc := range checks {
		t.Run(tc.name, func(t *testing.T) {
			if tc.got != tc.want {
				t.Errorf("%s = %v，期望 %v", tc.name, tc.got, tc.want)
			}
		})
	}

	if !c.APIDecryptEnabled {
		t.Error("API_DECRYPT_ENABLED 默认应为 true")
	}
	if !c.TokenSlidingExpire {
		t.Error("TOKEN_SLIDING_EXPIRE 默认应为 true")
	}
	if c.SnowflakeWorkerID != nil {
		t.Errorf("SNOWFLAKE_WORKER_ID 默认应为 nil（未配置），却得到 %v", *c.SnowflakeWorkerID)
	}
}

// TestCORSExposeHeadersContainsEncryptKey 是一条安全回归防线：
// 前端跨域时若读不到 encrypt-key 响应头，加密协商会静默失败。
func TestCORSExposeHeadersContainsEncryptKey(t *testing.T) {
	c := defaults()
	found := false
	for _, h := range c.CORSExposeHeaders {
		if h == "encrypt-key" {
			found = true
		}
	}
	if !found {
		t.Errorf("CORS_EXPOSE_HEADERS 必须包含 encrypt-key，当前为 %v", c.CORSExposeHeaders)
	}
}

// TestXSSExcludesNotice 验证富文本接口在 XSS 排除列表内。
func TestXSSExcludesNotice(t *testing.T) {
	c := defaults()
	found := false
	for _, u := range c.XSSExcludeURLs {
		if u == "/system/notice" {
			found = true
		}
	}
	if !found {
		t.Errorf("XSS_EXCLUDE_URLS 必须包含 /system/notice，当前为 %v", c.XSSExcludeURLs)
	}
}

// TestWhiteAPIListPath 验证白名单与 Python 端 setting.py 逐条一致，
// 且 IsWhiteListed 的匹配语义（精确 + '*' 结尾前缀）正确。
func TestWhiteAPIListPath(t *testing.T) {
	c := defaults()

	// 20 条，与 Python 端一致
	wantPaths := []string{
		"/", "/auth/login", "/auth/logout", "/auth/register", "/auth/code",
		"/auth/tenant/list", "/resource/sse/close", "/resource/websocket",
		"/common/health", "/common/health/*", "/upload/*", "/static*", "/web*",
		"/docs*", "/redoc", "/openapi.json", "/favicon.ico",
	}
	if len(c.WhiteAPIListPath) != len(wantPaths) {
		t.Errorf("白名单条数 = %d，期望 %d：%v",
			len(c.WhiteAPIListPath), len(wantPaths), c.WhiteAPIListPath)
	}
	for _, p := range wantPaths {
		found := false
		for _, got := range c.WhiteAPIListPath {
			if got == p {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("白名单缺少 %q", p)
		}
	}

	// 匹配语义
	allowed := []string{
		"/",
		"/auth/login",
		"/auth/code",
		"/common/health",
		"/common/health/ready",     // 命中 /common/health/*
		"/common/health/live",      // 命中 /common/health/*
		"/upload/2026/11/02/x.png", // 命中 /upload/*
		"/static/swagger/a.css",    // 命中 /static*
		"/web/index.html",          // 命中 /web*
		"/docs",                    // 命中 /docs*
		"/docs/oauth2-redirect",    // 命中 /docs*
		"/redoc",
		"/openapi.json",
	}
	for _, p := range allowed {
		if !c.IsWhiteListed(p) {
			t.Errorf("IsWhiteListed(%q) = false，期望 true", p)
		}
	}

	denied := []string{
		"/system/user/list",
		"/system/user",
		"/auth/logout2", // 前缀相似但不是精确匹配
		"/common/file/upload",
		"/monitor/operlog/list",
		"/resource/sse", // 白名单里只有 /resource/sse/close
		"/uploadx/y",    // /upload* 不在白名单，只有 /upload/*
	}
	for _, p := range denied {
		if c.IsWhiteListed(p) {
			t.Errorf("IsWhiteListed(%q) = true，期望 false（该路径必须受认证保护）", p)
		}
	}
}

// TestSplitList 验证逗号分隔解析（环境变量只能传字符串）。
func TestSplitList(t *testing.T) {
	cases := []struct {
		in   []string
		want []string
	}{
		{nil, nil},
		{[]string{}, []string{}},
		{[]string{"a,b,c"}, []string{"a", "b", "c"}},
		{[]string{"a", "b"}, []string{"a", "b"}},
		{[]string{" a , b "}, []string{"a", "b"}},
		{[]string{"a,,b"}, []string{"a", "b"}},
		{[]string{""}, []string{}},
	}
	for _, tc := range cases {
		got := splitList(tc.in)
		if len(got) != len(tc.want) {
			t.Errorf("splitList(%v) = %v，期望 %v", tc.in, got, tc.want)
			continue
		}
		for i := range got {
			if got[i] != tc.want[i] {
				t.Errorf("splitList(%v) = %v，期望 %v", tc.in, got, tc.want)
				break
			}
		}
	}
}

// TestValueForListKeys 验证列表键在装载阶段就被拆成切片。
//
// 这是必须的：mapstructure 的 WeaklyTypedInput **不做** string → []string 转换
// （它只对 []byte 特殊处理），留给解码阶段会得到一个单元素切片或报错。
func TestValueForListKeys(t *testing.T) {
	got := valueFor("TRUSTED_PROXY_IPS", "10.0.0.1, 10.0.0.2")
	list, ok := got.([]string)
	if !ok {
		t.Fatalf("valueFor(TRUSTED_PROXY_IPS) 返回 %T，期望 []string", got)
	}
	if len(list) != 2 || list[0] != "10.0.0.1" || list[1] != "10.0.0.2" {
		t.Errorf("拆分结果 = %v，期望 [10.0.0.1 10.0.0.2]", list)
	}

	// 非列表键保持字符串
	if s, ok := valueFor("SECRET_KEY", "abc").(string); !ok || s != "abc" {
		t.Errorf("valueFor(SECRET_KEY) = %#v，期望字符串 abc", valueFor("SECRET_KEY", "abc"))
	}

	// 空列表键 → 空切片（而非 nil，避免 mapstructure 解码歧义）
	if l, ok := valueFor("XSS_EXCLUDE_URLS", "").([]string); !ok || len(l) != 0 {
		t.Errorf("valueFor(XSS_EXCLUDE_URLS, \"\") = %#v，期望空切片", valueFor("XSS_EXCLUDE_URLS", ""))
	}

	// 空的指针型键 → nil（表示「未设置，保留默认值」）
	// SNOWFLAKE_WORKER_ID 是 *int，0 是合法 worker id，不能与「未配置」混淆
	if v := valueFor("SNOWFLAKE_WORKER_ID", ""); v != nil {
		t.Errorf("valueFor(SNOWFLAKE_WORKER_ID, \"\") = %#v，期望 nil", v)
	}
}

// TestEnvVarOverridesDefault 验证真实环境变量优先级最高。
func TestEnvVarOverridesDefault(t *testing.T) {
	t.Setenv("SERVER_PORT", "9999")
	t.Setenv("DATABASE_NAME", "walnut_seed_go_test")
	t.Setenv("TRUSTED_PROXY_IPS", "10.0.0.1,10.0.0.2")
	t.Setenv("XSS_ENABLED", "false")
	t.Setenv("SNOWFLAKE_WORKER_ID", "42")
	t.Setenv("MAX_FILE_SIZE", "1048576")

	c, err := Load("")
	if err != nil {
		t.Fatalf("Load 失败: %v", err)
	}

	if c.ServerPort != 9999 {
		t.Errorf("SERVER_PORT = %d，期望 9999（环境变量应覆盖默认值）", c.ServerPort)
	}
	if c.DatabaseName != "walnut_seed_go_test" {
		t.Errorf("DATABASE_NAME = %q", c.DatabaseName)
	}
	// 字符串 → bool 的弱类型转换
	if c.XSSEnabled {
		t.Error("XSS_ENABLED=false 应把 XSSEnabled 置为 false")
	}
	// 字符串 → *int
	if c.SnowflakeWorkerID == nil {
		t.Fatal("SNOWFLAKE_WORKER_ID=42 应被解析")
	}
	if *c.SnowflakeWorkerID != 42 {
		t.Errorf("SNOWFLAKE_WORKER_ID = %d，期望 42", *c.SnowflakeWorkerID)
	}
	// 字符串 → int64
	if c.MaxFileSize != 1048576 {
		t.Errorf("MAX_FILE_SIZE = %d，期望 1048576", c.MaxFileSize)
	}
	// 逗号分隔 → 切片
	if len(c.TrustedProxyIPs) != 2 || c.TrustedProxyIPs[0] != "10.0.0.1" {
		t.Errorf("TRUSTED_PROXY_IPS = %v，期望 [10.0.0.1 10.0.0.2]", c.TrustedProxyIPs)
	}
}

// TestEnvVarNotSetKeepsPointerNil 验证未设置时 *int 保持 nil（而非 0）。
func TestEnvVarNotSetKeepsPointerNil(t *testing.T) {
	// t.Setenv 无法「取消设置」，故用空值模拟 .env 里的 SNOWFLAKE_WORKER_ID=
	t.Setenv("SNOWFLAKE_WORKER_ID", "")
	c, err := Load("")
	if err != nil {
		t.Fatalf("Load 失败: %v", err)
	}
	if c.SnowflakeWorkerID != nil {
		t.Errorf("空的 SNOWFLAKE_WORKER_ID 应保持 nil，却得到 %d", *c.SnowflakeWorkerID)
	}
}

func TestLoadRejectsBadEnvironment(t *testing.T) {
	t.Setenv("ENVIRONMENT", "staging")
	if _, err := Load(""); err == nil {
		t.Error("ENVIRONMENT=staging 应当被拒绝（只接受 dev/prod）")
	} else if !strings.Contains(err.Error(), "ENVIRONMENT") {
		t.Errorf("错误信息应指明 ENVIRONMENT，实际为: %v", err)
	}
}

func TestLoadRejectsBadDatabaseType(t *testing.T) {
	t.Setenv("DATABASE_TYPE", "postgres")
	if _, err := Load(""); err == nil {
		t.Error("DATABASE_TYPE=postgres 应当被拒绝（仅支持 mysql）")
	}
}

func TestLoadRejectsBadWorkerID(t *testing.T) {
	t.Setenv("SNOWFLAKE_WORKER_ID", "1024")
	if _, err := Load(""); err == nil {
		t.Error("SNOWFLAKE_WORKER_ID=1024 超出 0-1023，应当被拒绝")
	}
}

func TestAddr(t *testing.T) {
	c := defaults()
	if got := c.Addr(); got != "0.0.0.0:8011" {
		t.Errorf("Addr() = %q，期望 0.0.0.0:8011", got)
	}
}

// ---- 启动期安全校验 ----

func TestValidateSecurityProdRejectsWeakSecret(t *testing.T) {
	c := defaults() // SECRET_KEY 含 "change-me"
	c.Environment = EnvProd

	if _, err := c.ValidateSecurity(); err == nil {
		t.Fatal("prod 环境下弱 SECRET_KEY 必须拒绝启动（fail-fast）")
	} else if !strings.Contains(err.Error(), "SECRET_KEY") {
		t.Errorf("错误信息应指明 SECRET_KEY，实际为: %v", err)
	}
}

func TestValidateSecurityDevOnlyWarns(t *testing.T) {
	c := defaults()
	c.Environment = EnvDev

	res, err := c.ValidateSecurity()
	if err != nil {
		t.Fatalf("dev 环境下弱 SECRET_KEY 只应告警，不应报错: %v", err)
	}
	if len(res.Warnings) == 0 {
		t.Error("dev 环境下应当产生弱密钥告警")
	}
}

func TestValidateSecurityProdAcceptsStrongSecret(t *testing.T) {
	c := defaults()
	c.Environment = EnvProd
	c.SecretKey = strings.Repeat("a", 64) // ≥32 字节且不含 change-me

	if _, err := c.ValidateSecurity(); err != nil {
		t.Fatalf("强 SECRET_KEY 在 prod 下不应报错: %v", err)
	}
}

func TestValidateSecurityTooShortSecret(t *testing.T) {
	c := defaults()
	c.Environment = EnvProd
	c.SecretKey = "short"

	if _, err := c.ValidateSecurity(); err == nil {
		t.Error("SECRET_KEY 长度不足 32 字节，prod 下必须拒绝启动")
	}
}

// TestValidateSecurityEmptyRSADisablesEncrypt 验证降级语义：
// 密钥为空 → 停用加解密 + 告警，**而不是带着假密钥运行**。
func TestValidateSecurityEmptyRSADisablesEncrypt(t *testing.T) {
	c := defaults()
	c.SecretKey = strings.Repeat("a", 64)
	c.APIDecryptPublicKey = ""
	c.APIDecryptPrivateKey = ""

	res, err := c.ValidateSecurity()
	if err != nil {
		t.Fatalf("密钥为空应降级而非报错: %v", err)
	}
	if c.APIDecryptEnabled {
		t.Error("RSA 密钥为空时 APIDecryptEnabled 必须被置为 false")
	}
	if c.APIDecryptDisabledReason == "" {
		t.Error("应记录停用原因，便于启动日志说明")
	}
	found := false
	for _, w := range res.Warnings {
		if w.Code == "API_DECRYPT_DISABLED" {
			found = true
		}
	}
	if !found {
		t.Errorf("应产生 API_DECRYPT_DISABLED 告警，实际为 %+v", res.Warnings)
	}
}

// TestValidateSecurityInvalidRSADisablesEncrypt 验证无效密钥（Base64 DER 不可解析）同样降级。
func TestValidateSecurityInvalidRSADisablesEncrypt(t *testing.T) {
	c := defaults()
	c.SecretKey = strings.Repeat("a", 64)
	c.APIDecryptPublicKey = "这不是合法的base64der"
	c.APIDecryptPrivateKey = "也不是"

	if _, err := c.ValidateSecurity(); err != nil {
		t.Fatalf("无效密钥应降级而非报错: %v", err)
	}
	if c.APIDecryptEnabled {
		t.Error("RSA 密钥无效时 APIDecryptEnabled 必须被置为 false")
	}
}

// TestValidateSecurityRejectsKnownBadRSAInProd 验证 RuoYi 出厂密钥黑名单。
func TestValidateSecurityRejectsKnownBadRSAInProd(t *testing.T) {
	c := defaults()
	c.Environment = EnvProd
	c.SecretKey = strings.Repeat("a", 64)
	c.APIDecryptPublicKey = knownBadRSAPublicPrefix + "尾随部分"

	if _, err := c.ValidateSecurity(); err == nil {
		t.Error("prod 下命中 RuoYi 出厂公开密钥必须拒绝启动")
	} else if !strings.Contains(err.Error(), "RuoYi") {
		t.Errorf("错误信息应指明是 RuoYi 出厂密钥，实际为: %v", err)
	}
}

// TestValidateSecurityAcceptsRealDevKeys 用 .env.dev 里那对真实的 1024 位开发密钥验证：
// 应当通过校验且**不**停用加解密。
//
// 这条用例同时守住了「Go 1.24+ 是否还接受 1024 位 RSA」这个已实测的问题
// （见实施计划 §9.2）——若未来 Go 版本收紧到拒绝 1024 位，此用例会失败。
func TestValidateSecurityAcceptsRealDevKeys(t *testing.T) {
	const (
		pub  = "MIGfMA0GCSqGSIb3DQEBAQUAA4GNADCBiQKBgQCzWeB2rv0CLeABVtm9K7Ak/uOCSzeSeVYNLzaIDM5Nb84NQ22fYjSFLDnQVZCfBGFKAF5Qhy/yIziDKfewdA48+MzHm/ln8gCRiPane78G8ja4wDTx1atmkGUTgrInvUasjKFKOA2DK/ejqg4bYPiGKdarHpNoaDFd7NntkgmyiwIDAQAB"
		priv = "MIICdQIBADANBgkqhkiG9w0BAQEFAASCAl8wggJbAgEAAoGBAL4RtvHd74nL6iqoWdb7GHYfxbDPqeaXgdSEfCrhBBcoP4I0xPgQSoUBsYX0j+4mxOtWCK2ruMPnrFG8ZPSe0BSDmfRTqv+ssLH3TY1Mk3gPsA2RhTSoeoFK8P0ZB7AFu1qGUvojy1VZhyfmZvuAxGb7SlbXf7KXFmYQ+/LwOYGxAgMBAAECgYAmfb/my2MvLLSHXwCSoNcbVOPcIrHtmNEaEJp0tIz3g5cY70bUVbhBp04kKprbR86pOgsX4KbSRaCIDPcE8wSASUukLBx9Zd5xoo7UwunOxGtpWTgmvDmSyRXWk7OlIPAHf3DXCLhH5ObczpVtiweB75lSM/8HeaFFXRdn1N0SgQJBAPISegTGGfp1iUE5kszIRLjPMLQPOMIi97v1jnfn06f8wHy+akPFESZiEz+3cscOX/vzFPHkueYz679HthXCPx0CQQDJAUbtgwkkWKRLSWIB54+mnVFvGbUkNte+CN0V5OX/0x3UBz5ekl4tnRnK58cOyuruJk5ynuqi0/36Hk7TReSlAkAQsgWIXsFb7M9SmE3vocBbnq6VPJeFjDPZ2s3Ap1SJuXztHZTmp6jPJOR++IAKCovELLe9FzFbFymjP6bnSUZhAkAEQperPwS5lXtd8R83pR3BTUJFlYjmTJdD+tIcva42PtY6ATFMNdPzKYo7Z7ArWxehloRBH6cfbljKrUvEZI3VAkBKr+lLNtm+sDAR+9V0BaDWiL32h9sLq0sCBgopnS4tPNcLzv70ac2UGHJ4ixXbwMNOOdSEDCzifwvKofcU6rzV"
	)

	c := defaults()
	c.SecretKey = strings.Repeat("a", 64)
	c.APIDecryptPublicKey = pub
	c.APIDecryptPrivateKey = priv

	if _, err := c.ValidateSecurity(); err != nil {
		t.Fatalf("真实的 1024 位开发密钥应通过校验: %v", err)
	}
	if !c.APIDecryptEnabled {
		t.Errorf("1024 位密钥有效，不应被停用（原因: %s）", c.APIDecryptDisabledReason)
	}
}

// TestParseDotEnv 验证 .env 解析：注释、空行、export 前缀、引号、值内含 '='。
func TestParseDotEnv(t *testing.T) {
	dir := t.TempDir()
	path := dir + "/test.env"
	content := `# 注释行
ENVIRONMENT=dev

  # 缩进的注释
SERVER_PORT=8011
export DEBUG=true
QUOTED="带引号的值"
SINGLE='单引号值'
RSA_KEY=MIGfMA0GCSqGSIb3DQEBAQUAA4GNADCBiQKBgQ==
EMPTY=
TRUSTED_PROXY_IPS=a,b,c
NOT_A_LIST_KEY=a,b,c
`
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("写测试文件失败: %v", err)
	}

	got, err := parseDotEnv(path)
	if err != nil {
		t.Fatalf("parseDotEnv 失败: %v", err)
	}

	if got["SERVER_PORT"] != "8011" {
		t.Errorf("SERVER_PORT = %#v", got["SERVER_PORT"])
	}
	if got["DEBUG"] != "true" {
		t.Errorf("export 前缀未被剥离: DEBUG = %#v", got["DEBUG"])
	}
	if got["QUOTED"] != "带引号的值" {
		t.Errorf("双引号未被剥离: QUOTED = %#v", got["QUOTED"])
	}
	if got["SINGLE"] != "单引号值" {
		t.Errorf("单引号未被剥离: SINGLE = %#v", got["SINGLE"])
	}
	// RSA 密钥里的 '=' 不能被当成键值分隔符切错（只按第一个 '=' 分割）
	if got["RSA_KEY"] != "MIGfMA0GCSqGSIb3DQEBAQUAA4GNADCBiQKBgQ==" {
		t.Errorf("含 '=' 的值被切错: RSA_KEY = %#v", got["RSA_KEY"])
	}
	if got["EMPTY"] != "" {
		t.Errorf("EMPTY = %#v，期望空串", got["EMPTY"])
	}
	// listKeys 中的键在装载阶段就拆分成切片
	if list, ok := got["TRUSTED_PROXY_IPS"].([]string); !ok || len(list) != 3 {
		t.Errorf("TRUSTED_PROXY_IPS = %#v，期望 [a b c]", got["TRUSTED_PROXY_IPS"])
	}
	// 不在 listKeys 中的键保持原始字符串——这是刻意的：
	// 只有目标类型为 []string 的配置项才该拆分，否则 SECRET_KEY 里含逗号就会被切碎
	if s, ok := got["NOT_A_LIST_KEY"].(string); !ok || s != "a,b,c" {
		t.Errorf("NOT_A_LIST_KEY = %#v，期望原样字符串 \"a,b,c\"", got["NOT_A_LIST_KEY"])
	}
	if _, exists := got["# 注释行"]; exists {
		t.Error("注释行被当成了配置项")
	}
}

func TestParseDotEnvMissingFileIsNotError(t *testing.T) {
	got, err := parseDotEnv("/不存在的路径/.env.dev")
	if err != nil {
		t.Errorf("文件不存在应是合法情况（Docker 镜像不打包 env/），却报错: %v", err)
	}
	if got != nil {
		t.Errorf("文件不存在应返回 nil map，却得到 %v", got)
	}
}

func TestUnquote(t *testing.T) {
	cases := map[string]string{
		`"abc"`: "abc",
		`'abc'`: "abc",
		`abc`:   "abc",
		`"abc'`: `"abc'`, // 引号不配对，不剥离
		`""`:    "",
		`"`:     `"`,
		``:      "",
		`"a"b"`: `a"b`,
	}
	for in, want := range cases {
		if got := unquote(in); got != want {
			t.Errorf("unquote(%q) = %q，期望 %q", in, got, want)
		}
	}
}
