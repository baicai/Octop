//go:build windows
package main
import(
"errors"
"fmt"
"os"
"os/exec"
"path/filepath"
"strings"
"syscall"
"unicode/utf16"
"unsafe"
"golang.org/x/sys/windows/registry"
)
var (u32=syscall.NewLazyDLL("user32.dll");foregroundProc=u32.NewProc("GetForegroundWindow");titleProc=u32.NewProc("GetWindowTextW");inputProc=u32.NewProc("SendInput");shellProc=syscall.NewLazyDLL("shell32.dll").NewProc("ShellExecuteW"))
func maohaoRegisterLoginStart()error{
exe,err:=os.Executable();if err!=nil{return err}
k,_,err:=registry.CreateKey(registry.CURRENT_USER,`SoftwareMicrosoftWindowsCurrentVersionRun`,registry.SET_VALUE)
if err!=nil{return err};defer k.Close()
return k.SetStringValue("OctopDemoMouseAgent",fmt.Sprintf("%q --maohao-agent",exe))
}
func maohaoForegroundWindow()uintptr{h,_,_:=foregroundProc.Call();return h}
func maohaoIsOctopForeground(hwnd uintptr)bool{
buf:=make([]uint16,512);n,_,_:=titleProc.Call(hwnd,uintptr(unsafe.Pointer(&buf[0])),uintptr(len(buf)))
return n>0&&strings.Contains(strings.ToLower(syscall.UTF16ToString(buf)),"octop")
}
type unicodeInput struct{Kind uint32;_ uint32;Payload [32]byte}
func maohaoTypeUnicode(s string)error{
for _,char:=range utf16.Encode([]rune(s)){
for _,up:=range []bool{false,true}{
var v unicodeInput;v.Kind=1;v.Payload[2]=byte(char);v.Payload[3]=byte(char>>8);v.Payload[4]=4
if up{v.Payload[4]|=2}
n,_,err:=inputProc.Call(1,uintptr(unsafe.Pointer(&v)),unsafe.Sizeof(v))
if n!=1{return fmt.Errorf("SendInput failed: %v",err)}
}}
return nil
}
func maohaoOpenWeChat()error{
for _,root:=range []string{os.Getenv("LOCALAPPDATA"),os.Getenv("ProgramFiles"),os.Getenv("ProgramFiles(x86)")}{
if root==""{continue}
for _,p:=range []string{filepath.Join(root,"Tencent","WeChat","WeChat.exe"),filepath.Join(root,"Tencent","Weixin","Weixin.exe"),filepath.Join(root,"Programs","Tencent","Weixin","Weixin.exe")} {
if fi,err:=os.Stat(p);err==nil&&!fi.IsDir(){
cmd:=exec.Command(p);prepareMaohaoCommand(cmd)
if cmd.Start()==nil{go func(){_=cmd.Wait()}();return nil}
}
}}
verb,_:=syscall.UTF16PtrFromString("open")
for _,exe:=range []string{"Weixin.exe","WeChat.exe"}{
name,_:=syscall.UTF16PtrFromString(exe)
h,_,_:=shellProc.Call(0,uintptr(unsafe.Pointer(verb)),uintptr(unsafe.Pointer(name)),0,0,1)
if h>32{return nil}
}
return errors.New("WeChat not installed in supported paths")
}
