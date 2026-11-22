package docs

import "walnut-seed-go/internal/pkg/routex"

// Mount 注册文档端点。均在公开白名单内（/docs* /redoc /openapi.json，
// 见 config.WhiteAPIListPath，镜像 Python setting.py），经 Public 组注册。
func (h *Handler) Mount(b routex.Binder) {
	b.GET("/openapi.json", h.OpenAPI)
	b.GET("/docs", h.SwaggerUI)
	b.GET("/docs/oauth2-redirect", h.OAuth2Redirect)
	b.GET("/redoc", h.ReDoc)
}
