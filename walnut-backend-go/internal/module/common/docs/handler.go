// Package docs 托管 API 文档 UI（Swagger UI / ReDoc）与 OpenAPI spec 端点，
// 对应 Python 端 init_app.register_docs。spec 来自冻结契约 api/golden/contract.json
// （main.go go:embed 注入，自包含二进制，容器无需额外 COPY）。
// Swagger UI / ReDoc 资源走 CDN（轻量托管，浏览器查看时需联网）。
package docs

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

// Handler 文档端点处理器。spec 为 OpenAPI JSON 字节（nil 时 /openapi.json 回退空对象）。
type Handler struct {
	spec []byte
}

func NewHandler(spec []byte) *Handler { return &Handler{spec: spec} }

// OpenAPI  serve 冻结契约 spec（Swagger UI / ReDoc 的数据源）。
func (h *Handler) OpenAPI(c *gin.Context) {
	spec := h.spec
	if len(spec) == 0 {
		spec = []byte("{}")
	}
	c.Data(http.StatusOK, "application/json; charset=utf-8", spec)
}

func (h *Handler) SwaggerUI(c *gin.Context) {
	c.Data(http.StatusOK, "text/html; charset=utf-8", []byte(swaggerHTML))
}

func (h *Handler) OAuth2Redirect(c *gin.Context) {
	c.Data(http.StatusOK, "text/html; charset=utf-8", []byte(oauth2RedirectHTML))
}

func (h *Handler) ReDoc(c *gin.Context) {
	c.Data(http.StatusOK, "text/html; charset=utf-8", []byte(redocHTML))
}
