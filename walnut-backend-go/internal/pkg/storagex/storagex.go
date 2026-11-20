// Package storagex 实现对象存储抽象，对应 Python 端 app/core/file_storage.py。
//
// 支持 S3 兼容存储（SeaweedFS / MinIO / AWS S3）。
// 阿里云 OSS 当前为占位实现（Python 端 oss2 也是可选懒导入）。
//
// 安全契约（与 Python 端逐条一致）：
//   - 扩展名白名单：ALLOWED_EXTENSIONS，不含 html/htm/svg（防存储型 XSS）
//   - 大小限制：MAX_FILE_SIZE，分块累计计数，超限即中止
//   - 所有下载/回源响应都带 X-Content-Type-Options: nosniff + Content-Disposition: attachment
package storagex

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"mime"
	"path/filepath"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"

	"walnut-seed-go/internal/config"
	"walnut-seed-go/internal/pkg/errs"
)

// UploadResult 上传结果，对应 Python 端 UploadResult dataclass。
// 字段名与 Python model_dump() 一致（snake_case，无 alias）。
type UploadResult struct {
	URL              string `json:"url"`
	OriginalFilename string `json:"original_filename"`
}

// ErrSizeExceeded 上传累计大小超过 MAX_FILE_SIZE 的哨兵错误，
// 由 handler 映射为 i18n 业务异常（upload.exceed.maxSize）。
var ErrSizeExceeded = errors.New("upload size exceeded")

// Service 对象存储服务接口。
type Service interface {
	Upload(ctx context.Context, filename string, data []byte) (*UploadResult, error)
	Download(ctx context.Context, fileURL string) (*ObjectData, error)
	Serve(ctx context.Context, fileURL string) (*ObjectData, error)
	Delete(ctx context.Context, fileURL string) error
	EnsureBucket(ctx context.Context) error
}

// ObjectData 下载/回源的对象数据。
type ObjectData struct {
	Data        []byte
	ContentType string
	Filename    string
}

// S3Service S3 兼容存储（SeaweedFS / MinIO）。
type S3Service struct {
	client    *minio.Client
	bucket    string
	urlPrefix string
	cfg       *config.Config
	log       *slog.Logger
}

// NewS3Service 构造 S3 存储服务。
func NewS3Service(cfg *config.Config, log *slog.Logger) (*S3Service, error) {
	// SeaweedFS 当前版本静态 -s3.config 不生效（上游 #4728/#8331），
	// S3 网关默认放行匿名访问但拒绝签名请求。故先用匿名凭证，
	// 与 Python 端 minio SDK 的行为对齐（Python SDK 对空 AK 走匿名模式）。
	client, err := minio.New(cfg.OSSS3Endpoint, &minio.Options{
		Creds:  credentials.NewStaticV4("", "", ""),
		Secure: cfg.OSSS3Secure,
	})
	if err != nil {
		return nil, fmt.Errorf("创建 S3 客户端失败: %w", err)
	}
	urlPrefix := strings.TrimRight(cfg.OSSS3URLPrefix, "/")
	return &S3Service{
		client:    client,
		bucket:    cfg.OSSS3BucketName,
		urlPrefix: urlPrefix,
		cfg:       cfg,
		log:       log,
	}, nil
}

// EnsureBucket 确保桶存在，对应 Python 端 ensure_bucket。
// 失败仅警告（启动时调用），上传时 NoSuchBucket 会惰性重试。
func (s *S3Service) EnsureBucket(ctx context.Context) error {
	exists, err := s.client.BucketExists(ctx, s.bucket)
	if err != nil {
		return err
	}
	if !exists {
		return s.client.MakeBucket(ctx, s.bucket, minio.MakeBucketOptions{})
	}
	return nil
}

// Upload 上传文件，返回公开 URL。
func (s *S3Service) Upload(ctx context.Context, filename string, data []byte) (*UploadResult, error) {
	if filename == "" {
		return nil, errs.BadRequest("上传文件不能为空")
	}
	if err := CheckAllowedExtension(filename, s.cfg.AllowedExtensions); err != nil {
		return nil, err
	}
	key := buildKey(filename)
	contentType := mime.TypeByExtension(filepath.Ext(filename))
	if contentType == "" {
		contentType = "application/octet-stream"
	}

	_, err := s.client.PutObject(ctx, s.bucket, key, bytes.NewReader(data), int64(len(data)),
		minio.PutObjectOptions{ContentType: contentType})
	if err != nil {
		// NoSuchBucket 时惰性重试一次
		if isNoSuchBucket(err) {
			if bErr := s.EnsureBucket(ctx); bErr != nil {
				s.log.Warn("OSS 桶自建失败", slog.String("error", bErr.Error()))
			}
			_, err = s.client.PutObject(ctx, s.bucket, key, bytes.NewReader(data), int64(len(data)),
				minio.PutObjectOptions{ContentType: contentType})
		}
		if err != nil {
			return nil, fmt.Errorf("上传对象失败: %w", err)
		}
	}
	url := s.urlPrefix + "/" + key
	return &UploadResult{URL: url, OriginalFilename: filename}, nil
}

// Download 下载文件（attachment + nosniff）。
func (s *S3Service) Download(ctx context.Context, fileURL string) (*ObjectData, error) {
	key := s.keyFromURL(fileURL)
	return s.getObject(ctx, key, true)
}

// Serve 回源访问（nosniff + attachment + cacheable）。
func (s *S3Service) Serve(ctx context.Context, fileURL string) (*ObjectData, error) {
	key := fileURL // serve 路由的路径参数就是 key
	return s.getObject(ctx, key, false)
}

// Delete 删除文件（best-effort，失败仅警告）。
func (s *S3Service) Delete(ctx context.Context, fileURL string) error {
	key := s.keyFromURL(fileURL)
	err := s.client.RemoveObject(ctx, s.bucket, key, minio.RemoveObjectOptions{})
	if err != nil {
		s.log.Warn("删除对象失败", slog.String("key", key), slog.String("error", err.Error()))
	}
	return nil
}

func (s *S3Service) keyFromURL(fileURL string) string {
	if s.urlPrefix != "" && strings.HasPrefix(fileURL, s.urlPrefix) {
		return strings.TrimLeft(fileURL[len(s.urlPrefix):], "/")
	}
	return strings.TrimLeft(fileURL, "/")
}

func (s *S3Service) getObject(ctx context.Context, key string, withFilename bool) (*ObjectData, error) {
	// 空 key：Python 端 minio SDK 抛 ValueError（空消息）→ code 400、msg ""。
	if key == "" {
		return nil, errs.WithCode(errs.CodeBadRequest, "")
	}

	obj, err := s.client.GetObject(ctx, s.bucket, key, minio.GetObjectOptions{})
	if err != nil {
		return nil, mapStorageError(err)
	}
	defer obj.Close()

	data, err := io.ReadAll(obj)
	if err != nil {
		return nil, mapStorageError(err)
	}

	stat, err := obj.Stat()
	contentType := "application/octet-stream"
	if err == nil && stat.ContentType != "" {
		contentType = stat.ContentType
	}

	out := &ObjectData{Data: data, ContentType: contentType}
	if withFilename {
		parts := strings.Split(key, "/")
		out.Filename = parts[len(parts)-1]
	}
	return out, nil
}

// mapStorageError 把 S3 错误映射为契约异常：NoSuchKey → code 404「文件不存在」
// （对齐 Python _get_object 的 ServiceException(NOT_FOUND)），其余原样透传给
// 调用方兜底为 500。minio-go 的 GetObject 是惰性的，错误在 Read/Stat 时才浮现。
func mapStorageError(err error) error {
	if minio.ToErrorResponse(err).Code == "NoSuchKey" {
		return errs.WithCode(errs.CodeNotFound, "文件不存在")
	}
	return err
}

func isNoSuchBucket(err error) bool {
	if err == nil {
		return false
	}
	return strings.Contains(err.Error(), "NoSuchBucket") ||
		strings.Contains(err.Error(), "bucket does not exist")
}

// ---- 辅助函数 ----

// buildKey 生成存储键：yyyy/MM/dd/{uuid32}.{ext}。
// 与 Python _build_key 一致：uuid4().hex 无连字符，扩展名保留原始大小写。
func buildKey(filename string) string {
	ext := filepath.Ext(filename)
	now := time.Now()
	return fmt.Sprintf("%d/%02d/%02d/%s%s",
		now.Year(), now.Month(), now.Day(),
		strings.ReplaceAll(uuid.New().String(), "-", ""), ext)
}

// CheckAllowedExtension 校验扩展名白名单（导出供 handler 在读取文件体之前调用，
// 对齐 Python 先校验扩展名再读大小的顺序）。
func CheckAllowedExtension(filename string, allowed []string) error {
	ext := strings.TrimPrefix(strings.ToLower(filepath.Ext(filename)), ".")
	if ext == "" {
		return errs.BadRequest("不允许上传的文件扩展名：.<空>")
	}
	for _, a := range allowed {
		if strings.ToLower(a) == ext {
			return nil
		}
	}
	return errs.BadRequest(fmt.Sprintf("不允许上传的文件扩展名：.%s", ext))
}

// ServeHeaders 构建安全响应头，对应 Python 端 _serve_headers。
func ServeHeaders(filename string, cacheable bool) map[string]string {
	h := map[string]string{
		"X-Content-Type-Options": "nosniff",
	}
	if filename != "" {
		h["Content-Disposition"] = fmt.Sprintf(`attachment; filename="%s"`, filename)
	} else {
		h["Content-Disposition"] = "attachment"
	}
	if cacheable {
		h["Cache-Control"] = "public, max-age=86400"
	}
	return h
}

// NewService 根据配置创建存储服务。
func NewService(cfg *config.Config, log *slog.Logger) (Service, error) {
	switch cfg.OSSType {
	case "s3", "":
		return NewS3Service(cfg, log)
	default:
		return nil, fmt.Errorf("不支持的存储类型: %s（仅支持 s3）", cfg.OSSType)
	}
}
