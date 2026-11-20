//go:build windows

package main
import(
 "errors"
 "os"
 "os/exec"
 "path/filepath"
 "syscall"
 "unsafe"
 "golang.org/x/sys/windows"
)
func replaceMouseCommandFile(src,dst string)error{
 a,e:=windows.UTF16PtrFromString(src);if e!=nil{return e}
 b,e:=windows.UTF16PtrFromString(dst);if e!=nil{return e}
 return windows.MoveFileEx(a,b,windows.MOVEFILE_REPLACE_EXISTING|windows.MOVEFILE_WRITE_THROUGH)
}
func mouseOpenURL(target string)error{
 verb,_:=syscall.UTF16PtrFromString("open");arg,e:=syscall.UTF16PtrFromString(target);if e!=nil{return e}
 h,_,_:=shellProc.Call(0,uintptr(unsafe.Pointer(verb)),uintptr(unsafe.Pointer(arg)),0,0,1)
 if h<=32{return errors.New("无法打开网址")};return nil
}
func mouseLaunchApp(target string)error{
 if target=="wechat"{return maohaoOpenWeChat()}
 if target=="browser"{return mouseOpenURL("https://www.baidu.com/")}
 name:=""
 switch target{case "calculator":name="calc.exe";case "notepad":name="notepad.exe";default:return errors.New("应用不受支持")}
 cmd:=exec.Command(filepath.Join(os.Getenv("SystemRoot"),"System32",name))
 if e:=cmd.Start();e!=nil{return e};go func(){_=cmd.Wait()}();return nil
}
func mouseCloseApp(target string)error{
 if target!="wechat"{return errors.New("应用不受支持")}
 for _,n:=range []string{"WeChatMainWndForPC","WeixinMainWndForPC"}{
 p,_:=syscall.UTF16PtrFromString(n);h,_,_:=findWindowProc.Call(uintptr(unsafe.Pointer(p)),0)
 if h!=0{ok,_,_:=u32.NewProc("PostMessageW").Call(h,0x0010,0,0);if ok!=0{return nil}}
 }
 return errors.New("没有找到微信窗口")
}
