package envelope

import (
	"encoding/json"
	"net/http"

	"github.com/gin-gonic/gin/render"
)

// jsonRender 强制走标准库 encoding/json 的渲染器。
//
// 为什么不用 gin 内置的 c.JSON：gin 的 render.JSON 会按 build tag 选用
// sonic / jsoniter / goccy 等替代实现，它们对自定义 MarshalJSON 的支持程度
// 与标准库存在差异（尤其 sonic 在部分场景会绕过自定义编码器）。
// 本项目的 JSInt 条件序列化与 JSONTime 格式是**契约级**行为，必须确定性一致，
// 因此这里固定使用标准库。
type jsonRender struct {
	Payload Response
}

func (r jsonRender) Render(w http.ResponseWriter) error {
	// 用 Marshal + Write 而非 json.Encoder.Encode：后者会在末尾追加 '\n'，
	// 而 Python 端 JSONResponse 不会。契约 diff 是逐字节比对，多一个换行就是差异。
	blob, err := json.Marshal(r.Payload)
	if err != nil {
		return err
	}
	r.WriteContentType(w)
	_, err = w.Write(blob)
	return err
}

func (r jsonRender) WriteContentType(w http.ResponseWriter) {
	h := w.Header()
	if val := h["Content-Type"]; len(val) == 0 {
		h["Content-Type"] = []string{"application/json"}
	}
}

var _ render.Render = jsonRender{}
