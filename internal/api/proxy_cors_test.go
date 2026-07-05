package api

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestIsCorsHeader(t *testing.T) {
	tests := []struct {
		header string
		want   bool
	}{
		{"Access-Control-Allow-Origin", true},
		{"access-control-allow-origin", true},
		{"ACCESS-CONTROL-ALLOW-METHODS", true},
		{"Access-Control-Allow-Headers", true},
		{"Access-Control-Allow-Credentials", true},
		{"Access-Control-Expose-Headers", true},
		{"Access-Control-Max-Age", true},
		{"Content-Type", false},
		{"X-Custom-Header", false},
		{"Vary", false},
		{"", false},
	}
	for _, tt := range tests {
		t.Run(tt.header, func(t *testing.T) {
			assert.Equal(t, tt.want, isCorsHeader(tt.header))
		})
	}
}

func TestCopyProxyResponseHeaders_StripsCorsHeaders(t *testing.T) {
	src := &http.Response{
		Header: http.Header{},
	}
	src.Header.Set("Content-Type", "application/json; charset=utf-8")
	src.Header.Set("Access-Control-Allow-Origin", "*")
	src.Header.Set("Access-Control-Allow-Methods", "GET, POST, DELETE")
	src.Header.Set("Access-Control-Allow-Headers", "*")
	src.Header.Set("X-Custom-Header", "custom-value")

	rec := httptest.NewRecorder()
	copyProxyResponseHeaders(rec, src)
	dst := rec.Result()

	assert.Equal(t, "application/json", dst.Header.Get("Content-Type"),
		"Content-Type should be normalized without charset")
	assert.Equal(t, "custom-value", dst.Header.Get("X-Custom-Header"),
		"non-CORS headers should be copied")
	assert.Empty(t, dst.Header.Get("Access-Control-Allow-Origin"),
		"CORS Allow-Origin should be stripped from upstream response")
	assert.Empty(t, dst.Header.Get("Access-Control-Allow-Methods"),
		"CORS Allow-Methods should be stripped from upstream response")
	assert.Empty(t, dst.Header.Get("Access-Control-Allow-Headers"),
		"CORS Allow-Headers should be stripped from upstream response")
}

func TestCopyProxyResponseHeaders_StripsCorsHeaders_CaseInsensitive(t *testing.T) {
	src := &http.Response{
		Header: http.Header{},
	}
	src.Header.Add("access-control-allow-origin", "*")
	src.Header.Add("Access-Control-Expose-Headers", "X-Custom")

	rec := httptest.NewRecorder()
	copyProxyResponseHeaders(rec, src)
	dst := rec.Result()

	assert.Empty(t, dst.Header.Get("Access-Control-Allow-Origin"),
		"CORS headers should be stripped regardless of case")
	assert.Empty(t, dst.Header.Get("Access-Control-Expose-Headers"),
		"CORS Expose-Headers should be stripped regardless of case")
}

func TestCopyProxyResponseHeaders_NonJsonContentTypePreserved(t *testing.T) {
	src := &http.Response{
		Header: http.Header{},
	}
	src.Header.Set("Content-Type", "text/html")

	rec := httptest.NewRecorder()
	copyProxyResponseHeaders(rec, src)
	dst := rec.Result()

	assert.Equal(t, "text/html", dst.Header.Get("Content-Type"),
		"non-JSON Content-Type should be copied as-is")
}

func TestCopyProxyResponseHeaders_NoCorsHeadersFromUpstream(t *testing.T) {
	src := &http.Response{
		Header: http.Header{},
	}
	src.Header.Set("Content-Type", "application/json")
	src.Header.Set("X-Trace-Id", "abc123")

	rec := httptest.NewRecorder()
	copyProxyResponseHeaders(rec, src)
	dst := rec.Result()

	assert.Equal(t, "application/json", dst.Header.Get("Content-Type"))
	assert.Equal(t, "abc123", dst.Header.Get("X-Trace-Id"))
	assert.Empty(t, dst.Header.Get("Access-Control-Allow-Origin"))
}
