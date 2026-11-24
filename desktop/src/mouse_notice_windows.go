//go:build windows

package main

import (
	"runtime"
	"sync"
	"syscall"
	"time"
	"unsafe"
)

const mouseTranscriptVisibleFor = 3 * time.Second

var mouseNoticeState struct {
	sync.Mutex
	phase, text     string
	transcript      string
	transcriptUntil time.Time
}

func mouseNoticeSet(phase, text string) {
	mouseNoticeState.Lock()
	defer mouseNoticeState.Unlock()
	if phase == "recognized" {
		if text == "" {
			return
		}
		mouseNoticeState.transcript = text
		mouseNoticeState.transcriptUntil = time.Now().Add(mouseTranscriptVisibleFor)
		mouseNoticeState.phase = "recognized"
		mouseNoticeState.text = ""
		return
	}
	// A new utterance invalidates the previous one; ordinary processing updates
	// must not erase a recognized transcript before the three-second timeout.
	if phase == "recording" || phase == "transcribing" || phase == "model" {
		mouseNoticeState.transcript = ""
		mouseNoticeState.transcriptUntil = time.Time{}
	}
	if mouseNoticeState.transcript != "" {
		return
	}
	mouseNoticeState.phase, mouseNoticeState.text = phase, text
}

func mouseNoticeClearError(text string) {
	mouseNoticeState.Lock()
	defer mouseNoticeState.Unlock()
	if mouseNoticeState.phase == "error" && mouseNoticeState.text == text {
		mouseNoticeState.phase, mouseNoticeState.text = "ready", ""
	}
}

// All timing decisions use one snapshot, making races between audio, status
// changes and the UI loop predictable. Existing transcription is never
// truncated; the native STATIC control wraps longer text across lines.
func mouseNoticeSnapshot(at time.Time) (string, string) {
	mouseNoticeState.Lock()
	defer mouseNoticeState.Unlock()
	if mouseNoticeState.transcript != "" {
		if at.Before(mouseNoticeState.transcriptUntil) {
			return "recognized", "识别结果：" + mouseNoticeState.transcript
		}
		mouseNoticeState.transcript = ""
		mouseNoticeState.transcriptUntil = time.Time{}
		mouseNoticeState.phase, mouseNoticeState.text = "ready", ""
	}
	return mouseNoticeState.phase, mouseNoticeState.text
}

type noticeRect struct{ Left, Top, Right, Bottom int32 }
type noticePoint struct{ X, Y int32 }
type noticeMessage struct {
	Hwnd           uintptr
	Message        uint32
	WParam, LParam uintptr
	Time           uint32
	Pt             noticePoint
	Private        uint32
}

func mouseNoticeLoop() {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	create := u32.NewProc("CreateWindowExW")
	cls, _ := syscall.UTF16PtrFromString("STATIC")
	empty, _ := syscall.UTF16PtrFromString("")
	// No activate, tool window, topmost. Wrapped Unicode text near the tray.
	const noticeWidth, noticeHeight = 520, 104
	hwnd, _, _ := create.Call(0x08000000|0x80|0x8,
		uintptr(unsafe.Pointer(cls)), uintptr(unsafe.Pointer(empty)),
		0x80000000|0x00800000|0, 0, 0, noticeWidth, noticeHeight, 0, 0, 0, 0)
	if hwnd == 0 {
		return
	}
	defer u32.NewProc("DestroyWindow").Call(hwnd)
	setText := u32.NewProc("SetWindowTextW")
	position := u32.NewProc("SetWindowPos")
	workArea := u32.NewProc("SystemParametersInfoW")
	peek := u32.NewProc("PeekMessageW")
	translate := u32.NewProc("TranslateMessage")
	dispatch := u32.NewProc("DispatchMessageW")
	i := 0
	for {
		var msg noticeMessage
		for {
			ok, _, _ := peek.Call(uintptr(unsafe.Pointer(&msg)), 0, 0, 0, 1)
			if ok == 0 {
				break
			}
			translate.Call(uintptr(unsafe.Pointer(&msg)))
			dispatch.Call(uintptr(unsafe.Pointer(&msg)))
		}
		phase, text := mouseNoticeSnapshot(time.Now())
		if phase == "recording" || phase == "transcribing" || phase == "executing" ||
			phase == "error" || phase == "model" || phase == "recognized" {
			if phase == "executing" {
				text = "正在" + text
			}
			if phase != "error" && phase != "recognized" {
				text += []string{".", "..", "..."}[i%3]
			}
			value, err := syscall.UTF16PtrFromString(text)
			if err == nil {
				setText.Call(hwnd, uintptr(unsafe.Pointer(value)))
			}
			var area noticeRect
			workArea.Call(0x0030, 0, uintptr(unsafe.Pointer(&area)), 0)
			position.Call(hwnd, ^uintptr(0), uintptr(area.Right-noticeWidth-16),
				uintptr(area.Bottom-noticeHeight-16), noticeWidth, noticeHeight, 0x0010|0x0040)
		} else {
			showWindowProc.Call(hwnd, 0)
		}
		i++
		time.Sleep(200 * time.Millisecond)
	}
}
