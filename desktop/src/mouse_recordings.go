package main

// Local voice samples are optional reference recordings for command editing.
// The matcher uses recognized text, never voiceprint identity or enrollment.
import (
 "bytes"
 "encoding/base64"
 "encoding/binary"
 "errors"
 "fmt"
 "os"
 "path/filepath"
 "runtime"
 "strings"
)

const maxCommandWavBytes = 16000*2*45 + 44

func validMouseRecordID(id string)bool{
 if len(id)==0||len(id)>96||strings.Contains(id,".."){return false}
 for _,c:=range id{
  if c>='a'&&c<='z'||c>='A'&&c<='Z'||c>='0'&&c<='9'||c=='-'||c=='_' {continue}
  return false
 }
 return true
}
func voiceSamplePath(id string)(string,error){
 if !validMouseRecordID(id){return "",errors.New("无效的指令编号")}
 return filepath.Join(octopHome(),"mouse-command-voice",id+".wav"),nil
}
func validMouseWAV(data []byte)bool{
 if len(data)<44||len(data)>maxCommandWavBytes{return false}
 if !bytes.Equal(data[:4],[]byte("RIFF"))||
  !bytes.Equal(data[8:12],[]byte("WAVE"))||
  !bytes.Equal(data[12:16],[]byte("fmt "))||
  !bytes.Equal(data[36:40],[]byte("data")){return false}
 if binary.LittleEndian.Uint16(data[20:22])!=1||binary.LittleEndian.Uint16(data[22:24])!=1||
  binary.LittleEndian.Uint32(data[24:28])!=16000||binary.LittleEndian.Uint16(data[34:36])!=16{return false}
 return int(binary.LittleEndian.Uint32(data[40:44]))==len(data)-44
}
func readCommandSampleInput(encoded string)([]byte,error){
 if len(encoded)>2*maxCommandWavBytes{return nil,errors.New("语音录音超过45秒")}
 data,err:=base64.StdEncoding.DecodeString(encoded)
 if err!=nil||!validMouseWAV(data){return nil,errors.New("仅支持45秒内的16kHz单声道WAV录音")}
 return data,nil
}
func (a *App) SaveMouseCommandRecording(id,encoded string)error{
 commandLock.Lock();defer commandLock.Unlock()
 path,err:=voiceSamplePath(id);if err!=nil{return err}
 items,err:=readMouseCommands();if err!=nil{return err}
 found:=false
 for _,c:=range items{if c.ID==id{found=true;break}}
 if !found{return errors.New("请先保存指令")}
 raw,err:=readCommandSampleInput(encoded);if err!=nil{return err}
 if err=os.MkdirAll(filepath.Dir(path),0700);err!=nil{return err}
 f,err:=os.CreateTemp(filepath.Dir(path),"voice-*.tmp")
 if err!=nil{return err}
 defer os.Remove(f.Name())
 if err=f.Chmod(0600);err!=nil{f.Close();return err}
 if _,err=f.Write(raw);err!=nil{f.Close();return err}
 if err=f.Close();err!=nil{return err}
 return replaceMouseCommandFile(f.Name(),path)
}
func (a *App) GetMouseCommandRecording(id string)(string,error){
 path,err:=voiceSamplePath(id);if err!=nil{return "",err}
 data,err:=os.ReadFile(path)
 if os.IsNotExist(err){return "",nil}
 if err!=nil{return "",err}
 if !validMouseWAV(data){return "",errors.New("录音文件无效")}
 return base64.StdEncoding.EncodeToString(data),nil
}
func (a *App) TranscribeMouseCommandRecording(encoded string)(string,error){
 if runtime.GOOS!="windows"{return "",errors.New("仅 Windows 客户端支持离线语音识别")}
 data,err:=readCommandSampleInput(encoded);if err!=nil{return "",err}
 if !launchReady(portableDir()){
  return "",errors.New("客户端正在准备离线运行环境，请完成初始化后重试")}
 worker:=&maohaoOfflineWorker{}
 defer worker.stop()
 result,err:=worker.transcribe(data,func(string,any){})
 if err!=nil{return "",fmt.Errorf("离线识别不可用：%w",err)}
 return strings.TrimSpace(result),nil
}
func pruneMouseRecordings(items []MouseCommand)error{
 keep:=make(map[string]bool,len(items))
 for _,c:=range items{keep[c.ID]=true}
 dir:=filepath.Join(octopHome(),"mouse-command-voice")
 entries,err:=os.ReadDir(dir)
 if os.IsNotExist(err){return nil}
 if err!=nil{return err}
 for _,e:=range entries{
  if e.IsDir()||!strings.HasSuffix(e.Name(),".wav"){continue}
  id:=strings.TrimSuffix(e.Name(),".wav")
  if validMouseRecordID(id)&&!keep[id]{
   if err:=os.Remove(filepath.Join(dir,e.Name()));err!=nil{return err}
  }
 }
 return nil
}
