// Package file 实现文件管理端点，对应 Python 端 module_common/file/controller.py。
package file

import (
	"errors"
	"io"
	"net/http"

	"github.com/gin-gonic/gin"

	"walnut-seed-go/internal/config"
	"walnut-seed-go/internal/pkg/envelope"
	"walnut-seed-go/internal/pkg/errs"
	"walnut-seed-go/internal/pkg/storagex"
)

// Handler 文件域端点。
type Handler struct {
	storage storagex.Service
	cfg     *config.Config
	errs    *errs.Handler
}

// NewHandler 构造文件 handler。
func NewHandler(storage storagex.Service, cfg *config.Config, eh *errs.Handler) *Handler {
	return &Handler{storage: storage, cfg: cfg, errs: eh}
}

// Upload POST /common/file/upload
func (h *Handler) Upload(c *gin.Context) {
	file, header, err := c.Request.FormFile("file")
	if err != nil {
		h.errs.Abort(c, errs.BadRequest("上传文件不能为空"))
		return
	}
	defer file.Close()

	// 先校验扩展名再读文件体（对齐 Python upload 的顺序：
	// 坏扩展名 + 超限时 Python 报扩展名错误而非大小错误）。
	if err := storagex.CheckAllowedExtension(header.Filename, h.cfg.AllowedExtensions); err != nil {
		h.errs.Abort(c, err)
		return
	}

	// 分块读取，累计计数，超限即中止
	data, err := readWithLimit(file, h.cfg.MaxFileSize)
	if err != nil {
		if errors.Is(err, storagex.ErrSizeExceeded) {
			h.errs.Abort(c, &errs.ServiceError{
				Code:       errs.CodeBadRequest,
				MessageKey: "upload.exceed.maxSize",
				KeyArgs:    []any{h.cfg.MaxFileSize / (1024 * 1024)},
			})
			return
		}
		h.errs.Abort(c, errs.BadRequest(err.Error()))
		return
	}

	result, err := h.storage.Upload(c.Request.Context(), header.Filename, data)
	if err != nil {
		h.errs.Abort(c, errs.Wrap(err, errs.MsgSystemError))
		return
	}
	envelope.OKMsg(c, result, "上传成功")
}

// Download GET /common/file/download?file_url=xxx
func (h *Handler) Download(c *gin.Context) {
	fileURL, exists := c.GetQuery("file_url")
	if !exists {
		h.errs.Abort(c, errs.BadRequest("Field required"))
		return
	}

	obj, err := h.storage.Download(c.Request.Context(), fileURL)
	if err != nil {
		h.errs.Abort(c, errs.Wrap(err, errs.MsgSystemError))
		return
	}

	headers := storagex.ServeHeaders(obj.Filename, false)
	for k, v := range headers {
		c.Header(k, v)
	}
	c.Data(http.StatusOK, obj.ContentType, obj.Data)
}

// Delete DELETE /common/file/delete?file_url=xxx
func (h *Handler) Delete(c *gin.Context) {
	fileURL, exists := c.GetQuery("file_url")
	if !exists {
		h.errs.Abort(c, errs.BadRequest("Field required"))
		return
	}

	if err := h.storage.Delete(c.Request.Context(), fileURL); err != nil {
		h.errs.Abort(c, errs.Wrap(err, errs.MsgSystemError))
		return
	}
	envelope.OKMsg(c, nil, "删除成功")
}

// readWithLimit 分块读取上传文件，超限即中止。
func readWithLimit(r io.Reader, maxSize int64) ([]byte, error) {
	const chunkSize = 1024 * 1024 // 1 MB
	buf := make([]byte, 0, chunkSize)
	var total int64

	for {
		chunk := make([]byte, chunkSize)
		n, err := r.Read(chunk)
		if n > 0 {
			total += int64(n)
			if total > maxSize {
				return nil, storagex.ErrSizeExceeded
			}
			buf = append(buf, chunk[:n]...)
		}
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, err
		}
	}
	return buf, nil
}
