package main

import (
	"bufio"
	"encoding/json"
	"net"
	"os"
	"strings"
	"testing"
	"time"
)

func TestMaohaoAIKeyDoesNotStartVoiceRecording(t *testing.T) {
	m := &maohaoMouseBridge{}
	for _, action := range []string{"down", "up", "click"} {
		m.onKey("device", "ai", action)
		if m.key != "" || m.device != "" || len(m.pcm) != 0 {
			t.Fatalf("AI %s unexpectedly started voice recording", action)
		}
	}
	m.onKey("device", "speech", "down")
	if m.key != "speech" {
		t.Fatal("speech key must still start hold-to-talk")
	}
	m.onKey("device", "speech", "up")
	if m.key != "" {
		t.Fatal("speech key release did not reset recorder")
	}
}

func TestMaohaoAgentExecutableMatch(t *testing.T) {
	if !sameMaohaoExecutable(`D:\Program Files\imaohao\Octop Demo\Octop.exe`, `D:\Program Files\imaohao\Octop Demo\OCTOP.EXE`) {
		t.Fatal("same installation with casing difference not recognized")
	}
	for _, path := range []string{"", `C:\Users\baicai\Octop-Demo-Work\desktop\src\bin\Octop.exe`} {
		if sameMaohaoExecutable(path, `D:\Program Files\imaohao\Octop Demo\Octop.exe`) {
			t.Fatalf("stale agent executable %q incorrectly recognized", path)
		}
	}
}

func TestMaohaoAuthenticatedHandshakeIdentifiesAgentBinary(t *testing.T) {
	client, server := net.Pipe()
	defer client.Close()
	agent := &maohaoAgent{peers: make(map[*maohaoPeer]struct{})}
	const token = "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"
	go agent.handle(server, token)
	_ = client.SetDeadline(time.Now().Add(3 * time.Second))
	if _, err := client.Write([]byte("{\"token\":\"" + token + "\"}\n")); err != nil {
		t.Fatal(err)
	}
	line, err := bufio.NewReader(client).ReadBytes('\n')
	if err != nil {
		t.Fatal(err)
	}
	var packet maohaoPacket
	if err = json.Unmarshal(line, &packet); err != nil {
		t.Fatal(err)
	}
	if packet.Event != "octop:maohao-status" {
		t.Fatalf("unexpected handshake %q", packet.Event)
	}
	var status struct {
		Phase      string `json:"phase"`
		Executable string `json:"executable"`
	}
	if err = json.Unmarshal(packet.Detail, &status); err != nil {
		t.Fatal(err)
	}
	expected, _ := os.Executable()
	if status.Phase != "agent-connected" || !sameMaohaoExecutable(status.Executable, expected) {
		t.Fatalf("invalid agent handshake: %+v", status)
	}
	if strings.TrimSpace(status.Executable) == "" {
		t.Fatal("empty binary path")
	}
}
