package main

// Local-only diagnostics for the Maohao mouse -> ASR -> command pipeline.
// Never log raw PCM, auth tokens, device identifiers, arbitrary dictated text,
// or full URLs. These logs are not uploaded to any network service.
import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
	"unicode/utf8"
)

const maohaoDebugLimitBytes = 2 * 1024 * 1024

var maohaoDebugMu sync.Mutex

func maohaoDebugPath() string {
	return filepath.Join(octopHome(), "logs", "maohao-debug.log")
}

func maohaoDebug(stage string, args ...any) {
	if len(args)%2 != 0 {
		return
	}
	info := map[string]any{}
	for i := 0; i < len(args); i += 2 {
		key, ok := args[i].(string)
		if !ok || key == "" {
			continue
		}
		info[key] = args[i+1]
	}
	row := map[string]any{
		"at":    time.Now().Format(time.RFC3339Nano),
		"pid":   os.Getpid(),
		"stage": stage,
		"data":  info,
	}
	line, err := json.Marshal(row)
	if err != nil {
		return
	}
	if len(line) > 4096 {
		return
	}
	line = append(line, '\n')
	maohaoDebugMu.Lock()
	defer maohaoDebugMu.Unlock()
	path := maohaoDebugPath()
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return
	}
	// Keep a small rolling local history. Other Octop processes may append to
	// the old file during rotation; each diagnostic entry is one short write.
	if stat, err := os.Stat(path); err == nil && stat.Size()+int64(len(line)) > maohaoDebugLimitBytes {
		_ = os.Remove(path + ".1")
		_ = os.Rename(path, path+".1")
	}
	file, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0600)
	if err != nil {
		return
	}
	_, _ = file.Write(line)
	_ = file.Close()
}

// Recognized text is private. For a likely command we keep a short preview
// to diagnose ASR mismatches; ordinary dictation stays length+hash only.
func maohaoDebugTranscript(text string) (string, int, string) {
	clean := strings.TrimSpace(text)
	hash := sha256.Sum256([]byte(clean))
	sum := hex.EncodeToString(hash[:6])
	length := utf8.RuneCountInString(clean)
	prefixes := []string{"打开", "开启", "启动", "关闭", "退出", "访问", "请打开", "帮我打开", "请帮我打开"}
	for _, prefix := range prefixes {
		if strings.HasPrefix(clean, prefix) {
			runes := []rune(clean)
			if len(runes) > 60 {
				runes = runes[:60]
			}
			return string(runes), length, sum
		}
	}
	return "", length, sum
}

func maohaoDebugErr(err error) string {
	if err == nil {
		return ""
	}
	// Errors can contain local filenames; they never include captured audio.
	message := fmt.Sprint(err)
	if len(message) > 300 {
		message = message[:300]
	}
	return message
}

// GetMaohaoDebugTail is only exposed through the local desktop bridge.
// It reads a bounded number of recent entries, never submits them remotely.
func (a *App) GetMaohaoDebugTail() (string, error) {
	data, err := os.ReadFile(maohaoDebugPath())
	if os.IsNotExist(err) {
		return "尚无诊断日志。请使用鼠标语音键说话后再刷新。", nil
	}
	if err != nil {
		return "", err
	}
	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	if len(lines) > 150 {
		lines = lines[len(lines)-150:]
	}
	return strings.Join(lines, "\n"), nil
}
