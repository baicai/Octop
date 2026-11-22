package main
import (
"bufio"
"crypto/rand"
"encoding/hex"
"encoding/json"
"errors"
"fmt"
"io"
"log"
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
listener net.Listener
worker *maohaoOfflineWorker
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
// The listener has already been claimed, so no other agent can own this file.
_ = os.Remove(maohaoTokenPath())
if err=os.Rename(temp,maohaoTokenPath());err!=nil{return "",err}
return secret,nil
}
func runMaohaoAgent()error{
if runtime.GOOS!="windows"{return nil}
maohaoDebug("agent.start","address",maohaoAddr)
ln,err:=net.Listen("tcp",maohaoAddr);if err!=nil{maohaoDebug("agent.listen_failed","error",maohaoDebugErr(err));return err}
maohaoDebug("agent.listening")
defer ln.Close()
secret,err:=createMaohaoSessionToken();if err!=nil{return err}
// Session credentials are regenerated on daemon startup; they are not a
// permanent device identifier, user account, or remote tracking token.
defer os.Remove(maohaoTokenPath())
a:=&maohaoAgent{peers:map[*maohaoPeer]struct{}{},worker:&maohaoOfflineWorker{}}
m:=newMaohaoMouseBridge(nil);m.agent=a;a.bridge=m;a.listener=ln
finished:=make(chan struct{})
go mouseNoticeLoop()
go func(){
defer close(finished)
for {
m.start()
m.mu.Lock();stopped:=m.stopped;m.mu.Unlock()
if stopped{return}
// If the SDK helper crashes, restart its listener without requiring a GUI.
time.Sleep(2*time.Second)
}
}()
defer func(){
  a.worker.stop()
  m.stop()
  select{case <-finished:case <-time.After(3*time.Second):}
}()
for {c,err:=ln.Accept();if err!=nil{
if errors.Is(err,net.ErrClosed){return nil}
return err
};go a.handle(c,secret)}
}
func (a *maohaoAgent)handle(conn net.Conn,secret string){
defer conn.Close();_=conn.SetReadDeadline(time.Now().Add(4*time.Second))
r:=bufio.NewReaderSize(conn,512)
line,err:=r.ReadString('\n');if err!=nil||len(line)>512{return}
var hello struct {
 Token string `json:"token"`
 Action string `json:"action"`
}
if json.Unmarshal([]byte(line),&hello)!=nil||hello.Token!=secret{return}
if hello.Action=="shutdown" {
maohaoDebug("agent.shutdown_received")
  _,_=io.WriteString(conn,"ok\n")
  _=a.listener.Close()
  return
}
if hello.Action!="" {return}
p:=&maohaoPeer{conn:conn}
a.mu.Lock();a.peers[p]=struct{}{};peers:=len(a.peers);a.mu.Unlock()
maohaoDebug("agent.gui_connected","peers",peers)
defer func(){a.mu.Lock();delete(a.peers,p);a.mu.Unlock()}()
exePath,_:=os.Executable()
status,_:=json.Marshal(map[string]string{"phase":"agent-connected","executable":exePath})
_=p.send(maohaoPacket{Event:"octop:maohao-status",Detail:status})
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
if time.Since(a.lastWake)<time.Second {a.mu.Unlock();maohaoDebug("ai.wake_throttled");return}
a.lastWake=time.Now();active:=len(a.peers)>0
a.mu.Unlock()
maohaoDebug("ai.wake_dispatch","gui_connected",active)
a.broadcast("octop:maohao-wake",map[string]string{"source":"ai-key"})
if active{return}
exe,err:=os.Executable();if err!=nil{return}
cmd:=exec.Command(exe);prepareMaohaoCommand(cmd)
if err:=cmd.Start();err==nil{
 maohaoDebug("ai.launch_gui_started","pid",cmd.Process.Pid)
 go func(){_=cmd.Wait()}()
}else{maohaoDebug("ai.launch_gui_failed","error",maohaoDebugErr(err))}
}
func (a *maohaoAgent)rememberFocus(device string){
a.mu.Lock();a.device=device;a.foreground=maohaoForegroundWindow()
hasFocus:=a.foreground!=0
octopFocus:=hasFocus&&maohaoIsOctopForeground(a.foreground)
a.mu.Unlock()
maohaoDebug("audio.focus_captured","has_focus",hasFocus,"octop_foreground",octopFocus)
}
func (a *maohaoAgent)onAudio(device string,wav []byte){
a.mu.Lock();foreground,wasDevice:=a.foreground,a.device
active:=len(a.peers)>0
a.device="";a.foreground=0;a.mu.Unlock()
maohaoDebug("audio.received","wav_bytes",len(wav),"device_matches",wasDevice==device,"gui_connected",active,"has_focus",foreground!=0)
if wasDevice!=device{maohaoDebug("audio.discarded","reason","missing_start_or_device_mismatch");return}
_ = active
// A single offline STT path is used for both system commands and Octop
// conversation text, even if the Octop window is not open.
go a.backgroundAction(wav,foreground)
}
func classifyMaohaoSpeech(s string)string{
s=strings.Trim(strings.TrimSpace(s),"，,。！! ")
switch s {case "打开微信","请打开微信","帮我打开微信","请帮我打开微信","启动微信":return "open-wechat"}
if s==""{return "empty"}
return "dictate"
}
// Match explicit system commands before routing unmatched speech to chat.
func maohaoSpeechRoute(items []MouseCommand,msg string,octopForeground bool)(string,MouseCommand){
 if cmd,ok:=findMouseCommand(items,msg);ok{return "command",cmd}
 if strings.TrimSpace(msg)==""{return "empty",MouseCommand{}}
 if octopForeground{return "chat",MouseCommand{}}
 return "dictate",MouseCommand{}
}
func (a *maohaoAgent)backgroundAction(wav []byte,window uintptr){
 maohaoDebug("asr.started","wav_bytes",len(wav),"has_focus",window!=0)
 var msg string
 var err error
 source:="sensevoice"
 if os.Getenv("OCTOP_MAOHAO_LOCAL_STT_URL")!=""{
  source="local_http"
  msg,err=maohaoLocalSTT(wav)
 }else{
  msg,err=a.worker.transcribe(wav,a.broadcast)
 }
 if err!=nil{
  maohaoDebug("asr.failed","source",source,"error",maohaoDebugErr(err))
  a.broadcast("octop:maohao-status",map[string]string{"phase":"local-stt-unavailable"})
  mouseCommandNotice("error","语音识别未就绪或失败")
  return
 }
 preview,count,hash:=maohaoDebugTranscript(msg)
 maohaoDebug("asr.result","source",source,"runes",count,"hash12",hash,"command_preview",preview)
 // Show exactly the recognized UTF-8 text while executing a command or
 // inserting it into the focused input, without stealing focus.
 if strings.TrimSpace(msg)!="" {mouseCommandNotice("recognized",msg)}
 items,readErr:=readMouseCommands()
 if readErr!=nil{
  maohaoDebug("commands.read_failed","error",maohaoDebugErr(readErr))
  mouseCommandNotice("error","指令集读取失败")
  return
 }
 octopFront:=window!=0&&maohaoIsOctopForeground(window)
 route,cmd:=maohaoSpeechRoute(items,msg,octopFront)
 maohaoDebug("commands.route","route",route,"count",len(items),"octop_foreground",octopFront,"command_id",cmd.ID)
 switch route{
 case "command":
  if cmd.Action=="close-app"{
   maohaoDebug("commands.blocked","id",cmd.ID,"reason","close_requires_confirmation")
   mouseCommandNotice("error","关闭操作请在指令管理中确认执行")
   return
  }
  maohaoDebug("commands.execute_start","id",cmd.ID,"action",cmd.Action)
  mouseCommandNotice("executing",cmd.Name)
  if err:=executeMouseCommand(cmd);err!=nil{
   maohaoDebug("commands.execute_failed","id",cmd.ID,"error",maohaoDebugErr(err))
   mouseCommandNotice("error","指令执行失败")
   return
  }
  maohaoDebug("commands.execute_ok","id",cmd.ID)
  mouseCommandNotice("ready","")
 case "chat":
  maohaoDebug("dictation.sent_to_chat","runes",count)
  a.broadcast("octop:maohao-transcript",map[string]string{"text":msg})
  mouseCommandNotice("ready","")
 case "empty":
  maohaoDebug("asr.empty")
  mouseCommandNotice("error","未识别到语音")
 default:
  if window==0||window!=maohaoForegroundWindow(){
   maohaoDebug("dictation.discarded","reason","no_focus_or_focus_changed","has_focus",window!=0)
   mouseCommandNotice("error","未识别到指令，输入焦点已变化")
   return
  }
  if err:=maohaoTypeUnicode(msg);err!=nil{
   maohaoDebug("dictation.insert_failed","error",maohaoDebugErr(err))
   mouseCommandNotice("error","文字输入失败")
   return
  }
  maohaoDebug("dictation.insert_ok","runes",count)
  mouseCommandNotice("ready","")
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
func shutdownMaohaoAgent()error{
 secret,err:=maohaoToken();if err!=nil{return err}
 conn,err:=net.DialTimeout("tcp",maohaoAddr,2*time.Second);if err!=nil{return err}
 defer conn.Close()
 _=conn.SetDeadline(time.Now().Add(2*time.Second))
 return json.NewEncoder(conn).Encode(map[string]string{"token":secret,"action":"shutdown"})
}
// A listener from a removed or superseded checkout cannot own the SDK.
func sameMaohaoExecutable(agentPath,clientPath string)bool{
return agentPath!="" && clientPath!="" && strings.EqualFold(filepath.Clean(agentPath),filepath.Clean(clientPath))
}
func launchMaohaoAgent(exe string){
cmd:=exec.Command(exe,"--maohao-agent");prepareMaohaoCommand(cmd)
if err:=cmd.Start();err!=nil{log.Printf("Maohao agent start failed: %v",err);return}
go func(){_=cmd.Wait()}()
}
func connectMaohaoAgent(a *App){
if runtime.GOOS!="windows"{return}
_ = maohaoRegisterLoginStart()
exe,err:=os.Executable();if err!=nil{return}
launchMaohaoAgent(exe)
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
if pkt.Event=="octop:maohao-status" {
var status struct{Phase string `json:"phase"`;Executable string `json:"executable"`}
if json.Unmarshal(pkt.Detail,&status)==nil && status.Phase=="agent-connected" && !sameMaohaoExecutable(status.Executable,exe){
 // Shut down the stale SDK owner using the authenticated local IPC command.
 conn.Close()
 if err:=shutdownMaohaoAgent();err!=nil{log.Printf("Maohao stale agent shutdown failed: %v",err)} else {
 // The old process may spend up to three seconds closing its SDK helper.
 time.Sleep(3500*time.Millisecond)
 launchMaohaoAgent(exe)
 }
 break
}
}
if pkt.Event=="octop:maohao-wake" {a.showWindow();continue}
if (pkt.Event=="octop:maohao-transcript"||pkt.Event=="octop:maohao-audio"||pkt.Event=="octop:maohao-status")&&a.window!=nil{
name,_:=json.Marshal(pkt.Event)
a.window.ExecJS("window.dispatchEvent(new CustomEvent("+string(name)+",{detail:"+string(pkt.Detail)+"}));")
}
}
conn.Close();time.Sleep(time.Second)
}
}
