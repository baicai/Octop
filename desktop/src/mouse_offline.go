package main

// Offline SenseVoice model setup and the agent-owned CPU recognizer process.
// The model is DSH's pinned INT8 SenseVoice revision; user speech never leaves
// this machine. Download runs outside the Wails UI/event processing goroutine.
import (
 "bufio"
 "crypto/sha256"
 "encoding/base64"
 "encoding/hex"
 "encoding/json"
 "errors"
 "fmt"
 "hash"
 "io"
 "net/http"
 "os"
 "os/exec"
 "path/filepath"
 "strings"
 "sync"
 "time"
 "unicode/utf8"
)

type senseVoiceAsset struct {
 Name string
 URL string
 Size int64
 SHA string
}
var senseVoiceAssets=[]senseVoiceAsset{
 {"model.int8.onnx","https://huggingface.co/csukuangfj/sherpa-onnx-sense-voice-zh-en-ja-ko-yue-2024-07-17/resolve/2365baeacb507f821a0c8120fcee3d484dba7a07/model.int8.onnx",239233841,"c71f0ce00bec95b07744e116345e33d8cbbe08cef896382cf907bf4b51a2cd51"},
 {"tokens.txt","https://huggingface.co/csukuangfj/sherpa-onnx-sense-voice-zh-en-ja-ko-yue-2024-07-17/resolve/2365baeacb507f821a0c8120fcee3d484dba7a07/tokens.txt",315894,"f449eb28dc567533d7fa59be34e2abca8784f771850c78a47fb731a31429a1dc"},
}

func senseVoiceModelDir()string{
 if p:=strings.TrimSpace(os.Getenv("OCTOP_MAOHAO_MODEL_DIR"));p!=""&&filepath.IsAbs(p){return filepath.Clean(p)}
 return filepath.Join(octopHome(),"speech-models","sensevoice-int8")
}
func modelHashMatches(path string,asset senseVoiceAsset)bool {
 f,err:=os.Open(path);if err!=nil{return false}
 defer f.Close()
 stat,err:=f.Stat()
 if err!=nil||stat.Size()!=asset.Size {return false}
 hasher:=sha256.New()
 if _,err=io.Copy(hasher,f);err!=nil{return false}
 return hex.EncodeToString(hasher.Sum(nil))==asset.SHA
}
func downloadModelOne(root string,asset senseVoiceAsset,origin string,progress func(int))error{
 target:=filepath.Join(root,asset.Name)
 req,err:=http.NewRequest("GET",origin,nil)
 if err!=nil{return err}
 client:=&http.Client{Timeout:20*time.Minute}
 response,err:=client.Do(req)
 if err!=nil{return err}
 defer response.Body.Close()
 if response.StatusCode!=http.StatusOK{return fmt.Errorf("model HTTP status %d",response.StatusCode)}
 tmp,err:=os.CreateTemp(root,asset.Name+".partial-*")
 if err!=nil{return err}
 tmpName:=tmp.Name()
 defer os.Remove(tmpName)
 hasher:=sha256.New()
 written,err:=copyVerifiedModel(tmp,hasher,io.LimitReader(response.Body,asset.Size+1),asset.Size,progress)
 closeErr:=tmp.Close()
 if err!=nil{return err}
 if closeErr!=nil{return closeErr}
 if written!=asset.Size||hex.EncodeToString(hasher.Sum(nil))!=asset.SHA{
   return fmt.Errorf("model checksum or length mismatch")
 }
 // os.Rename fails on Windows if the invalid target already exists.
 _=os.Remove(target)
 if err=os.Rename(tmpName,target);err!=nil{return err}
 return nil
}
func copyVerifiedModel(dst io.Writer,h hash.Hash,src io.Reader,size int64,progress func(int))(int64,error){
 buf:=make([]byte,128*1024)
 var copied int64
 last:=-1
 for{
  n,err:=src.Read(buf)
  if n>0{
    count,writeErr:=dst.Write(buf[:n])
    if writeErr!=nil{return copied,writeErr}
    if count!=n{return copied,io.ErrShortWrite}
    _,_=h.Write(buf[:n]);copied+=int64(n)
    if size>0&&progress!=nil{
      percentage:=int(copied*100/size)
      if percentage>100{percentage=100}
      if percentage/10!=last/10{
        last=percentage;progress(percentage)
      }
    }
  }
  if errors.Is(err,io.EOF){break}
  if err!=nil{return copied,err}
 }
 return copied,nil
}
func ensureSenseVoiceModels(notify func(string,any)) (string,error){
 root:=senseVoiceModelDir()
 if err:=os.MkdirAll(root,0700);err!=nil{return "",err}
 for _,asset:=range senseVoiceAssets{
  target:=filepath.Join(root,asset.Name)
  if modelHashMatches(target,asset){
   maohaoDebug("models.file_ready","name",asset.Name)
   continue
  }
  maohaoDebug("models.download_needed","name",asset.Name)
  mouseCommandNotice("model","正在下载离线语音模型")
  notify("octop:maohao-status",map[string]string{"phase":"model-downloading","resource":asset.Name})
  var first error
  for i,src:=range []string{asset.URL,strings.Replace(asset.URL,"https://huggingface.co/","https://hf-mirror.com/",1)}{
    err:=downloadModelOne(root,asset,src,func(pct int){
      mouseCommandNotice("model",fmt.Sprintf("正在下载离线语音模型 %d%%",pct))
      notify("octop:maohao-status",map[string]any{
        "phase":"model-downloading","resource":asset.Name,"progress":pct,
      })
    })
    if err==nil {first=nil;break}
    if i==0 {first=err}
  }
  if first!=nil || !modelHashMatches(target,asset){
    maohaoDebug("models.download_failed","name",asset.Name,"error",maohaoDebugErr(first))
    return "",fmt.Errorf("offline speech model preparation failed for %s",asset.Name)
  }
 }
 mouseCommandNotice("transcribing","识别中")
 notify("octop:maohao-status",map[string]string{"phase":"model-ready"})
 return root,nil
}

type maohaoOfflineWorker struct {
 mu sync.Mutex
 prepared bool
 cmd *exec.Cmd
 input io.WriteCloser
 lines chan string
}
func (w *maohaoOfflineWorker) reset(){
 if w.input!=nil {_=w.input.Close()}
 if w.cmd!=nil&&w.cmd.Process!=nil {_=w.cmd.Process.Kill()}
 w.cmd=nil;w.input=nil;w.lines=nil
}
func (w *maohaoOfflineWorker) stop(){
 w.mu.Lock();defer w.mu.Unlock();w.reset()
}
func (w *maohaoOfflineWorker) start(root string)error{
 maohaoDebug("worker.start","model_dir_exists",root!="")
 portable:=portableDir()
 py:=pythonExe(portable)
 script:=filepath.Join(portable,"sensevoice_worker.py")
 if _,err:=os.Stat(py);err!=nil {
  // The agent may start before the Octop UI has ever been opened.
  if err=ensurePortable(LocaleZH,func(string){});err!=nil{
    return fmt.Errorf("bundled offline runtime extraction failed: %w",err)
  }
 }
 // Older demo installs may have the same Octop version but no SenseVoice
 // worker. Repair only when a physical mouse recording needs local ASR:
 // leave the core version-comparison/backup logic unchanged.
 if !portableHasMaohaoOffline(portable) {
  maohaoDebug("worker.repair_needed")
  if err:=repairMaohaoSpeechRuntime(portable);err!=nil {
    maohaoDebug("worker.repair_failed","error",maohaoDebugErr(err))
    return fmt.Errorf("bundled speech runtime repair failed: %w",err)
  }
 }
 if _,err:=os.Stat(py);err!=nil{return fmt.Errorf("portable Python not installed")}
 if _,err:=os.Stat(script);err!=nil{return fmt.Errorf("SenseVoice worker not packaged")}
 cmd:=exec.Command(py,script,root)
 cmd.Dir=portable
 cmd.Env=senseVoiceWorkerEnv(portable)
 cmd.Stderr=io.Discard // do not persist microphone or device diagnostics
 prepareMaohaoCommand(cmd)
 stdout,err:=cmd.StdoutPipe();if err!=nil{return err}
 stdin,err:=cmd.StdinPipe();if err!=nil{return err}
 if err=cmd.Start();err!=nil{
   maohaoDebug("worker.spawn_failed","error",maohaoDebugErr(err))
   return err
 }
 maohaoDebug("worker.spawned","pid",cmd.Process.Pid)
 lines:=make(chan string,2)
 go func(){
   scanner:=bufio.NewScanner(stdout)
   scanner.Buffer(make([]byte,16*1024),64*1024)
   for scanner.Scan(){lines<-scanner.Text()}
   close(lines)
   _=cmd.Wait()
 }()
 w.cmd,w.input,w.lines=cmd,stdin,lines
 select{
 case line,ok:=<-lines:
  if !ok{
   maohaoDebug("worker.exited_before_ready")
   return errors.New("SenseVoice worker exited during startup")
  }
  var ready struct{Ready bool `json:"ready"`}
  if json.Unmarshal([]byte(line),&ready)!=nil||!ready.Ready{
   maohaoDebug("worker.not_ready","response_bytes",len(line))
   return errors.New("SenseVoice worker not ready")
  }
  maohaoDebug("worker.ready")
  return nil
 case <-time.After(45*time.Second):
  maohaoDebug("worker.start_timeout")
  return errors.New("SenseVoice model loading timed out")
 }
}
func (w *maohaoOfflineWorker) transcribe(wav []byte,notify func(string,any))(string,error){
 w.mu.Lock()
 defer w.mu.Unlock()
 if !w.prepared {
  root,err:=ensureSenseVoiceModels(notify)
  if err!=nil{return "",err}
  w.prepared=true
  _=root
 }
 if w.cmd==nil {
  mouseCommandNotice("model","正在加载离线语音模型")
  notify("octop:maohao-status",map[string]string{"phase":"model-loading"})
  if err:=w.start(senseVoiceModelDir());err!=nil{w.reset();return "",err}
 }
 maohaoDebug("worker.transcribe_started","wav_bytes",len(wav))
 if len(wav)>16000*2*45+44{return "",errors.New("PCM recording too large")}
 payload,err:=json.Marshal(map[string]string{"wavBase64":base64.StdEncoding.EncodeToString(wav)})
 if err!=nil{return "",err}
 if _,err=w.input.Write(append(payload,'\n'));err!=nil{
  maohaoDebug("worker.write_failed","error",maohaoDebugErr(err))
  w.reset();return "",err
 }
 select{
 case line,ok:=<-w.lines:
  if !ok{
   maohaoDebug("worker.exited_during_transcribe")
   w.reset();return "",errors.New("SenseVoice worker exited")
  }
  text,decodeErr:=decodeSenseVoiceResponse(line)
  if decodeErr!=nil{
   maohaoDebug("worker.response_error","response_bytes",len(line),"error",maohaoDebugErr(decodeErr))
   return "",decodeErr
  }
  return text,nil
 case <-time.After(90*time.Second):
  maohaoDebug("worker.transcription_timeout")
  w.reset()
  return "",errors.New("SenseVoice transcription deadline exceeded")
 }
}


// An older worker must use UTF-8 even when Windows is configured for CP936.
func senseVoiceWorkerEnv(portable string)[]string{
 return overlayEnv(os.Environ(),map[string]string{
  "PYTHONPATH":filepath.Join(portable,"packages"),
  "PYTHONNOUSERSITE":"1",
  "PYTHONIOENCODING":"utf-8",
  "PYTHONUTF8":"1",
 })
}

// Go's JSON parser replaces invalid UTF-8 with U+FFFD unless rejected first.
func decodeSenseVoiceResponse(line string)(string,error){
 if !utf8.ValidString(line){return "",errors.New("SenseVoice response is not UTF-8")}
 var result struct{
  Text string `json:"text"`
  Error string `json:"error"`
 }
 if err:=json.Unmarshal([]byte(line),&result);err!=nil||result.Error!=""{
  return "",errors.New("SenseVoice recognition failed")
 }
 if strings.ContainsRune(result.Text,utf8.RuneError){
  return "",errors.New("SenseVoice response contains corrupted Unicode")
 }
 return strings.TrimSpace(result.Text),nil
}
