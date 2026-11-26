package main

import (
	"math"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestFormatFloat(t *testing.T) {
	cases := []struct {
		in   float64
		want string
	}{
		{0, "0"},
		{1, "1"},
		{12.5, "12.5"},
		{12.50004, "12.5"},
		{1.23456, "1.2346"},
		{0.00001, "0"},
		{0.1, "0.1"},
		{-3.25, "-3.25"},
		{66, "66"},
		{100, "100"},
		{1234567.8912, "1234567.8912"},
	}
	for _, c := range cases {
		if got := formatFloat(c.in); got != c.want {
			t.Errorf("formatFloat(%v) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestLabelString(t *testing.T) {
	if got := labelString(nil); got != "" {
		t.Errorf("labelString(nil) = %q, want \"\"", got)
	}
	if got := labelString(map[string]string{}); got != "" {
		t.Errorf("labelString({}) = %q, want \"\"", got)
	}
	if got := labelString(map[string]string{"mountpoint": "/"}); got != `{mountpoint="/"}` {
		t.Errorf("labelString 单标签 = %q, want `{mountpoint=\"/\"}`", got)
	}
	// 标签键按字典序排序
	if got := labelString(map[string]string{"z": "1", "a": "2", "m": "3"}); got != `{a="2",m="3",z="1"}` {
		t.Errorf("labelString 排序 = %q, want `{a=\"2\",m=\"3\",z=\"1\"}`", got)
	}
	// 换行转义：先由 Replacer 转成反斜杠+n，再由 %q 转义反斜杠
	if got := labelString(map[string]string{"k": "line1\nline2"}); got != `{k="line1\\nline2"}` {
		t.Errorf("labelString 换行转义 = %q, want `{k=\"line1\\\\nline2\"}`", got)
	}
	// 引号转义：同理，双重转义后的字面形式
	if got := labelString(map[string]string{"k": `say "hi"`}); got != `{k="say \\\"hi\\\""}` {
		t.Errorf("labelString 引号转义 = %q, want `{k=\"say \\\\\\\"hi\\\\\\\"\"}`", got)
	}
}

func TestRenderPrometheusEmpty(t *testing.T) {
	if got := renderPrometheus(nil); got != "" {
		t.Errorf("renderPrometheus(nil) = %q, want \"\"", got)
	}
	if got := renderPrometheus([]metric{}); got != "" {
		t.Errorf("renderPrometheus([]) = %q, want \"\"", got)
	}
}

func TestRenderPrometheusSortingAndTypes(t *testing.T) {
	out := renderPrometheus([]metric{
		{name: "z_metric", help: "Z", typ: "counter", value: 7},
		{name: "a_metric", help: "A", typ: "gauge", value: 1},
		{name: "a_metric", help: "A", typ: "gauge", labels: map[string]string{"x": "2"}, value: 3},
		{name: "a_metric", help: "A", typ: "gauge", labels: map[string]string{"x": "1"}, value: 2},
	})
	// 按指标名排序：a_metric 在 z_metric 之前
	if strings.Index(out, "a_metric") > strings.Index(out, "z_metric") {
		t.Errorf("指标未按名排序:\n%s", out)
	}
	// 同名指标 HELP/TYPE 只输出一次
	if n := strings.Count(out, "# TYPE a_metric gauge"); n != 1 {
		t.Errorf("# TYPE a_metric gauge 出现 %d 次, want 1:\n%s", n, out)
	}
	if !strings.Contains(out, "# TYPE z_metric counter") {
		t.Errorf("counter 类型行缺失:\n%s", out)
	}
	// 同名样本按标签排序：无标签 < x="1" < x="2"
	i0 := strings.Index(out, "a_metric 1\n")
	i1 := strings.Index(out, `a_metric{x="1"} 2`)
	i2 := strings.Index(out, `a_metric{x="2"} 3`)
	if i0 < 0 || i1 < 0 || i2 < 0 || !(i0 < i1 && i1 < i2) {
		t.Errorf("同名样本顺序不对:\n%s", out)
	}
}

func TestRenderPrometheusValueFormatting(t *testing.T) {
	out := renderPrometheus([]metric{
		{name: "m_half", help: "h", typ: "gauge", value: 12.5000},
		{name: "m_int", help: "h", typ: "gauge", value: 100},
		{name: "m_zero", help: "h", typ: "gauge", value: 0},
	})
	for _, want := range []string{"m_half 12.5\n", "m_int 100\n", "m_zero 0\n"} {
		if !strings.Contains(out, want) {
			t.Errorf("缺少样本行 %q:\n%s", want, out)
		}
	}
}

func TestMetricsHandlerNoAuth(t *testing.T) {
	h := metricsHandler("")
	rec := httptest.NewRecorder()
	h(rec, httptest.NewRequest(http.MethodGet, "/metrics", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("无 token 应返回 200,实际 %d", rec.Code)
	}
	if ct := rec.Header().Get("Content-Type"); ct != "text/plain; version=0.0.4; charset=utf-8" {
		t.Errorf("Content-Type = %q", ct)
	}
	body := rec.Body.String()
	if !strings.Contains(body, "# HELP jiankong_build_info") {
		t.Error("响应体缺少 jiankong_build_info")
	}
	if !strings.Contains(body, "jiankong_scrape_duration_seconds") {
		t.Error("响应体缺少 jiankong_scrape_duration_seconds")
	}
}

func TestMetricsHandlerWrongToken(t *testing.T) {
	h := metricsHandler("s3cret")
	req := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	req.Header.Set("Authorization", "Bearer wrong")
	rec := httptest.NewRecorder()
	h(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Errorf("错误 token 应返回 401,实际 %d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "unauthorized") {
		t.Errorf("401 响应体 = %q, want 包含 unauthorized", rec.Body.String())
	}
}

func TestCollectMetricsInvariants(t *testing.T) {
	ms := collectMetrics()
	if len(ms) == 0 {
		t.Fatal("采集结果为空")
	}
	seen := map[string]bool{}
	foundBuild := false
	foundCPUMem := false
	for _, m := range ms {
		if m.name == "" || !strings.HasPrefix(m.name, "jiankong_") {
			t.Errorf("指标名异常: %q", m.name)
		}
		if m.help == "" {
			t.Errorf("指标 %s 缺少 help", m.name)
		}
		if m.typ != "gauge" && m.typ != "counter" {
			t.Errorf("指标 %s 类型异常: %q", m.name, m.typ)
		}
		if math.IsNaN(m.value) || math.IsInf(m.value, 0) {
			t.Errorf("指标 %s 值为非有限数: %v", m.name, m.value)
		}
		key := m.name + labelString(m.labels)
		if seen[key] {
			t.Errorf("重复样本: %s", key)
		}
		seen[key] = true
		if m.name == "jiankong_build_info" {
			foundBuild = true
			if m.value != 1 {
				t.Errorf("jiankong_build_info = %v, want 1", m.value)
			}
			if m.labels["version"] != version {
				t.Errorf("jiankong_build_info version 标签 = %q, want %q", m.labels["version"], version)
			}
		}
		if strings.Contains(m.name, "cpu") || strings.Contains(m.name, "memory") {
			foundCPUMem = true
		}
	}
	if !foundBuild {
		t.Error("缺少 jiankong_build_info")
	}
	if !foundCPUMem {
		t.Error("缺少 CPU/内存类指标")
	}
}
