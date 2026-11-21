package main

import (
    "bytes"
    "encoding/binary"
    "encoding/json"
    "testing"
)

func TestMaohaoWAVHeader(t *testing.T) {
    pcm:=bytes.Repeat([]byte{0,1},16000)
    wav:=buildMaohaoWav(pcm)
    if len(wav)!=len(pcm)+44 || string(wav[:4])!="RIFF" || string(wav[8:12])!="WAVE" { t.Fatal("WAV header") }
    if binary.LittleEndian.Uint32(wav[24:28])!=16000 { t.Fatal("sample rate") }
    if binary.LittleEndian.Uint16(wav[22:24])!=1 { t.Fatal("mono channel") }
    if !bytes.Equal(wav[44:],pcm) { t.Fatal("PCM differs") }
}
func TestMaohaoAudioRequiresKeyHold(t *testing.T) {
    m:=&maohaoMouseBridge{}
    send:=func(v any){ b,_:=json.Marshal(v);m.consumeLine(b) }
    packet:=map[string]any{"kind":"audio","deviceId":"m1","format":"pcm_s16le","sampleRate":16000,"data":"AQABAA=="}
    send(packet)
    if len(m.pcm)!=0 { t.Fatal("captured audio without key") }
    m.onKey("m1","speech","down")
    send(packet)
    if len(m.pcm)!=4 { t.Fatal("no audio on held key") }
    m.onKey("m1","speech","up")
    if len(m.pcm)!=0 || m.key!="" { t.Fatal("recording did not stop") }
}
