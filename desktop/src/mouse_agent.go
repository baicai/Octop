package main
import (
"bufio"
"crypto/rand"
"encoding/base64"
"encoding/hex"
"encoding/json"
"errors"
"fmt"
"io"
"net"
"net/http"
"net/url"
"os"
"os/exec"
"path/filepath"
"runtime"
"strings"
"sync"
"time"
)
const maohaoAddr="127.0.0.1:18089"
type maohaoPacket struct {Event string `json:"event"`; Detail json.RawMessage `json:"detail"`}
type maohaoPeer struct {conn net.Conn;mu sync.Mutex}
func (p *maohaoPeer) send(pkt maohaoPacket)error{
p.mu.Lock();defer p.mu.Unlock()
_=p.conn.SetWriteDeadline(time.Now().Add(2*time.Second))
return json.NewEncoder(p.conn).Encode(pkt)
}
type maohaoAgent struct {
mu sync.Mutex
peers map[*maohaoPeer]struct{}
bridge *maohaoMouseBridge
device string
foreground uintptr
lastWake time.Time
}
func maohaoTokenPath()string{return filepath.Join(octopHome(),"maohao-agent.token")}
func maohaoToken()(string,error){
raw,err:=os.ReadFile(maohaoTokenPath());if err!=nil{return "",err}
secret:=strings.TrimSpace(string(raw))
if len(secret)!=64{return "",errors.New("invalid Maohao session token")}
if _,err:=hex.DecodeString(secret);err!=nil{return "",err}
return secret,nil
}
func createMaohaoSessionToken()(string,error){
if err:=os.MkdirAll(octopHome(),0700);err!=nil{return "",err}
b:=make([]byte,32);if _,err:=rand.Read(b);err!=nil{return "",err}
secret:=hex.EncodeToString(b)
f,err:=os.CreateTemp(octopHome(),"maohao-session-*");if err!=nil{return "",err}
temp:=f.Name()
defer os.Remove(temp)
if err=f.Chmod(0600);err!=nil{f.Close();return "",err}
if _,err=f.WriteString(secret);err!=nil{f.Close();return "",err}
if err=f.Close();err!=nil{return "",err}
if err=os.Rename(temp,maohaoTokenPath());err!=nil{return "",err}
return secret,nil
}
func runMaohaoAgent()error{
if runtime.GOOS!="windows"{return nil}
ln,err:=net.Listen("tcp",maohaoAddr);if err!=nil{return err}
defer ln.Close()
secret,err:=createMaohaoSessionToken();if err!=nil{return err}
// Session credentials are regenerated on daemon startup; they are not a
// permanent device identifier, user account, or remote tracking token.
defer os.Remove(maohaoTokenPath())
a:=&maohaoAgent{peers:map[*maohaoPeer]struct{}{}}
m:=newMaohaoMouseBridge(nil);m.agent=a;a.bridge=m
go m.start();defer m.stop()
for {c,err:=ln.Accept();if err!=nil{return err};go a.handle(c,secret)}
}
func (a *maohaoAgent)handle(conn net.Conn,secret string){
defer conn.Close();_=conn.SetReadDeadline(time.Now().Add(4*time.Second))
r:=bufio.NewReaderSize(conn,512)
line,err:=r.ReadString('
');if err!=nil||len(line)>512{return}
var hello struct{Token string `json:"token"`}
if json.Unmarshal([]byte(line),&hello)!=nil||hello.Token!=secret{return}
p:=&maohaoPeer{conn:conn}
a.mu.Lock();a.peers[p]=struct{}{};a.mu.Unlock()
defer func(){a.mu.Lock();delete(a.peers,p);a.mu.Unlock()}()
_=p.send(maohaoPacket{Event:"octop:maohao-status",Detail:json.RawMessage(`{"phase":"agent-connected"}`)})
_=conn.SetReadDeadline(time.Time{})
// One way only: a renderer cannot ask this agent to record or inject input.
_,_=io.Copy(io.Discard,io.LimitReader(r,1024))
}
func (a *maohaoAgent) broadcast(name string,detail any){
raw,err:=json.Marshal(detail);if err!=nil{return}
pkt:=maohaoPacket{Event:name,Detail:raw}
a.mu.Lock();peers:=make([]*maohaoPeer,0,len(a.peers));for p:=range a.peers {peers=append(peers,p)};a.mu.Unlock()
for _,p:=range peers {if err:=p.send(pkt);err!=nil{_=p.conn.Close()}}
}
func (a *maohaoAgent)wake(){
a.mu.Lock()
if time.Since(a.lastWake)<time.Second {a.mu.Unlock();return}
a.lastWake=time.Now();active:=len(a.peers)>0
a.mu.Unlock()
a.broadcast("octop:maohao-wake",map[string]string{"source":"ai-key"})
if active{return}
exe,err:=os.Executable();if err!=nil{return}
cmd:=exec.Command(exe);prepareMaohaoCommand(cmd)
if cmd.Start()==nil{go func(){_=cmd.Wait()}()}
}
func (a *maohaoAgent)rememberFocus(device string){
a.mu.Lock();a.device=device;a.foreground=maohaoForegroundWindow();a.mu.Unlock()
}
func (a *maohaoAgent)onAudio(device string,wav []byte){
a.mu.Lock();foreground,wasDevice:=a.foreground,a.device
active:=len(a.peers)>0
a.device="";a.foreground=0;a.mu.Unlock()
if wasDevice!=device{return}
if foreground!=0&&!maohaoIsOctopForeground(foreground){
go a.backgroundAction(wav,foreground);return
}
if active {a.broadcast("octop:maohao-audio",map[string]string{"wavBase64":base64.StdEncoding.EncodeToString(wav)})
}else{go a.backgroundAction(wav,foreground)}
}
func classifyMaohaoSpeech(s string)string{
s=strings.Trim(strings.TrimSpace(s),"，,。！! ")
switch s {case "打开微信","请打开微信","帮我打开微信","请帮我打开微信","启动微信":return "open-wechat"}
if s==""{return "empty"}
return "dictate"
}
func (a *maohaoAgent)backgroundAction(wav []byte,window uintptr){
msg,err:=maohaoLocalSTT(wav)
if err!=nil {a.broadcast("octop:maohao-status",map[string]string{"phase":"local-stt-unavailable"});return}
switch classifyMaohaoSpeech(msg){
case "open-wechat":_ = maohaoOpenWeChat()
case "dictate":
if window!=0&&window==maohaoForegroundWindow(){_=maohaoTypeUnicode(msg)}
}
}
func maohaoLocalSTT(wav []byte)(string,error){
raw:=strings.TrimSpace(os.Getenv("OCTOP_MAOHAO_LOCAL_STT_URL"))
if raw==""{return "",errors.New("local STT not configured")}
u,err:=url.Parse(raw)
if err!=nil||u.Scheme!="http"||(u.Hostname()!="127.0.0.1"&&u.Hostname()!="localhost"){return "",errors.New("local-only STT URL required")}
req,err:=http.NewRequest("POST",u.String(),strings.NewReader(string(wav)))
if err!=nil{return "",err}
req.Header.Set("Content-Type","audio/wav")
c:=&http.Client{Timeout:30*time.Second,CheckRedirect:func(*http.Request,[]*http.Request)error{return http.ErrUseLastResponse}}
resp,err:=c.Do(req);if err!=nil{return "",err}
defer resp.Body.Close()
if resp.StatusCode!=200{return "",fmt.Errorf("STT status %d",resp.StatusCode)}
var out struct{Text string `json:"text"`}
if err=json.NewDecoder(io.LimitReader(resp.Body,8192)).Decode(&out);err!=nil{return "",err}
return strings.TrimSpace(out.Text),nil
}
func connectMaohaoAgent(a *App){
if runtime.GOOS!="windows"{return}
_ = maohaoRegisterLoginStart()
exe,err:=os.Executable();if err!=nil{return}
cmd:=exec.Command(exe,"--maohao-agent");prepareMaohaoCommand(cmd)
if cmd.Start()==nil{go func(){_=cmd.Wait()}()}
for {
secret,err:=maohaoToken()
if err!=nil {time.Sleep(time.Second);continue}
conn,err:=net.DialTimeout("tcp",maohaoAddr,2*time.Second)
if err!=nil {time.Sleep(time.Second);continue}
if json.NewEncoder(conn).Encode(map[string]string{"token":secret})!=nil {conn.Close();time.Sleep(time.Second);continue}
s:=bufio.NewScanner(conn);s.Buffer(make([]byte,65536),4*1024*1024)
for s.Scan(){
var pkt maohaoPacket
if json.Unmarshal(s.Bytes(),&pkt)!=nil{continue}
if pkt.Event=="octop:maohao-wake" {a.showWindow();continue}
if (pkt.Event=="octop:maohao-audio"||pkt.Event=="octop:maohao-status")&&a.window!=nil{
name,_:=json.Marshal(pkt.Event)
a.window.ExecJS("window.dispatchEvent(new CustomEvent("+string(name)+",{detail:"+string(pkt.Detail)+"}));")
}
}
conn.Close();time.Sleep(time.Second)
}
}
