// Package config 是 Go 后端配置的唯一事实源，对齐 Python 端 app/config/setting.py。
//
// 加载优先级（与 Python 端一致，高 → 低）：
//
//	真实环境变量（compose / shell 注入）
//	  > env/.env.{ENVIRONMENT} 文件（仅本地开发；镜像不打包 env/）
//	  > 本文件中的代码默认值（兜底）
//
// 字段上的 koanf tag 即环境变量名，必须与 Python 端逐字一致，
// 否则三端在 compose 里无法共用同一份 docker/.env。
package config

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/go-viper/mapstructure/v2"
	"github.com/knadh/koanf/providers/confmap"
	"github.com/knadh/koanf/v2"
)

// Environment 运行环境。
type Environment string

const (
	EnvDev  Environment = "dev"
	EnvProd Environment = "prod"
)

func (e Environment) IsProd() bool { return e == EnvProd }

// Config 全量配置。默认值即 Python 端 setting.py 的默认值，改动会同时影响所有环境。
type Config struct {
	// ---- 环境与服务 ----
	Environment Environment `koanf:"ENVIRONMENT"`
	ServerHost  string      `koanf:"SERVER_HOST"`
	ServerPort  int         `koanf:"SERVER_PORT"`
	RootPath    string      `koanf:"ROOT_PATH"`

	// ---- 文档（Go 侧由 swaggo 或静态 OpenAPI 提供，键名保持对齐）----
	Debug       bool   `koanf:"DEBUG"`
	Title       string `koanf:"TITLE"`
	Version     string `koanf:"VERSION"`
	Description string `koanf:"DESCRIPTION"`
	Summary     string `koanf:"SUMMARY"`
	DocsURL     string `koanf:"DOCS_URL"`
	RedocURL    string `koanf:"REDOC_URL"`

	// ---- 日志 ----
	LoggerLevel            string   `koanf:"LOGGER_LEVEL"`
	LogDir                 string   `koanf:"LOG_DIR"`
	LogFilename            string   `koanf:"LOG_FILENAME"`
	LogMaxSizeMB           int      `koanf:"LOG_MAX_SIZE_MB"`
	LogMaxBackups          int      `koanf:"LOG_MAX_BACKUPS"`
	LogMaxAgeDays          int      `koanf:"LOG_MAX_AGE_DAYS"`
	OperationRecordMethod  []string `koanf:"OPERATION_RECORD_METHOD"`
	OperationLogRetentionD int      `koanf:"OPERATION_LOG_RETENTION_DAYS"`

	// ---- CORS ----
	ProdCORSOrigins  string   `koanf:"PROD_CORS_ORIGINS"`
	AllowMethods     []string `koanf:"ALLOW_METHODS"`
	AllowHeaders     []string `koanf:"ALLOW_HEADERS"`
	AllowCredentials bool     `koanf:"ALLOW_CREDENTIALS"`
	CORSMaxAge       int      `koanf:"CORS_MAX_AGE"`
	// CORSExposeHeaders 必须包含 encrypt-key，否则前端跨域时读不到加密协商头
	CORSExposeHeaders []string `koanf:"CORS_EXPOSE_HEADERS"`
	HTTPSRedirect     bool     `koanf:"HTTPS_REDIRECT"`
	AllowedHosts      []string `koanf:"ALLOWED_HOSTS"`

	// ---- JWT / 认证 ----
	SecretKey                string   `koanf:"SECRET_KEY"`
	Algorithm                string   `koanf:"ALGORITHM"`
	TokenName                string   `koanf:"TOKEN_NAME"`
	TokenPrefix              string   `koanf:"TOKEN_PREFIX"`
	AccessTokenExpireSeconds int      `koanf:"ACCESS_TOKEN_EXPIRE_SECONDS"`
	TokenSlidingExpire       bool     `koanf:"TOKEN_SLIDING_EXPIRE"`
	ClientIDHeader           string   `koanf:"CLIENT_ID_HEADER"`
	PasswordMaxRetryCount    int      `koanf:"PASSWORD_MAX_RETRY_COUNT"`
	PasswordLockTime         int      `koanf:"PASSWORD_LOCK_TIME"`
	UserImportInitPassword   string   `koanf:"USER_IMPORT_INIT_PASSWORD"`
	WhiteAPIListPath         []string `koanf:"WHITE_API_LIST_PATH"`
	TrustedProxyIPs          []string `koanf:"TRUSTED_PROXY_IPS"`
	SecurityExcludes         []string `koanf:"SECURITY_EXCLUDES"`

	// ---- 验证码 ----
	CaptchaEnable        bool   `koanf:"CAPTCHA_ENABLE"`
	CaptchaType          string `koanf:"CAPTCHA_TYPE"`
	CaptchaNumberLength  int    `koanf:"CAPTCHA_NUMBER_LENGTH"`
	CaptchaCharLength    int    `koanf:"CAPTCHA_CHAR_LENGTH"`
	CaptchaExpireSeconds int    `koanf:"CAPTCHA_EXPIRE_SECONDS"`

	// ---- 雪花 ID ----
	SnowflakeWorkerID *int `koanf:"SNOWFLAKE_WORKER_ID"`

	// ---- 数据库 ----
	DatabaseType        string `koanf:"DATABASE_TYPE"`
	DatabaseHost        string `koanf:"DATABASE_HOST"`
	DatabasePort        int    `koanf:"DATABASE_PORT"`
	DatabaseUser        string `koanf:"DATABASE_USER"`
	DatabasePassword    string `koanf:"DATABASE_PASSWORD"`
	DatabaseName        string `koanf:"DATABASE_NAME"`
	DatabaseAutoMigrate bool   `koanf:"DATABASE_AUTO_MIGRATE"`
	DatabaseEcho        bool   `koanf:"DATABASE_ECHO"`
	PoolSize            int    `koanf:"POOL_SIZE"`
	MaxOverflow         int    `koanf:"MAX_OVERFLOW"`
	PoolTimeout         int    `koanf:"POOL_TIMEOUT"`
	PoolRecycle         int    `koanf:"POOL_RECYCLE"`
	PoolPrePing         bool   `koanf:"POOL_PRE_PING"`

	// 迁移脚本目录与种子 SQL 路径。
	// 本机开发用相对路径（CWD = walnut-backend-go/），容器内两者都在 WORKDIR
	// /walnut-backend 下，故同一套默认值两边都成立（见 Dockerfile 的 COPY）。
	MigrationsDir string `koanf:"MIGRATIONS_DIR"`
	SeedFile      string `koanf:"SEED_FILE"`

	// HealthProbeTimeout /common/health/ready 对**单个**依赖的探测超时（秒）。
	//
	// ⚠️ 这是一处与 Python 端的**有意偏差**（见实施计划 R19）：
	// Python 的 _check_database 无显式超时，靠 SQLAlchemy 的 POOL_TIMEOUT=30 兜底
	// （实测 MySQL 不可达时耗时约 4s 才返回 503）。故依赖「慢但存活」时两端可能
	// 给出相反结论（Python 200 / Go 503）。
	//
	// 保留短超时的理由：compose 的 healthcheck 是 timeout: 5s / start_interval: 2s，
	// 探针必须在 5s 内出结论，否则被判「探测失败」而非「未就绪」，语义更糟。
	// 提为配置项后可按部署环境调优。
	HealthProbeTimeout int `koanf:"HEALTH_PROBE_TIMEOUT"`

	// ---- Redis ----
	RedisHost     string `koanf:"REDIS_HOST"`
	RedisPort     int    `koanf:"REDIS_PORT"`
	RedisDBName   int    `koanf:"REDIS_DB_NAME"`
	RedisUser     string `koanf:"REDIS_USER"`
	RedisPassword string `koanf:"REDIS_PASSWORD"`
	// RedisKeyPrefix 必须与另两端区分（walnut-seed-python / walnut-seed-java），
	// 否则共用 Redis 实例时登录态、验证码、限流计数会互相串键
	RedisKeyPrefix           string `koanf:"REDIS_KEY_PREFIX"`
	RedisHealthCheckInterval int    `koanf:"REDIS_HEALTH_CHECK_INTERVAL"`
	RedisDefaultCacheTTL     int    `koanf:"REDIS_DEFAULT_CACHE_TTL"`

	// ---- XSS ----
	XSSEnabled     bool     `koanf:"XSS_ENABLED"`
	XSSExcludeURLs []string `koanf:"XSS_EXCLUDE_URLS"`

	// ---- 接口传输加解密（RSA + AES）----
	APIDecryptEnabled    bool   `koanf:"API_DECRYPT_ENABLED"`
	APIDecryptHeaderFlag string `koanf:"API_DECRYPT_HEADER_FLAG"`
	APIDecryptPublicKey  string `koanf:"API_DECRYPT_PUBLIC_KEY"`
	APIDecryptPrivateKey string `koanf:"API_DECRYPT_PRIVATE_KEY"`

	// ---- 字段入库加密（与上面是两套互不相干的体系，勿混）----
	FieldEncryptEnabled    bool   `koanf:"FIELD_ENCRYPT_ENABLED"`
	FieldEncryptAlgorithm  string `koanf:"FIELD_ENCRYPT_ALGORITHM"`
	FieldEncryptEncode     string `koanf:"FIELD_ENCRYPT_ENCODE"`
	FieldEncryptPassword   string `koanf:"FIELD_ENCRYPT_PASSWORD"`
	FieldEncryptPublicKey  string `koanf:"FIELD_ENCRYPT_PUBLIC_KEY"`
	FieldEncryptPrivateKey string `koanf:"FIELD_ENCRYPT_PRIVATE_KEY"`

	// ---- SSE / WebSocket ----
	SSEEnabled             bool   `koanf:"SSE_ENABLED"`
	SSEPath                string `koanf:"SSE_PATH"`
	WebSocketEnabled       bool   `koanf:"WEBSOCKET_ENABLED"`
	WebSocketPath          string `koanf:"WEBSOCKET_PATH"`
	WebSocketAllowedOrigin string `koanf:"WEBSOCKET_ALLOWED_ORIGINS"`

	// ---- 对象存储 ----
	OSSType            string `koanf:"OSS_TYPE"`
	OSSS3Endpoint      string `koanf:"OSS_S3_ENDPOINT"`
	OSSS3AccessKey     string `koanf:"OSS_S3_ACCESS_KEY"`
	OSSS3SecretKey     string `koanf:"OSS_S3_SECRET_KEY"`
	OSSS3BucketName    string `koanf:"OSS_S3_BUCKET_NAME"`
	OSSS3Secure        bool   `koanf:"OSS_S3_SECURE"`
	OSSS3URLPrefix     string `koanf:"OSS_S3_URL_PREFIX"`
	OSSAliyunEndpoint  string `koanf:"OSS_ALIYUN_ENDPOINT"`
	OSSAliyunAccessKey string `koanf:"OSS_ALIYUN_ACCESS_KEY_ID"`
	OSSAliyunSecretKey string `koanf:"OSS_ALIYUN_ACCESS_KEY_SECRET"`
	OSSAliyunBucket    string `koanf:"OSS_ALIYUN_BUCKET_NAME"`
	OSSAliyunURLPrefix string `koanf:"OSS_ALIYUN_URL_PREFIX"`

	// ---- 体积与压缩 ----
	GzipMinSize       int      `koanf:"GZIP_MIN_SIZE"`
	GzipCompressLevel int      `koanf:"GZIP_COMPRESS_LEVEL"`
	MaxFileSize       int64    `koanf:"MAX_FILE_SIZE"`
	MaxRequestSize    int64    `koanf:"MAX_REQUEST_SIZE"`
	AllowedExtensions []string `koanf:"ALLOWED_EXTENSIONS"`
	StaticURL         string   `koanf:"STATIC_URL"`

	// 运行时状态：密钥校验可能把加解密降级关闭（对应 Python 端就地修改 settings）
	// 该字段不经配置加载，由 validate.go 写入
	APIDecryptDisabledReason string `koanf:"-"`
}

// defaults 返回代码默认值，与 setting.py 逐条对齐。
func defaults() *Config {
	return &Config{
		Environment: EnvDev,
		ServerHost:  "0.0.0.0",
		ServerPort:  8011,
		RootPath:    "",

		Debug:       true,
		Title:       "WalnutSeed 接口文档",
		Version:     "1.0.0",
		Description: "WalnutSeed 现代化全栈应用脚手架接口文档",
		Summary:     "接口汇总",
		DocsURL:     "/docs",
		RedocURL:    "/redoc",

		LoggerLevel:            "DEBUG",
		LogDir:                 "", // 空 → 由 bootstrap 解析为 <repo>/data/logs
		LogFilename:            "walnut-seed-go.log",
		LogMaxSizeMB:           50,
		LogMaxBackups:          10,
		LogMaxAgeDays:          30,
		OperationRecordMethod:  []string{"POST", "PUT", "PATCH", "DELETE", "HEAD", "OPTIONS"},
		OperationLogRetentionD: 90,

		ProdCORSOrigins:  "",
		AllowMethods:     []string{"*"},
		AllowHeaders:     []string{"*"},
		AllowCredentials: true,
		CORSMaxAge:       1800,
		CORSExposeHeaders: []string{
			"X-Request-ID", "encrypt-key", "Content-Disposition", "download-filename",
		},
		HTTPSRedirect: false,
		AllowedHosts:  []string{"*"},

		// 与 Python 端相同的开发占位值：含 change-me 且 prod 会拒绝启动
		SecretKey:                "walnut-seed-change-me-32-bytes-min-jwt-secret-0123456789",
		Algorithm:                "HS256",
		TokenName:                "Authorization",
		TokenPrefix:              "Bearer",
		AccessTokenExpireSeconds: 60 * 60 * 24 * 30,
		TokenSlidingExpire:       true,
		ClientIDHeader:           "clientid",
		PasswordMaxRetryCount:    5,
		PasswordLockTime:         10,
		UserImportInitPassword:   "Walnut@123",
		WhiteAPIListPath: []string{
			"/",
			"/auth/login",
			"/auth/logout",
			"/auth/register",
			"/auth/code",
			"/auth/tenant/list",
			"/resource/sse/close",
			"/resource/websocket",
			"/common/health",
			"/common/health/*",
			"/upload/*",
			"/static*",
			"/web*",
			"/docs*",
			"/redoc",
			"/openapi.json",
			"/favicon.ico",
		},
		TrustedProxyIPs: []string{},
		SecurityExcludes: []string{
			"/*.html", "/**/*.html", "/**/*.css", "/**/*.js", "/favicon.ico",
			"/error", "/*/api-docs", "/*/api-docs/**",
		},

		CaptchaEnable:        true,
		CaptchaType:          "math",
		CaptchaNumberLength:  1,
		CaptchaCharLength:    4,
		CaptchaExpireSeconds: 120,

		SnowflakeWorkerID: nil,

		DatabaseType:        "mysql",
		DatabaseHost:        "localhost",
		DatabasePort:        3306,
		DatabaseUser:        "root",
		DatabasePassword:    "",
		DatabaseName:        "walnut_seed_go",
		DatabaseAutoMigrate: false,
		DatabaseEcho:        false,
		MigrationsDir:       "migrations",
		SeedFile:            "seed/mysql_data.sql",
		// 2 秒：必须显著小于 compose healthcheck 的 timeout: 5s，
		// 否则探针会把「依赖慢」误判成「探测失败」
		HealthProbeTimeout: 2,
		PoolSize:           10,
		MaxOverflow:        20,
		PoolTimeout:        30,
		PoolRecycle:        1800,
		PoolPrePing:        true,

		RedisHost:                "localhost",
		RedisPort:                6379,
		RedisDBName:              0,
		RedisUser:                "",
		RedisPassword:            "",
		RedisKeyPrefix:           "walnut-seed-go",
		RedisHealthCheckInterval: 20,
		RedisDefaultCacheTTL:     86400,

		XSSEnabled:     true,
		XSSExcludeURLs: []string{"/system/notice"},

		APIDecryptEnabled:    true,
		APIDecryptHeaderFlag: "encrypt-key",
		APIDecryptPublicKey:  "",
		APIDecryptPrivateKey: "",

		FieldEncryptEnabled:   false,
		FieldEncryptAlgorithm: "BASE64",
		FieldEncryptEncode:    "BASE64",

		SSEEnabled:             true,
		SSEPath:                "/resource/sse",
		WebSocketEnabled:       false,
		WebSocketPath:          "/resource/websocket",
		WebSocketAllowedOrigin: "*",

		OSSType:         "s3",
		OSSS3Endpoint:   "localhost:8333",
		OSSS3AccessKey:  "walnut",
		OSSS3SecretKey:  "walnut123",
		OSSS3BucketName: "walnut-seed",
		OSSS3Secure:     false,
		OSSS3URLPrefix:  "/upload",

		GzipMinSize:       1000,
		GzipCompressLevel: 9,
		MaxFileSize:       10 * 1024 * 1024,
		MaxRequestSize:    20 * 1024 * 1024,
		AllowedExtensions: []string{
			"jpg", "jpeg", "png", "gif", "bmp", "webp", "svg", "ico",
			"pdf", "doc", "docx", "xls", "xlsx", "ppt", "pptx",
			"txt", "md", "csv", "json", "xml", "zip", "rar", "7z", "gz", "tar",
			"mp3", "mp4", "avi", "mov", "wav",
		},
		StaticURL: "/static",
	}
}

// Load 按三级优先级装载配置。envFileDir 为空时跳过文件层（Docker 场景）。
func Load(environment string) (*Config, error) {
	cfg := defaults()

	if environment != "" {
		cfg.Environment = Environment(environment)
	}
	// ENVIRONMENT 环境变量优先于 --env 旗标之外的默认值，但低于显式旗标；
	// 旗标已在 cmd 层解析并通过 environment 传入，故此处仅在未传旗标时读环境变量
	if environment == "" {
		if v := strings.TrimSpace(os.Getenv("ENVIRONMENT")); v != "" {
			cfg.Environment = Environment(v)
		}
	}

	k := koanf.New(".")

	// 第 2 层：env/.env.{ENVIRONMENT}
	envFile := filepath.Join("env", ".env."+string(cfg.Environment))
	if fileMap, err := parseDotEnv(envFile); err != nil {
		return nil, fmt.Errorf("解析 %s 失败: %w", envFile, err)
	} else if len(fileMap) > 0 {
		// confmap.Provider 已实现 koanf 的 Reader 接口，直接给出结构化 map，
		// 因此 parser 传 nil（传了也会被忽略，反而误导读者）
		if err := k.Load(confmap.Provider(fileMap, "."), nil); err != nil {
			return nil, fmt.Errorf("装载 %s 失败: %w", envFile, err)
		}
	}

	// 第 1 层（最高）：真实环境变量。
	// 不经 koanf 的 env.Provider，而是自己读 os.Environ()——因为需要按 listKeys
	// 把逗号分隔的字符串拆成切片，而 mapstructure 的 WeaklyTypedInput
	// **不会**做 string → []string 的拆分（它只对 []byte 特殊处理）。
	if err := k.Load(confmap.Provider(envMap(), "."), nil); err != nil {
		return nil, fmt.Errorf("装载环境变量失败: %w", err)
	}

	// WeaklyTypedInput 必须开启：环境变量与 .env 文件里的值**恒为字符串**，
	// 而结构体字段有 int / bool / []string / *int 等多种类型。
	// 不开启时 mapstructure 会直接报解码错误，而不是像 pydantic 那样做类型强转。
	if err := k.UnmarshalWithConf("", cfg, koanf.UnmarshalConf{
		Tag: "koanf",
		DecoderConfig: &mapstructure.DecoderConfig{
			TagName:          "koanf",
			Result:           cfg,
			WeaklyTypedInput: true,
			// 未识别的键（PATH、USERNAME 等无关环境变量）静默忽略而非报错
			ErrorUnused: false,
			// 空字符串不应把已有的默认值覆盖成零值——例如 .env 里写了
			// LOG_DIR= 表示「用默认值」，而非「设为空」。交由 normalize 兜底处理。
			ZeroFields: false,
		},
	}); err != nil {
		return nil, fmt.Errorf("映射配置到结构体失败: %w", err)
	}

	if err := cfg.normalize(); err != nil {
		return nil, err
	}
	return cfg, nil
}

// parseDotEnv 读取 KEY=VALUE 文件。手写而非引入 dotenv 解析器：
// 格式极简，且能精确控制注释、引号、行内 # 的处理，避免解析器差异导致三端配置不一致。
func parseDotEnv(path string) (map[string]any, error) {
	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil // 文件不存在是合法的（Docker 镜像不打包 env/）
		}
		return nil, err
	}
	defer f.Close()

	out := make(map[string]any)
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 64*1024), 8*1024*1024) // RSA 密钥行可能很长
	lineNo := 0
	for sc.Scan() {
		lineNo++
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		line = strings.TrimPrefix(line, "export ")
		k, v, ok := strings.Cut(line, "=")
		if !ok {
			return nil, fmt.Errorf("第 %d 行缺少 '=': %s", lineNo, line)
		}
		key := strings.TrimSpace(k)
		if key == "" {
			return nil, fmt.Errorf("第 %d 行键名为空", lineNo)
		}
		// valueFor 返回 nil 表示「该键视为未设置」，不能写入 map，
		// 否则 mapstructure 会把 nil 解码进字段、抹掉代码默认值
		if val := valueFor(key, strings.TrimSpace(unquote(strings.TrimSpace(v)))); val != nil {
			out[key] = val
		}
	}
	if err := sc.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

// listKeys 是目标类型为 []string 的配置项。
//
// 环境变量与 .env 文件只能表达字符串，故这些键的值必须在**装载阶段**按逗号拆分；
// 交给 mapstructure 是行不通的——它的 WeaklyTypedInput 不做 string → []string 转换。
var listKeys = map[string]struct{}{
	"WHITE_API_LIST_PATH":     {},
	"TRUSTED_PROXY_IPS":       {},
	"SECURITY_EXCLUDES":       {},
	"ALLOW_METHODS":           {},
	"ALLOW_HEADERS":           {},
	"CORS_EXPOSE_HEADERS":     {},
	"ALLOWED_HOSTS":           {},
	"XSS_EXCLUDE_URLS":        {},
	"ALLOWED_EXTENSIONS":      {},
	"OPERATION_RECORD_METHOD": {},
}

// emptyMeansUnset 是「空值表示未设置，应保留代码默认值」的键。
//
// 仅含指针类型字段：SNOWFLAKE_WORKER_ID 是 *int，把空串解码进去会失败或产生
// 语义错误的 0（0 是合法 worker id，与「未配置」含义完全不同）。
// 普通字符串字段的空值仍按 pydantic 语义处理为「显式设为空」。
var emptyMeansUnset = map[string]struct{}{
	"SNOWFLAKE_WORKER_ID": {},
}

// valueFor 把原始字符串转为适合 mapstructure 解码的值。
func valueFor(key, raw string) any {
	if raw == "" {
		if _, skip := emptyMeansUnset[key]; skip {
			return nil // nil 由调用方过滤，等价于「该键不出现」
		}
		if _, isList := listKeys[key]; isList {
			return []string{}
		}
		return ""
	}
	if _, isList := listKeys[key]; isList {
		return splitCSV(raw)
	}
	return raw
}

// splitCSV 按逗号拆分并去除空白项。
func splitCSV(raw string) []string {
	parts := strings.Split(raw, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if s := strings.TrimSpace(p); s != "" {
			out = append(out, s)
		}
	}
	return out
}

// envMap 读取全部真实环境变量并做与 .env 文件相同的值转换。
//
// 不做键名白名单过滤：与 pydantic-settings 一致，按字段名匹配、未识别的键忽略，
// 这样 compose 注入任意配置项都无需改动本文件。
func envMap() map[string]any {
	environ := os.Environ()
	out := make(map[string]any, len(environ))
	for _, entry := range environ {
		k, v, ok := strings.Cut(entry, "=")
		if !ok || k == "" {
			continue
		}
		if val := valueFor(k, v); val != nil {
			out[k] = val
		}
	}
	return out
}

// unquote 去掉成对的首尾引号（值内部的 # 不当作注释，与 Python dotenv 行为一致）。
func unquote(s string) string {
	if len(s) >= 2 {
		if (s[0] == '"' && s[len(s)-1] == '"') || (s[0] == '\'' && s[len(s)-1] == '\'') {
			return s[1 : len(s)-1]
		}
	}
	return s
}

// normalize 做类型收敛与派生校验。
func (c *Config) normalize() error {
	switch c.Environment {
	case EnvDev, EnvProd:
	default:
		return fmt.Errorf("ENVIRONMENT 只能是 dev 或 prod，当前为 %q", c.Environment)
	}
	if c.DatabaseType != "mysql" {
		return fmt.Errorf("DATABASE_TYPE 仅支持 mysql，当前为 %q", c.DatabaseType)
	}
	if c.ServerPort <= 0 || c.ServerPort > 65535 {
		return fmt.Errorf("SERVER_PORT 非法: %d", c.ServerPort)
	}
	if c.SnowflakeWorkerID != nil {
		if *c.SnowflakeWorkerID < 0 || *c.SnowflakeWorkerID > 1023 {
			return fmt.Errorf("SNOWFLAKE_WORKER_ID 必须在 0-1023 范围内，当前值：%d", *c.SnowflakeWorkerID)
		}
	}
	// 逗号分隔字符串 → 切片（环境变量只能传字符串，文件层已给切片）
	c.TrustedProxyIPs = splitList(c.TrustedProxyIPs)
	c.AllowedHosts = splitList(c.AllowedHosts)
	c.XSSExcludeURLs = splitList(c.XSSExcludeURLs)
	c.WhiteAPIListPath = splitList(c.WhiteAPIListPath)
	c.CORSExposeHeaders = splitList(c.CORSExposeHeaders)
	c.AllowedExtensions = splitList(c.AllowedExtensions)
	c.OperationRecordMethod = splitList(c.OperationRecordMethod)
	return nil
}

// splitList 兼容两种来源：文件层已是多元素切片（原样返回），
// 环境变量层是单个逗号分隔字符串（拆分）。
func splitList(in []string) []string {
	if len(in) == 0 {
		return in
	}
	out := make([]string, 0, len(in))
	for _, item := range in {
		for _, part := range strings.Split(item, ",") {
			if p := strings.TrimSpace(part); p != "" {
				out = append(out, p)
			}
		}
	}
	return out
}

// Addr 返回监听地址。
func (c *Config) Addr() string {
	return c.ServerHost + ":" + strconv.Itoa(c.ServerPort)
}

// IsWhiteListed 判断路径是否命中公开白名单（支持 * 结尾前缀匹配），
// 语义与 Python 端 init_app._white_listed 完全一致。
func (c *Config) IsWhiteListed(path string) bool {
	for _, item := range c.WhiteAPIListPath {
		if strings.HasSuffix(item, "*") {
			if strings.HasPrefix(path, strings.TrimSuffix(item, "*")) {
				return true
			}
			continue
		}
		if path == item {
			return true
		}
	}
	return false
}
