package main
import "testing"
func TestVoiceCommandClassify(t *testing.T) {
cases:=map[string]string{"打开微信":"open-wechat","请帮我打开微信。":"open-wechat","启动微信":"open-wechat","今天天气怎么样":"dictate","帮我删除所有文件":"dictate","":"empty"}
for said,want:=range cases {if got:=classifyMaohaoSpeech(said);got!=want {t.Fatalf("%q: %s want %s",said,got,want)}}
}
func TestSTTDisabledNoUpload(t *testing.T){
t.Setenv("OCTOP_MAOHAO_LOCAL_STT_URL","")
if _,e:=maohaoLocalSTT([]byte("pcm"));e==nil{t.Fatal("unconfigured STT allowed")}
t.Setenv("OCTOP_MAOHAO_LOCAL_STT_URL","http://other-host.example/stt")
if _,e:=maohaoLocalSTT([]byte("pcm"));e==nil{t.Fatal("external endpoint accepted")}
}
