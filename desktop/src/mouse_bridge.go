package main

// The vendor uMouse SDK runs in a separate native helper, never inside Wails.
// The desktop retains at most 45 seconds of PCM after a physical key-down.
import (
    "bufio"
    "bytes"
    "encoding/base64"
    "encoding/binary"
    "encoding/json"
    "io"
    "log"
    "os"
    "os/exec"
    "path/filepath"
    "runtime"
    "strconv"
    "sync"
    "time"
)

const maxMaohaoPCMBytes = 16000 * 2 * 45

type maohaoMouseBridge struct {
    app *App
    agent *maohaoAgent
    mu sync.Mutex
    cmd *exec.Cmd
    stdin io.WriteCloser
    stopped bool
    device string
    key string
    pcm []byte
}

func newMaohaoMouseBridge(a *App) *maohaoMouseBridge {
    return &maohaoMouseBridge{app: a}
}

func (m *maohaoMouseBridge) start() {
    if runtime.GOOS != "windows" { return }
    maohaoDebug("bridge.start")
    binaryPath, err := os.Executable()
    if err != nil { maohaoDebug("bridge.exe_missing","error",maohaoDebugErr(err));return }
    sdkDir := filepath.Join(filepath.Dir(binaryPath), "umouse-sdk")
    helper := filepath.Join(sdkDir, "umouse-bridge.exe")
    if _, err := os.Stat(helper); err != nil {
        maohaoDebug("bridge.helper_missing","error",maohaoDebugErr(err)); return
    }
    if _, err := os.Stat(filepath.Join(sdkDir, "uMouseSdk.dll")); err != nil {
        maohaoDebug("bridge.sdk_dll_missing","error",maohaoDebugErr(err)); return
    }
    cmd := exec.Command(helper, "--audio")
    cmd.Dir = sdkDir
    cmd.Stderr = io.Discard // Vendor errors may contain hardware identifiers.
    prepareMaohaoCommand(cmd)
    stdout, err := cmd.StdoutPipe()
    if err != nil { maohaoDebug("bridge.stdout_pipe_failed","error",maohaoDebugErr(err)); return }
    stdin, err := cmd.StdinPipe()
    if err != nil { maohaoDebug("bridge.stdin_pipe_failed","error",maohaoDebugErr(err)); return }

    m.mu.Lock()
    if m.stopped { m.mu.Unlock(); return }
    if err = cmd.Start(); err != nil {
        m.mu.Unlock()
        maohaoDebug("bridge.launch_failed","error",maohaoDebugErr(err))
        log.Printf("Maohao mouse SDK unavailable: %T", err)
        return
    }
    m.cmd, m.stdin = cmd, stdin
    maohaoDebug("bridge.helper_started","pid",cmd.Process.Pid)
    m.mu.Unlock()
    m.emit("octop:maohao-status", map[string]string{"phase":"starting"})

    scanner := bufio.NewScanner(stdout)
    scanner.Buffer(make([]byte, 64*1024), 128*1024)
    for scanner.Scan() { m.consumeLine(scanner.Bytes()) }
    scanErr:=scanner.Err()
    waitErr:=cmd.Wait()
    maohaoDebug("bridge.helper_exited","scan_error",maohaoDebugErr(scanErr),"exit_error",maohaoDebugErr(waitErr))
    m.mu.Lock()
    if m.cmd == cmd { m.cmd, m.stdin = nil, nil }
    m.device, m.key, m.pcm = "", "", nil
    stopped := m.stopped
    m.mu.Unlock()
    if !stopped { m.emit("octop:maohao-status", map[string]string{"phase":"disconnected"}) }
}

func (m *maohaoMouseBridge) consumeLine(line []byte) {
    if len(line) > 128*1024 { return }
    var frame struct {
        Kind string `json:"kind"`
        DeviceID string `json:"deviceId"`
        Version string `json:"version"`
        JSON string `json:"json"`
        Data string `json:"data"`
        Format string `json:"format"`
        SampleRate int `json:"sampleRate"`
    }
    if json.Unmarshal(line, &frame) != nil {maohaoDebug("bridge.frame_invalid_json");return}
    if frame.Kind == "ready" {
        maohaoDebug("bridge.sdk_ready","version",frame.Version)
        m.emit("octop:maohao-status", map[string]string{"phase":"ready","version":frame.Version})
        return
    }
    if frame.DeviceID == "" || len(frame.DeviceID)>128 { return }
    if frame.Kind == "connected" {
        maohaoDebug("bridge.mouse_connected")
        m.emit("octop:maohao-status", map[string]string{"phase":"device-connected"})
        return
    }
    if frame.Kind == "disconnected" {
        maohaoDebug("bridge.mouse_disconnected")
        m.mu.Lock()
        if frame.DeviceID == m.device { m.device,m.key,m.pcm = "","",nil }
        m.mu.Unlock()
        m.emit("octop:maohao-status", map[string]string{"phase":"device-disconnected"})
        return
    }
    if frame.Kind == "audio" {
        if frame.Format!="pcm_s16le" || frame.SampleRate!=16000 || len(frame.Data)>90000 { return }
        chunk,err := base64.StdEncoding.DecodeString(frame.Data)
        if err!=nil || len(chunk)==0 || len(chunk)%2!=0 {
        maohaoDebug("bridge.audio_chunk_invalid")
        return
    }
        m.mu.Lock()
        if m.device == frame.DeviceID && m.key != "" {
            if len(m.pcm)+len(chunk) <= maxMaohaoPCMBytes {
                m.pcm = append(m.pcm,chunk...)
            } else {
                m.device,m.key,m.pcm = "","",nil
                maohaoDebug("bridge.audio_overflow","bytes",len(m.pcm)+len(chunk))
            }
        }
        m.mu.Unlock()
        return
    }
    if frame.Kind!="message" || len(frame.JSON)==0 || len(frame.JSON)>16000 { return }
    var keyEvent struct {
        Type string `json:"type"`
        Key string `json:"key"`
        Action string `json:"action"`
    }
    if json.Unmarshal([]byte(frame.JSON),&keyEvent)!=nil {
        maohaoDebug("bridge.key_json_invalid")
        return
    }
    if keyEvent.Type!="keyEvent"{return}
    maohaoDebug("bridge.key_received","key",keyEvent.Key,"action",keyEvent.Action)
    if keyEvent.Key!="ai" && keyEvent.Key!="speech"{
        maohaoDebug("bridge.key_ignored","key",keyEvent.Key)
        return
    }
    m.onKey(frame.DeviceID,keyEvent.Key,keyEvent.Action)
}

func (m *maohaoMouseBridge) onKey(device,key,action string) {
    // AI is an app-wake button, not a microphone. Wake only on release/click
    // (some firmware emits both) so a press never starts audio capture.
    if key=="ai" {
        if action=="click" || action=="up" {
            maohaoDebug("bridge.ai_wake","action",action)
            if m.agent!=nil { m.agent.wake() }
            if m.app!=nil {
                m.app.showWindow()
                if m.app.window!=nil {
                    m.app.window.ExecJS("if (!location.pathname.startsWith('/chat')) location.assign('/chat');")
                }
            }
        }
        return
    }
    if key!="speech" { return }
    if m.agent!=nil && action=="down" {m.agent.rememberFocus(device)}
    switch action {
    case "down":
        m.mu.Lock()
        if m.key=="" {
            m.device,m.key,m.pcm = device,key,nil
            maohaoDebug("bridge.recording_started")
        } else {
            maohaoDebug("bridge.recording_down_ignored","recording_in_progress",true)
        }
        m.mu.Unlock()
        if m.agent!=nil{mouseCommandNotice("recording","正在说话")}
        m.emit("octop:maohao-status", map[string]string{"phase":"recording"})
    case "up":
        m.mu.Lock()
        if m.device!=device || m.key!=key {
            m.mu.Unlock()
            maohaoDebug("bridge.recording_up_ignored","reason","no_matching_key_down")
            return
        }
        pcm:=m.pcm
        m.device,m.key,m.pcm = "","",nil
        m.mu.Unlock()
        maohaoDebug("bridge.recording_finished","pcm_bytes",len(pcm))
        if len(pcm)<3200 {
            maohaoDebug("bridge.recording_discarded","reason","too_short","pcm_bytes",len(pcm))
            if m.agent!=nil{mouseCommandNotice("ready","")}
            m.emit("octop:maohao-status", map[string]string{"phase":"ready"})
            return
        }
        if m.agent!=nil {mouseCommandNotice("transcribing","识别中");m.agent.onAudio(device,buildMaohaoWav(pcm))} else {m.emit("octop:maohao-audio",map[string]string{"wavBase64":base64.StdEncoding.EncodeToString(buildMaohaoWav(pcm))})}
        m.emit("octop:maohao-status",map[string]string{"phase":"transcribing"})
    }
}

func buildMaohaoWav(pcm []byte) []byte {
    out:=bytes.NewBuffer(make([]byte,0,len(pcm)+44))
    out.WriteString("RIFF")
    _ = binary.Write(out,binary.LittleEndian,uint32(len(pcm)+36))
    out.WriteString("WAVEfmt ")
    _ = binary.Write(out,binary.LittleEndian,uint32(16))
    _ = binary.Write(out,binary.LittleEndian,uint16(1))
    _ = binary.Write(out,binary.LittleEndian,uint16(1))
    _ = binary.Write(out,binary.LittleEndian,uint32(16000))
    _ = binary.Write(out,binary.LittleEndian,uint32(32000))
    _ = binary.Write(out,binary.LittleEndian,uint16(2))
    _ = binary.Write(out,binary.LittleEndian,uint16(16))
    out.WriteString("data")
    _ = binary.Write(out,binary.LittleEndian,uint32(len(pcm)))
    out.Write(pcm)
    return out.Bytes()
}

func (m *maohaoMouseBridge) emit(name string,data any) {
    if m.agent!=nil {m.agent.broadcast(name,data);return}
    if m.app==nil || m.app.window==nil { return }
    jsonValue,err:=json.Marshal(data)
    if err!=nil { return }
    m.app.window.ExecJS("window.dispatchEvent(new CustomEvent("+
        strconv.Quote(name)+", {detail:"+string(jsonValue)+"}));")
}

func (m *maohaoMouseBridge) stop() {
    m.mu.Lock()
    m.stopped = true
    cmd,stdin:=m.cmd,m.stdin
    m.mu.Unlock()
    if cmd==nil || cmd.Process==nil { return }
    if stdin!=nil {
        _,_ = io.WriteString(stdin,"stop\n")
        _ = stdin.Close()
    }
    go func(){time.Sleep(2*time.Second);_ = cmd.Process.Kill()}()
}
