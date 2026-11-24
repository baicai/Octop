package main

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestSenseVoiceDecoderPreservesChineseAndRejectsGBK(t *testing.T) {
	const expected = "打开微信。"
	for _, input := range []string{
		"{\"text\":\"打开微信。\"}",
		"{\"text\":\"\\u6253\\u5f00\\u5fae\\u4fe1\\u3002\"}",
	} {
		got, err := decodeSenseVoiceResponse(input)
		if err != nil || got != expected {
			t.Fatalf("decode Chinese: got=%q err=%v", got, err)
		}
		c, ok := findMouseCommand(defaultMouseCommands(), got)
		if !ok || c.ID != "wechat" {
			t.Fatalf("recognized Chinese did not match command: %q", got)
		}
	}
	gbk := []byte{0xb4, 0xf2, 0xbf, 0xaa, 0xce, 0xa2, 0xd0, 0xc5, 0xa1, 0xa3}
	malformed := fmt.Sprintf("{\"text\":\"%s\"}", string(gbk))
	if _, err := decodeSenseVoiceResponse(malformed); err == nil {
		t.Fatal("corrupt GBK bytes were accepted as UTF-8")
	}
	if _, err := decodeSenseVoiceResponse("{\"text\":\"\\ufffd\\ufffd\"}"); err == nil {
		t.Fatal("Unicode replacement characters were accepted")
	}
}

func TestSenseVoiceWorkerEnvOverridesChineseWindowsCodePage(t *testing.T) {
	t.Setenv("PYTHONIOENCODING", "cp936")
	t.Setenv("PYTHONUTF8", "0")
	env := senseVoiceWorkerEnv("C:\\portable")
	got := map[string]string{}
	for _, pair := range env {
		if i := strings.IndexByte(pair, '='); i >= 0 {
			got[strings.ToUpper(pair[:i])] = pair[i+1:]
		}
	}
	if got["PYTHONIOENCODING"] != "utf-8" || got["PYTHONUTF8"] != "1" {
		t.Fatalf("worker stdout not forced UTF-8: %q/%q", got["PYTHONIOENCODING"], got["PYTHONUTF8"])
	}
}

func TestSenseVoicePythonPipeOnWindowsCodePage(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("Windows-specific CP936 test")
	}
	py := filepath.Join("..", "portable", "release", "Octop-windows-amd64", "runtime", "python.exe")
	if _, err := os.Stat(py); err != nil {
		t.Skip("portable Python runtime not available in this checkout")
	}
	snippet := "import json;print(json.dumps({'text':'打开微信。'},ensure_ascii=False),flush=True)"
	legacy := exec.Command(py, "-c", snippet)
	legacy.Env = overlayEnv(os.Environ(), map[string]string{"PYTHONIOENCODING": "cp936", "PYTHONUTF8": "0"})
	raw, err := legacy.Output()
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(raw, []byte{0xce, 0xa2}) {
		t.Fatalf("CP936 reproduction absent; bytes=%d", len(raw))
	}
	if _, err = decodeSenseVoiceResponse(strings.TrimSpace(string(raw))); err == nil {
		t.Fatal("GBK bytes unexpectedly passed UTF-8 decode")
	}
	modern := exec.Command(py, "-c", snippet)
	modern.Env = senseVoiceWorkerEnv(filepath.Dir(py))
	clean, err := modern.Output()
	if err != nil {
		t.Fatal(err)
	}
	got, err := decodeSenseVoiceResponse(strings.TrimSpace(string(clean)))
	if err != nil || got != "打开微信。" {
		t.Fatalf("forced UTF-8 decode failed: %q %v", got, err)
	}
}
