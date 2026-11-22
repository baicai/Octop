package main
import (
 "encoding/json"
 "errors"
 "net/url"
 "os"
 "path/filepath"
 "strings"
 "sync"
 "time"
)
type MouseCommand struct {
 ID string `json:"id"`
 Name string `json:"name"`
 Aliases []string `json:"aliases"`
 Action string `json:"action"`
 Target string `json:"target"`
 Enabled bool `json:"enabled"`
}
var commandLock sync.Mutex
func defaultMouseCommands() []MouseCommand {
 return []MouseCommand{
 {"wechat","打开微信",[]string{"开启微信","启动微信","请打开微信","帮我打开微信","请帮我打开微信"},"open-app","wechat",true},
 {"calculator","打开计算器",[]string{"启动计算器","开启计算器"},"open-app","calculator",true},
 {"notepad","打开记事本",[]string{"启动记事本","开启记事本"},"open-app","notepad",true},
 {"browser","打开浏览器",[]string{"启动浏览器","开启浏览器"},"open-app","browser",true},
 {"baidu","访问百度",[]string{"打开百度","访问百度网站"},"open-url","https://www.baidu.com/",true},
 {"close-wechat","关闭微信",[]string{"退出微信"},"close-app","wechat",false},
 }
}
func mouseCommandPath()string{return filepath.Join(octopHome(),"mouse-commands.json")}
func normalizeMouseCommand(s string)string{
 return strings.ToLower(strings.Trim(strings.TrimSpace(s),"。！!，,？? "))
}
func readMouseCommands()([]MouseCommand,error){
 raw,err:=os.ReadFile(mouseCommandPath())
 if os.IsNotExist(err){
  maohaoDebug("commands.defaults_used","count",len(defaultMouseCommands()))
  return defaultMouseCommands(),nil
}
 if err!=nil{return nil,err}
 var items []MouseCommand
 if err=json.Unmarshal(raw,&items);err!=nil{
  maohaoDebug("commands.json_invalid","error",maohaoDebugErr(err))
  return nil,err
}
 if err=validateMouseCommands(items);err!=nil{
  maohaoDebug("commands.config_invalid","error",maohaoDebugErr(err))
  return nil,err
}
return items,nil
}
func validateMouseCommands(items []MouseCommand)error{
 if len(items)>300{return errors.New("最多支持300条指令")}
 ids:=map[string]bool{};phrases:=map[string]bool{}
 for _,c:=range items{
 if c.ID==""||ids[c.ID]||normalizeMouseCommand(c.Name)==""||len(c.Name)>300||len(c.Aliases)>20{return errors.New("指令ID、名称或别名无效")}
 ids[c.ID]=true
 for _,p:=range append([]string{c.Name},c.Aliases...){
 n:=normalizeMouseCommand(p)
 if n==""||len(n)>300||phrases[n]{return errors.New("触发词为空或重复："+p)}
 phrases[n]=true
 }
 switch c.Action{
 case "open-app":
 if c.Target!="wechat"&&c.Target!="calculator"&&c.Target!="notepad"&&c.Target!="browser"{return errors.New("请选择受支持的应用")}
 case "close-app":
 if c.Target!="wechat"{return errors.New("当前仅支持关闭微信窗口")}
 case "open-url":
 u,e:=url.Parse(c.Target);if e!=nil||u.Hostname()==""||(u.Scheme!="https"&&u.Scheme!="http")||u.User!=nil{return errors.New("请输入有效的HTTP/HTTPS网址")}
 default:return errors.New("不支持的操作")
 }
 }
 return nil
}
func findMouseCommand(items []MouseCommand,s string)(MouseCommand,bool){
 n:=normalizeMouseCommand(s)
 for _,c:=range items{
 if !c.Enabled{continue}
 for _,p:=range append([]string{c.Name},c.Aliases...){if n==normalizeMouseCommand(p){return c,true}}
 }
 return MouseCommand{},false
}
func (a *App) ListMouseCommands()([]MouseCommand,error){
 commandLock.Lock();defer commandLock.Unlock()
 return readMouseCommands()
}
func (a *App) SaveMouseCommands(items []MouseCommand)error{
 commandLock.Lock();defer commandLock.Unlock()
 if err:=validateMouseCommands(items);err!=nil{return err}
 if err:=os.MkdirAll(octopHome(),0700);err!=nil{return err}
 raw,err:=json.MarshalIndent(items,"","  ");if err!=nil{return err}
 f,err:=os.CreateTemp(octopHome(),"commands-*.tmp");if err!=nil{return err}
 defer os.Remove(f.Name())
 if _,err=f.Write(raw);err!=nil{f.Close();return err}
 if err=f.Close();err!=nil{return err}
 if err:=replaceMouseCommandFile(f.Name(),mouseCommandPath());err!=nil{
 maohaoDebug("commands.save_failed","error",maohaoDebugErr(err))
 return err
}
maohaoDebug("commands.saved","count",len(items))
return pruneMouseRecordings(items)
}
func (a *App) TestMouseCommand(id string,confirmed bool)error{
 commandLock.Lock();items,err:=readMouseCommands();commandLock.Unlock()
 if err!=nil{return err}
 for _,c:=range items{if c.ID==id{
 maohaoDebug("commands.test_requested","id",id,"action",c.Action)
 if c.Action=="close-app"&&!confirmed{return errors.New("请确认关闭窗口")}
 err:=executeMouseCommand(c)
 if err!=nil{maohaoDebug("commands.test_failed","id",id,"error",maohaoDebugErr(err))}
 if err==nil{maohaoDebug("commands.test_ok","id",id)}
 return err
 }}
 return errors.New("指令不存在")
}
func executeMouseCommand(c MouseCommand)error{
 switch c.Action{
 case "open-app":return mouseLaunchApp(c.Target)
 case "open-url":return mouseOpenURL(c.Target)
 case "close-app":return mouseCloseApp(c.Target)
 }
 return errors.New("不支持的操作")
}
func mouseCommandNotice(phase,text string){
 mouseNoticeSet(phase,text)
 if phase=="error" {time.AfterFunc(time.Second,func(){mouseNoticeClearError(text)})}
}
