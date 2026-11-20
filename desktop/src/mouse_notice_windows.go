//go:build windows

package main

import (
 "sync"
 "time"
 "runtime"
 "syscall"
 "unsafe"
)
var mouseNoticeState struct{sync.Mutex;phase,text string}
func mouseNoticeSet(phase,text string){mouseNoticeState.Lock();mouseNoticeState.phase=phase;mouseNoticeState.text=text;mouseNoticeState.Unlock()}
func mouseNoticeClearError(text string){mouseNoticeState.Lock();defer mouseNoticeState.Unlock();if mouseNoticeState.phase=="error"&&mouseNoticeState.text==text{mouseNoticeState.phase="ready";mouseNoticeState.text=""}}
type noticeRect struct{Left,Top,Right,Bottom int32}
type noticePoint struct{X,Y int32}
type noticeMessage struct{Hwnd uintptr;Message uint32;WParam,LParam uintptr;Time uint32;Pt noticePoint;Private uint32}
func mouseNoticeLoop(){
 runtime.LockOSThread();defer runtime.UnlockOSThread()
 create:=u32.NewProc("CreateWindowExW")
 cls,_:=syscall.UTF16PtrFromString("STATIC");empty,_:=syscall.UTF16PtrFromString("")
 // No activate, tool window, topmost. Simple native text overlay above taskbar.
 hwnd,_,_:=create.Call(0x08000000|0x80|0x8,uintptr(unsafe.Pointer(cls)),uintptr(unsafe.Pointer(empty)),0x80000000|0x00800000|1,0,0,310,48,0,0,0,0)
 if hwnd==0{return}
 defer u32.NewProc("DestroyWindow").Call(hwnd)
 setText:=u32.NewProc("SetWindowTextW");position:=u32.NewProc("SetWindowPos");workArea:=u32.NewProc("SystemParametersInfoW")
 peek:=u32.NewProc("PeekMessageW");translate:=u32.NewProc("TranslateMessage");dispatch:=u32.NewProc("DispatchMessageW")
 i:=0
 for{
 var msg noticeMessage
 for{ok,_,_:=peek.Call(uintptr(unsafe.Pointer(&msg)),0,0,0,1);if ok==0{break};translate.Call(uintptr(unsafe.Pointer(&msg)));dispatch.Call(uintptr(unsafe.Pointer(&msg)))}
 mouseNoticeState.Lock();phase,text:=mouseNoticeState.phase,mouseNoticeState.text;mouseNoticeState.Unlock()
 if phase=="recording"||phase=="transcribing"||phase=="executing"||phase=="error"{
 if phase=="executing"{text="正在"+text}
 if phase!="error"{text+=[]string{".","..","..."}[i%3]}
 p,e:=syscall.UTF16PtrFromString(text);if e==nil{setText.Call(hwnd,uintptr(unsafe.Pointer(p)))}
 var area noticeRect;workArea.Call(0x0030,0,uintptr(unsafe.Pointer(&area)),0)
 position.Call(hwnd,^uintptr(0),uintptr(area.Right-326),uintptr(area.Bottom-64),310,48,0x0010|0x0040)
 }else{showWindowProc.Call(hwnd,0)}
 i++;time.Sleep(200*time.Millisecond)
 }
}
