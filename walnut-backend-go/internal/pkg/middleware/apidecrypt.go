package middleware

import (
	"bytes"
	"crypto/rsa"
	"errors"
	"io"
	"log/slog"
	"net/http"

	"github.com/gin-gonic/gin"

	"walnut-seed-go/internal/pkg/cryptox"
	"walnut-seed-go/internal/pkg/envelope"
)

// ctxKeyEncryptResponse 标记「本端点响应体需加密」，由 EncryptResponse() 设置。
// 对应 Python 端 @api_encrypt(response=True) 装饰器打在端点函数上的
// _api_encrypt_response 属性。
const ctxKeyEncryptResponse = "api_encrypt_response"

// 与 Python 端 ApiDecryptMiddleware 的两个 except 分支逐字一致的文案。
const (
	msgDecryptForbidden = "没有访问权限，请联系管理员授权"
	msgDecryptFailed    = "请求解密失败"
)

// ApiDecryptConfig 接口加解密中间件配置。
type ApiDecryptConfig struct {
	// HeaderFlag 协商头名，默认 encrypt-key（settings.API_DECRYPT_HEADER_FLAG）
	HeaderFlag string
	// PrivateKeyB64 解**请求**用（与前端 VITE_GLOB_RSA_PUBLIC_KEY 配对）
	PrivateKeyB64 string
	// PublicKeyB64 加**响应**用（与前端 VITE_GLOB_RSA_PRIVATE_KEY 配对）
	PublicKeyB64 string
	Logger       *slog.Logger
}

// EncryptResponse 是**路由级**标记中间件：给端点打上「响应体需加密」的标记。
//
// 用法对应 Python 端的 @api_encrypt(response=True)：
//
//	g.POST("/login", middleware.EncryptResponse(), handler)
//
// 必须放在业务 handler **之前**：标记要在响应写出前设置好。
func EncryptResponse() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Set(ctxKeyEncryptResponse, true)
		c.Next()
	}
}

// ApiDecrypt 实现接口传输加解密，对齐 Python 端 ApiDecryptMiddleware。
//
// **请求方向**（透明）：请求头带 HeaderFlag 且方法是 POST/PUT 时解密请求体；
// 不带该头的明文请求直接放行——这使前后端可以灰度切换加密开关。
//
// **响应方向**（显式）：仅当路由挂了 EncryptResponse() 标记时才加密。
//
// ⚠️ 在中间件链中的位置**必须在 XSS 与 RequestLog 之前**：
//   - 在 RequestLog 之前 → 请求日志记的是解密后明文（否则记的是密文，无法排障）
//   - 在 XSS 之前 → XSS 过滤作用于明文（在密文上过滤等于没过滤）
//
// 协议（与前端 jsencrypt + crypto-js 对齐）：
//
//	请求：encrypt-key = RSA_PKCS1v15(base64(aesKey), 后端公钥)
//	      body        = AES_ECB_PKCS7(明文JSON, aesKey) 的 Base64 文本
//	响应：encrypt-key = RSA_PKCS1v15(base64(aesKey), 前端公钥)
//	      body        = AES_ECB_PKCS7(明文JSON, aesKey) 的 Base64 文本
//
// ⚠️ 被 RSA 包裹的是 **base64(aesKey)** 而不是裸密钥——漏掉这层 Base64 是最常见的
// 对接失败原因：RSA 解出的是 44 字符 Base64 文本，直接当密钥会因长度不是
// 16/24/32 而报「AES秘钥长度要求为16位、24位、32位」。
func ApiDecrypt(cfg ApiDecryptConfig) gin.HandlerFunc {
	header := cfg.HeaderFlag
	if header == "" {
		header = "encrypt-key"
	}
	logger := cfg.Logger
	if logger == nil {
		logger = slog.Default()
	}

	// 密钥在**构造期**解析一次：RSA 密钥解析涉及大整数运算，不该每请求做一遍。
	// 解析失败则该方向降级为不可用——与 Python 端「密钥无效则不注册中间件」效果一致；
	// config.ValidateSecurity 已在启动期把这种情况关掉并告警，故这里只记日志。
	privKey, privErr := parsePrivateKey(cfg.PrivateKeyB64)
	if privErr != nil && cfg.PrivateKeyB64 != "" {
		logger.Warn("接口解密私钥解析失败，请求解密不可用", slog.String("err", privErr.Error()))
	}
	pubKey, pubErr := parsePublicKey(cfg.PublicKeyB64)
	if pubErr != nil && cfg.PublicKeyB64 != "" {
		logger.Warn("接口加密公钥解析失败，响应加密不可用", slog.String("err", pubErr.Error()))
	}

	return func(c *gin.Context) {
		// ---- 请求解密 ----
		if encryptKey := c.GetHeader(header); encryptKey != "" &&
			(c.Request.Method == http.MethodPost || c.Request.Method == http.MethodPut) {

			if privKey == nil {
				// 私钥不可用却收到加密请求：**不能**静默把密文当明文交给业务层
				// （那会让业务层抛一堆莫名其妙的 JSON 解析错误，极难定位）。
				// 按 Python 端 ServiceException 分支处理：HTTP 200 + code 403。
				logger.Error("收到加密请求但服务端 RSA 私钥不可用",
					slog.String("path", c.Request.URL.Path))
				envelope.FailWith(c, http.StatusOK, envelope.CodeForbidden, msgDecryptForbidden, nil)
				c.Abort()
				return
			}

			plain, err := decryptRequest(c, encryptKey, privKey)
			if err != nil {
				logger.Error("接口解密失败",
					slog.String("path", c.Request.URL.Path),
					slog.String("err", err.Error()))
				var ke *keyFormatError
				if errors.As(err, &ke) {
					// 密钥/格式类问题 → 403（对应 Python 的 except ServiceException）
					envelope.FailWith(c, http.StatusOK, envelope.CodeForbidden, msgDecryptForbidden, nil)
				} else {
					// 其他异常 → 400（对应 Python 的 except Exception）
					envelope.FailWith(c, http.StatusOK, envelope.CodeBadRequest, msgDecryptFailed, nil)
				}
				c.Abort()
				return
			}
			replaceBody(c, plain)
		}

		// ---- 响应加密（仅显式标记的端点）----
		if v, ok := c.Get(ctxKeyEncryptResponse); !ok || v != true {
			c.Next()
			return
		}
		if pubKey == nil {
			logger.Warn("端点要求响应加密但 RSA 公钥不可用，按明文返回",
				slog.String("path", c.Request.URL.Path))
			c.Next()
			return
		}

		// 缓冲响应体：加密必须先拿到完整明文，故用包装 writer 截留
		bw := &bufferedWriter{ResponseWriter: c.Writer, body: &bytes.Buffer{}, status: http.StatusOK}
		c.Writer = bw
		c.Next()

		encrypted, wrappedKey, err := encryptResponse(bw.body.Bytes(), pubKey)
		if err != nil {
			// 加密失败时按 Python 端的做法**回退明文**并记错误日志：
			// 宁可让这一个响应未加密（前端因缺少 encrypt-key 头会按明文解析），
			// 也不要返回 500 让整个请求失败。
			logger.Error("接口响应加密失败，按明文返回",
				slog.String("path", c.Request.URL.Path),
				slog.String("err", err.Error()))
			c.Writer = bw.ResponseWriter
			writeRaw(bw.ResponseWriter, bw.status, bw.body.Bytes(), "application/json; charset=utf-8")
			return
		}

		h := bw.ResponseWriter.Header()
		// 必须暴露该头，否则前端跨域时读不到（CORS expose headers 限制）
		h.Set("Access-Control-Expose-Headers", header)
		h.Set(header, wrappedKey)
		writeRaw(bw.ResponseWriter, bw.status, encrypted, "application/json; charset=utf-8")
	}
}

// decryptRequest 解开请求体，返回明文字节。
func decryptRequest(c *gin.Context, encryptKey string, priv *rsa.PrivateKey) ([]byte, error) {
	raw, err := io.ReadAll(c.Request.Body)
	if err != nil {
		return nil, err
	}
	if len(bytes.TrimSpace(raw)) == 0 {
		// 空 body 无需解密（如 PUT 只带查询参数的场景）
		return raw, nil
	}

	// 1) RSA 解出 base64(aesKey)
	wrapped, err := cryptox.DecryptByRSA(encryptKey, priv)
	if err != nil {
		// RSA 解不开通常意味着密钥不配对（前端用了另一对公钥），归为格式类错误
		return nil, &keyFormatError{err: err}
	}
	// 2) Base64 解码得到真正的 AES 密钥字符串
	aesKey, err := cryptox.DecodeBase64Str(wrapped)
	if err != nil {
		return nil, &keyFormatError{err: err}
	}
	// 3) AES 解密 body（body 本身就是 Base64 文本形式的密文）
	plain, err := cryptox.DecryptByAES(string(raw), aesKey)
	if err != nil {
		return nil, &keyFormatError{err: err}
	}
	return []byte(plain), nil
}

// encryptResponse 加密响应体，返回 (Base64 密文字节, RSA 包裹的密钥)。
func encryptResponse(raw []byte, pub *rsa.PublicKey) ([]byte, string, error) {
	aesKey, err := cryptox.RandomAESKey()
	if err != nil {
		return nil, "", err
	}
	body, err := cryptox.EncryptByAES(string(raw), aesKey)
	if err != nil {
		return nil, "", err
	}
	// 被 RSA 包裹的是 base64(aesKey)，不是裸密钥
	wrapped, err := cryptox.EncryptByRSA(cryptox.EncodeBase64Str(aesKey), pub)
	if err != nil {
		return nil, "", err
	}
	return []byte(body), wrapped, nil
}

// replaceBody 把解密后的明文塞回请求，并修正 ContentLength 与 Content-Type。
//
// ⚠️ ContentLength 必须同步更新：Gin/业务层用它决定是否读 body，
// 留着密文的长度会让后续读取截断或阻塞。
func replaceBody(c *gin.Context, plain []byte) {
	c.Request.Body = io.NopCloser(bytes.NewReader(plain))
	c.Request.ContentLength = int64(len(plain))
	c.Request.Header.Set("Content-Type", "application/json")
	c.Request.Header.Set("Content-Length", itoa(len(plain)))
}

// keyFormatError 标记「密钥或密文格式问题」，用于区分 403 与 400。
// 对应 Python 端 `except ServiceException` 与 `except Exception` 的分野。
type keyFormatError struct{ err error }

func (e *keyFormatError) Error() string { return e.err.Error() }
func (e *keyFormatError) Unwrap() error { return e.err }

// bufferedWriter 包装 gin.ResponseWriter，把响应体截留在缓冲区而不立即写出。
//
// 必须实现 gin.ResponseWriter 接口的全部方法（否则 c.Writer = bw 赋值会编译失败），
// 这里通过嵌入原 writer 继承其余方法，只覆写 Write / WriteString / Status。
type bufferedWriter struct {
	gin.ResponseWriter
	body   *bytes.Buffer
	status int
}

func (w *bufferedWriter) Write(b []byte) (int, error) { return w.body.Write(b) }

func (w *bufferedWriter) WriteString(s string) (int, error) { return w.body.WriteString(s) }

func (w *bufferedWriter) WriteHeader(code int) { w.status = code }

// Status 返回业务 handler 设置的状态码。
//
// ⚠️ 必须覆写：嵌入的 ResponseWriter.Status() 返回的是**已写出**的状态码，
// 而我们把 WriteHeader 拦下来了，底层 writer 仍是默认的 200。
// 不覆写的话，handler 里设的 503/401 会在加密后被丢掉、变成 200。
func (w *bufferedWriter) Status() int { return w.status }

func (w *bufferedWriter) Written() bool { return w.body.Len() > 0 }

// writeRaw 用指定状态码与 Content-Type 写出原始字节。
func writeRaw(w gin.ResponseWriter, status int, body []byte, contentType string) {
	h := w.Header()
	h.Set("Content-Type", contentType)
	h.Set("Content-Length", itoa(len(body)))
	w.WriteHeader(status)
	_, _ = w.Write(body)
}

func parsePrivateKey(b64 string) (*rsa.PrivateKey, error) {
	if b64 == "" {
		return nil, nil
	}
	return cryptox.ParsePrivateKey(b64)
}

func parsePublicKey(b64 string) (*rsa.PublicKey, error) {
	if b64 == "" {
		return nil, nil
	}
	return cryptox.ParsePublicKey(b64)
}
