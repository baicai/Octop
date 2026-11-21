package main
import "testing"
func TestMouseCommandDefaults(t *testing.T){
 items:=defaultMouseCommands()
 if err:=validateMouseCommands(items);err!=nil{t.Fatal(err)}
 for _,s:=range []string{"打开微信","开启微信","启动微信。"}{
 c,ok:=findMouseCommand(items,s);if !ok||c.Target!="wechat"{t.Fatal(s)}
 }
 if _,ok:=findMouseCommand(items,"关闭微信");ok{t.Fatal("disabled matched")}
 if _,ok:=findMouseCommand(items,"请告诉我如何打开微信");ok{t.Fatal("substring matched")}
 items[1].Aliases=[]string{"启动微信"}
 if validateMouseCommands(items)==nil{t.Fatal("duplicate accepted")}
}
