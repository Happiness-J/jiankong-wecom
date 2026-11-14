package main

import (
	"encoding/json"
	"testing"
)

func TestHuman(t *testing.T) {
	cases := []struct {
		in   uint64
		want string
	}{
		{0, "0 B"},
		{512, "512 B"},
		{1024, "1.0 KB"},
		{1536, "1.5 KB"},
		{1024 * 1024, "1.0 MB"},
		{8 * 1024 * 1024 * 1024, "8.0 GB"},
	}
	for _, c := range cases {
		if got := human(c.in); got != c.want {
			t.Errorf("human(%d) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestIcon(t *testing.T) {
	if icon(50, 80) != "✅" {
		t.Error("icon below threshold should be ✅")
	}
	if icon(80, 80) != "✅" {
		t.Error("icon at threshold should be ✅")
	}
	if icon(81, 80) != "⚠️" {
		t.Error("icon above threshold should be ⚠️")
	}
}

func TestBuildWecomPayload(t *testing.T) {
	b := buildWecomPayload("## 标题\n- **CPU**: 10%")
	var got struct {
		MsgType  string            `json:"msgtype"`
		Markdown map[string]string `json:"markdown"`
	}
	if err := json.Unmarshal(b, &got); err != nil {
		t.Fatalf("payload is not valid JSON: %v", err)
	}
	if got.MsgType != "markdown" {
		t.Errorf("msgtype = %q, want markdown", got.MsgType)
	}
	if got.Markdown["content"] != "## 标题\n- **CPU**: 10%" {
		t.Errorf("content mismatch: %q", got.Markdown["content"])
	}
}
