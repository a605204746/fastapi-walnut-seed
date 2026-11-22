package middleware

import (
	"compress/gzip"
	"fmt"
	"io"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

func gzipEngine() *gin.Engine {
	gin.SetMode(gin.TestMode)
	e := gin.New()
	e.Use(GZip(GZipConfig{MinSize: 1000, CompressLevel: 9}))
	e.GET("/small", func(c *gin.Context) { c.JSON(200, gin.H{"code": 200, "msg": "操作成功", "data": nil}) })
	e.GET("/stream", func(c *gin.Context) {
		c.Writer.Header().Set("Content-Type", "text/event-stream; charset=utf-8")
		c.Writer.WriteHeader(200)
		fmt.Fprint(c.Writer, "event: connected\n\n")
		c.Writer.Flush()
	})
	e.GET("/png", func(c *gin.Context) { c.Data(200, "image/png", []byte("fakepng")) })
	return e
}

func TestGZipCompressesSmallResponses(t *testing.T) {
	// Python 端 minimum_size 被 BaseHTTPMiddleware 流式化架空，小响应也压缩
	req := httptest.NewRequest("GET", "/small", nil)
	req.Header.Set("Accept-Encoding", "gzip")
	w := httptest.NewRecorder()
	gzipEngine().ServeHTTP(w, req)
	if w.Header().Get("Content-Encoding") != "gzip" {
		t.Fatalf("small response not compressed: %v", w.Header())
	}
	if !strings.Contains(w.Header().Get("Vary"), "Accept-Encoding") {
		t.Fatalf("missing Vary: %v", w.Header())
	}
	zr, err := gzip.NewReader(w.Body)
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(zr)
	if !strings.Contains(string(body), `"code":200`) {
		t.Fatalf("decompressed body wrong: %s", body)
	}
}

func TestGZipVaryWithoutAcceptEncoding(t *testing.T) {
	req := httptest.NewRequest("GET", "/small", nil)
	w := httptest.NewRecorder()
	gzipEngine().ServeHTTP(w, req)
	if w.Header().Get("Content-Encoding") != "" {
		t.Fatal("compressed without Accept-Encoding")
	}
	if !strings.Contains(w.Header().Get("Vary"), "Accept-Encoding") {
		t.Fatalf("Python 实测无 AE 也带 Vary，这里缺失: %v", w.Header())
	}
}

func TestGZipExcludesSSEAndImages(t *testing.T) {
	for _, path := range []string{"/stream", "/png"} {
		req := httptest.NewRequest("GET", path, nil)
		req.Header.Set("Accept-Encoding", "gzip")
		w := httptest.NewRecorder()
		gzipEngine().ServeHTTP(w, req)
		if w.Header().Get("Content-Encoding") == "gzip" {
			t.Fatalf("%s should not be compressed", path)
		}
	}
}

func TestGZipHeadEmptyBody(t *testing.T) {
	req := httptest.NewRequest("HEAD", "/small", nil)
	req.Header.Set("Accept-Encoding", "gzip")
	w := httptest.NewRecorder()
	gzipEngine().ServeHTTP(w, req)
	if w.Body.Len() != 0 {
		t.Fatalf("HEAD body not empty: %d", w.Body.Len())
	}
}