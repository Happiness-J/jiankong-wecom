package main

import (
	"encoding/json"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
)

// loadConfig：默认值（只测成功路径；缺失 WECOM_WEBHOOK 会 log.Fatal，不测）。
func TestLoadConfigDefaults(t *testing.T) {
	t.Setenv("WECOM_WEBHOOK", "https://example.invalid/fake-webhook")
	for _, k := range []string{"REPORT_TIME", "CPU_THRESHOLD", "MEM_THRESHOLD", "DISK_THRESHOLD", "CUSTOM_TITLE"} {
		if err := os.Unsetenv(k); err != nil {
			t.Fatalf("Unsetenv(%s): %v", k, err)
		}
	}
	cfg := loadConfig()
	if cfg.WecomWebhook != "https://example.invalid/fake-webhook" {
		t.Errorf("WecomWebhook = %q", cfg.WecomWebhook)
	}
	if cfg.ReportTime != "-" {
		t.Errorf("ReportTime = %q, want \"-\"", cfg.ReportTime)
	}
	if cfg.CPUAlert != 80 || cfg.MemAlert != 80 || cfg.DiskAlert != 80 {
		t.Errorf("阈值 = %d/%d/%d, want 80/80/80", cfg.CPUAlert, cfg.MemAlert, cfg.DiskAlert)
	}
	if cfg.CustomTitle != "服务器状态日报" {
		t.Errorf("CustomTitle = %q, want \"服务器状态日报\"", cfg.CustomTitle)
	}
}

func TestLoadConfigCustom(t *testing.T) {
	t.Setenv("WECOM_WEBHOOK", "https://example.invalid/fake-webhook")
	t.Setenv("REPORT_TIME", "08:30")
	t.Setenv("CPU_THRESHOLD", "90")
	t.Setenv("MEM_THRESHOLD", "70")
	t.Setenv("DISK_THRESHOLD", "60")
	t.Setenv("CUSTOM_TITLE", "测试标题")
	cfg := loadConfig()
	if cfg.ReportTime != "08:30" {
		t.Errorf("ReportTime = %q, want \"08:30\"", cfg.ReportTime)
	}
	if cfg.CPUAlert != 90 || cfg.MemAlert != 70 || cfg.DiskAlert != 60 {
		t.Errorf("阈值 = %d/%d/%d, want 90/70/60", cfg.CPUAlert, cfg.MemAlert, cfg.DiskAlert)
	}
	if cfg.CustomTitle != "测试标题" {
		t.Errorf("CustomTitle = %q, want \"测试标题\"", cfg.CustomTitle)
	}
}

// sendWecomMarkdown：用 httptest 假服务器，不发真实请求。
func TestSendWecomMarkdownSuccess(t *testing.T) {
	var gotMethod, gotCT, gotContent string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		gotCT = r.Header.Get("Content-Type")
		body, _ := io.ReadAll(r.Body)
		var p struct {
			MsgType  string            `json:"msgtype"`
			Markdown map[string]string `json:"markdown"`
		}
		if err := json.Unmarshal(body, &p); err == nil {
			gotContent = p.Markdown["content"]
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"errcode":0,"errmsg":"ok"}`))
	}))
	defer srv.Close()

	if err := sendWecomMarkdown(srv.URL, "## 测试\n- **CPU**: 10%"); err != nil {
		t.Fatalf("sendWecomMarkdown = %v, want nil", err)
	}
	if gotMethod != http.MethodPost {
		t.Errorf("请求方法 = %q, want POST", gotMethod)
	}
	if gotCT != "application/json" {
		t.Errorf("Content-Type = %q, want application/json", gotCT)
	}
	if gotContent != "## 测试\n- **CPU**: 10%" {
		t.Errorf("推送内容 = %q", gotContent)
	}
}

func TestSendWecomMarkdownHTTPError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte("boom"))
	}))
	defer srv.Close()

	err := sendWecomMarkdown(srv.URL, "x")
	if err == nil {
		t.Fatal("HTTP 500 应返回错误")
	}
	if !strings.Contains(err.Error(), "wecom http 500") {
		t.Errorf("错误信息 = %q, want 包含 wecom http 500", err.Error())
	}
}

func TestSendWecomMarkdownErrcode(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"errcode":40001,"errmsg":"invalid credential"}`))
	}))
	defer srv.Close()

	err := sendWecomMarkdown(srv.URL, "x")
	if err == nil {
		t.Fatal("errcode 非 0 应返回错误")
	}
	if !strings.Contains(err.Error(), "wecom errcode 40001") {
		t.Errorf("错误信息 = %q, want 包含 wecom errcode 40001", err.Error())
	}
}

func TestSendWecomMarkdownInvalidJSONBody(t *testing.T) {
	// HTTP 200 但响应体不是 JSON：按实现，解析失败即视为成功。
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("not-json"))
	}))
	defer srv.Close()

	if err := sendWecomMarkdown(srv.URL, "x"); err != nil {
		t.Errorf("非 JSON 的 200 响应应返回 nil,实际 %v", err)
	}
}

func TestSendWecomMarkdownUnreachable(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	url := srv.URL
	srv.Close() // 端口已关闭，连接会被拒绝

	if err := sendWecomMarkdown(url, "x"); err == nil {
		t.Error("连接被拒应返回错误")
	}
}

// firstPrivateIPv4：只断言返回合法 IPv4，且在内网段或为回退值。
func TestFirstPrivateIPv4(t *testing.T) {
	ipStr := firstPrivateIPv4()
	ip := net.ParseIP(ipStr)
	if ip == nil {
		t.Fatalf("firstPrivateIPv4 = %q, 不是合法 IP", ipStr)
	}
	if ip.To4() == nil {
		t.Errorf("firstPrivateIPv4 = %q, 不是 IPv4", ipStr)
	}
	if !ip.IsPrivate() && !ip.IsLoopback() {
		t.Errorf("firstPrivateIPv4 = %q, 既非内网 IP 也非回退 127.0.0.1", ipStr)
	}
}

func TestOSVersion(t *testing.T) {
	if got := osVersion(); got == "" {
		t.Error("osVersion 不应返回空字符串")
	}
}

func TestDetectVirtualization(t *testing.T) {
	if got := detectVirtualization(); got == "" {
		t.Error("detectVirtualization 不应返回空字符串")
	}
}

// buildReport：冒烟测试，只断言报告包含各章节（会真实采集本机指标，不发请求）。
func TestBuildReportContainsSections(t *testing.T) {
	cfg := &Config{
		WecomWebhook: "https://example.invalid/fake-webhook",
		ReportTime:   "-",
		CPUAlert:     80,
		MemAlert:     80,
		DiskAlert:    80,
		CustomTitle:  "测试标题",
	}
	out := buildReport(cfg)
	for _, want := range []string{"测试标题", "主机名", "内网IP", "推送时间", "CPU", "内存", "磁盘", "系统", "运行"} {
		if !strings.Contains(out, want) {
			t.Errorf("buildReport 缺少 %q:\n%s", want, out)
		}
	}
	if strings.Contains(out, "计划时刻") {
		t.Error("ReportTime 为 - 时不应出现计划时刻")
	}
}

func TestBuildReportWithReportTime(t *testing.T) {
	cfg := &Config{
		WecomWebhook: "https://example.invalid/fake-webhook",
		ReportTime:   "08:30",
		CPUAlert:     80,
		MemAlert:     80,
		DiskAlert:    80,
		CustomTitle:  "测试标题",
	}
	out := buildReport(cfg)
	if !strings.Contains(out, "计划时刻") || !strings.Contains(out, "08:30") {
		t.Errorf("ReportTime 为 08:30 时应出现计划时刻:\n%s", out)
	}
}

func TestBuildWecomPayloadEdgeCases(t *testing.T) {
	// 空内容
	b := buildWecomPayload("")
	var p struct {
		MsgType  string            `json:"msgtype"`
		Markdown map[string]string `json:"markdown"`
	}
	if err := json.Unmarshal(b, &p); err != nil {
		t.Fatalf("空内容 payload 不是合法 JSON: %v", err)
	}
	if p.Markdown["content"] != "" {
		t.Errorf("空内容 content = %q, want \"\"", p.Markdown["content"])
	}
	// 特殊字符往返
	text := "引号 \"test\" 反斜杠 \\ 换行\n emoji 🚀 <>&"
	b = buildWecomPayload(text)
	if err := json.Unmarshal(b, &p); err != nil {
		t.Fatalf("特殊字符 payload 不是合法 JSON: %v", err)
	}
	if p.Markdown["content"] != text {
		t.Errorf("特殊字符 content 往返不一致:\n got %q\nwant %q", p.Markdown["content"], text)
	}
	// 顶层只有 msgtype 和 markdown 两个键
	var m map[string]json.RawMessage
	if err := json.Unmarshal(b, &m); err != nil {
		t.Fatalf("unmarshal map: %v", err)
	}
	if len(m) != 2 {
		t.Errorf("顶层键数量 = %d, want 2", len(m))
	}
}
