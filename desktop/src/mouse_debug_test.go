package main

import (
	"os"
	"strings"
	"testing"
)

func TestMaohaoSpeechRouteCommandFirst(t *testing.T) {
	items := defaultMouseCommands()
	route, command := maohaoSpeechRoute(items, "打开微信。", true)
	if route != "command" || command.ID != "wechat" {
		t.Fatalf("Octop-front command skipped: route=%q id=%q", route, command.ID)
	}
	route, command = maohaoSpeechRoute(items, "打开计算器", false)
	if route != "command" || command.ID != "calculator" {
		t.Fatalf("system command bypassed: route=%q", route)
	}
	route, _ = maohaoSpeechRoute(items, "普通聊天内容", true)
	if route != "chat" {
		t.Fatal("unmatched Octop-front speech should go to chat")
	}
	route, _ = maohaoSpeechRoute(items, "普通聊天内容", false)
	if route != "dictate" {
		t.Fatal("unmatched external speech should be typed")
	}
	route, _ = maohaoSpeechRoute(items, "  ", false)
	if route != "empty" {
		t.Fatal("empty ASR result should not execute")
	}
	route, _ = maohaoSpeechRoute(items, "关闭微信", true)
	if route == "command" {
		t.Fatal("disabled command unexpectedly matched")
	}
}

func TestMaohaoDebugFileAndPrivacy(t *testing.T) {
	t.Setenv("OCTOP_HOME", t.TempDir())
	const spoken = "我的银行卡验证码是123456"
	preview, characters, hash := maohaoDebugTranscript(spoken)
	if preview != "" || characters == 0 || len(hash) != 12 {
		t.Fatal("private dictation exposed in debug preview")
	}
	preview, characters, hash = maohaoDebugTranscript("打开微信")
	if preview != "打开微信" || characters != 4 || len(hash) != 12 {
		t.Fatal("explicit command should have diagnostic preview")
	}
	maohaoDebug("unit.test", "stage", "speech", "bytes", 1234)
	data, err := os.ReadFile(maohaoDebugPath())
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "unit.test") || !strings.Contains(string(data), "\"bytes\":1234") {
		t.Fatal("diagnostic event missing")
	}
	if strings.Contains(string(data), spoken) {
		t.Fatal("private dictation in debug log")
	}
	tail, err := (&App{}).GetMaohaoDebugTail()
	if err != nil || !strings.Contains(tail, "unit.test") {
		t.Fatalf("debug log not viewable: %v", err)
	}
}
