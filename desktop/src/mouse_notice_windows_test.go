//go:build windows

package main

import (
	"testing"
	"time"
)

func TestMaohaoTranscriptNoticeThreeSeconds(t *testing.T) {
	mouseNoticeSet("transcribing", "识别中")
	mouseNoticeSet("recognized", "打开微信。")
	now := time.Now()
	phase, text := mouseNoticeSnapshot(now)
	if phase != "recognized" || text != "识别结果：打开微信。" {
		t.Fatalf("incorrect Chinese transcript: %s %q", phase, text)
	}
	// These updates happen immediately after recognition while the command
	// executes and after it returns. Neither may erase the ASR diagnostic text.
	mouseNoticeSet("executing", "打开微信")
	mouseNoticeSet("ready", "")
	phase, text = mouseNoticeSnapshot(now.Add(2900 * time.Millisecond))
	if phase != "recognized" || text != "识别结果：打开微信。" {
		t.Fatalf("recognized text disappeared early: %s %q", phase, text)
	}
	phase, text = mouseNoticeSnapshot(now.Add(3100 * time.Millisecond))
	if phase != "ready" || text != "" {
		t.Fatalf("recognized text not closed after three seconds: %s %q", phase, text)
	}
}

func TestMaohaoTranscriptNoticeNewRecordingSupersedesOld(t *testing.T) {
	mouseNoticeSet("recognized", "第一条语音")
	mouseNoticeSet("recording", "正在说话")
	phase, text := mouseNoticeSnapshot(time.Now())
	if phase != "recording" || text != "正在说话" {
		t.Fatalf("new recording did not override stale recognition: %s %q", phase, text)
	}
	mouseNoticeSet("recognized", "第二条语音")
	phase, text = mouseNoticeSnapshot(time.Now())
	if phase != "recognized" || text != "识别结果：第二条语音" {
		t.Fatalf("latest recognized text missing: %s %q", phase, text)
	}
	mouseNoticeSet("ready", "")
	phase, text = mouseNoticeSnapshot(time.Now().Add(3100 * time.Millisecond))
	if phase != "ready" || text != "" {
		t.Fatalf("latest result persisted too long: %s %q", phase, text)
	}
}

func TestMaohaoTranscriptNoticeErrorDoesNotClobberResult(t *testing.T) {
	mouseNoticeSet("recognized", "访问网页。")
	mouseNoticeSet("error", "指令执行失败")
	mouseNoticeClearError("指令执行失败")
	phase, text := mouseNoticeSnapshot(time.Now().Add(2 * time.Second))
	if phase != "recognized" || text != "识别结果：访问网页。" {
		t.Fatalf("failure hid the recognized words: %s %q", phase, text)
	}
	phase, text = mouseNoticeSnapshot(time.Now().Add(4 * time.Second))
	if phase != "ready" || text != "" {
		t.Fatalf("result was not dismissed: %s %q", phase, text)
	}
}
