package main
import (
 "encoding/base64"
 "os"
 "testing"
)
func TestCommandRecordingRoundtripAndPrune(t *testing.T){
 t.Setenv("OCTOP_HOME",t.TempDir())
 a:=&App{}
 data:=buildMaohaoWav(make([]byte,3200))
 if !validMouseWAV(data){t.Fatal("valid PCM16 WAV rejected")}
 encoded:=base64.StdEncoding.EncodeToString(data)
 if err:=a.SaveMouseCommandRecording("wechat",encoded);err!=nil{t.Fatal(err)}
 got,err:=a.GetMouseCommandRecording("wechat")
 if err!=nil||got!=encoded{t.Fatalf("readback: %v",err)}
 if err:=a.SaveMouseCommands([]MouseCommand{});err!=nil{t.Fatal(err)}
 got,err=a.GetMouseCommandRecording("wechat")
 if err!=nil||got!=""{t.Fatal("deleted command left audio behind")}
}
func TestCommandRecordingValidation(t *testing.T){
 for _,id:=range []string{"../bad","bad/else","..",""}{
  if validMouseRecordID(id){t.Fatalf("unsafe path: %s",id)}
 }
 wav:=buildMaohaoWav(make([]byte,3200))
 wav[24]=0
 if validMouseWAV(wav){t.Fatal("invalid sample rate")}
 if _,err:=readCommandSampleInput(base64.StdEncoding.EncodeToString(wav));err==nil{
  t.Fatal("invalid WAV accepted")
 }
 if _,err:=os.Stat(".");err!=nil{t.Fatal(err)}
}
