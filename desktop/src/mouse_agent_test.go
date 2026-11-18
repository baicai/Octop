package main
import (
"testing"
"net/http"
"net/http/httptest"
"io"
"strings"
)
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

func TestBackgroundSTTLoopbackContract(t *testing.T){
  expected:="RIFFfakewav"
  server:=httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter,r *http.Request){
    if r.Method!="POST" || r.Header.Get("Content-Type")!="audio/wav"{t.Error("wrong STT method/type")}
    b,e:=io.ReadAll(r.Body);if e!=nil||string(b)!=expected{t.Error("incorrect PCM WAV")}
    _,_=io.WriteString(w,`{"text":"打开微信"}`)
  }))
  defer server.Close()
  if !strings.HasPrefix(server.URL,"http://127.0.0.1:") {t.Skip("unexpected loopback family")}
  t.Setenv("OCTOP_MAOHAO_LOCAL_STT_URL",server.URL)
  got,err:=maohaoLocalSTT([]byte(expected))
  if err!=nil||got!="打开微信"{t.Fatalf("transcription=%q error=%v",got,err)}
}
func TestMaohaoSessionCredentialRotates(t *testing.T){
  t.Setenv("OCTOP_HOME",t.TempDir())
  first,err:=createMaohaoSessionToken()
  if err!=nil||len(first)!=64{t.Fatalf("first token: %v",err)}
  second,err:=createMaohaoSessionToken()
  if err!=nil||len(second)!=64||first==second{t.Fatalf("token rotation failed: %v",err)}
  read,err:=maohaoToken()
  if err!=nil||read!=second {t.Fatalf("token read mismatch: %v",err)}
}
