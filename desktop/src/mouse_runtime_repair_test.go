package main

import (
	"archive/zip"
	"bytes"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"testing"
)

func writeMaohaoRepairFixture(t *testing.T, incomplete bool) string {
	t.Helper()
	file := filepath.Join(t.TempDir(), "speech-fixture.zip")
	out, err := os.Create(file)
	if err != nil {
		t.Fatal(err)
	}
	archive := zip.NewWriter(out)
	data := map[string][]byte{
		"sensevoice_worker.py":                          []byte("# local speech worker"),
		"packages/sherpa_onnx/__init__.py":              []byte("# sherpa module"),
		"packages/sherpa_onnx-2.0.1.dist-info/METADATA": []byte("Version: 2.0.1"),
	}
	if !incomplete {
		data["packages/sherpa_onnx/lib/sherpa.dll"] = []byte("native-dll-fixture")
	}
	for name, body := range data {
		wr, err := archive.Create("Octop-windows-amd64/" + name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err = wr.Write(body); err != nil {
			t.Fatal(err)
		}
	}
	if err = archive.Close(); err != nil {
		t.Fatal(err)
	}
	if err = out.Close(); err != nil {
		t.Fatal(err)
	}
	return file
}
func mockPortable(t *testing.T) string {
	t.Helper()
	root := filepath.Join(t.TempDir(), "portable")
	if err := os.MkdirAll(filepath.Join(root, "runtime"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "launch.py"), []byte("print('ok')"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "runtime", "python.exe"), bytes.Repeat([]byte("x"), 2048), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "users.sqlite"), []byte("KEEP-USER-DATA"), 0600); err != nil {
		t.Fatal(err)
	}
	return root
}
func TestMaohaoSpeechRepairSelectiveAndConcurrent(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("Windows Maohao SDK runtime")
	}
	zipPath := writeMaohaoRepairFixture(t, false)
	t.Setenv("OCTOP_DESKTOP_PORTABLE_ZIP", zipPath)
	root := mockPortable(t)
	if portableHasMaohaoOffline(root) {
		t.Fatal("fake runtime unexpectedly ready")
	}
	const workers = 4
	errCh := make(chan error, workers)
	var wg sync.WaitGroup
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() { defer wg.Done(); errCh <- repairMaohaoSpeechRuntime(root) }()
	}
	wg.Wait()
	close(errCh)
	for err := range errCh {
		if err != nil {
			t.Fatalf("concurrent repair failed: %v", err)
		}
	}
	if !portableHasMaohaoOffline(root) {
		t.Fatal("speech runtime not installed")
	}
	data, err := os.ReadFile(filepath.Join(root, "users.sqlite"))
	if err != nil || string(data) != "KEEP-USER-DATA" {
		t.Fatal("existing user data changed")
	}
	for _, name := range []string{"portable.new", "portable.previous"} {
		if _, err := os.Stat(filepath.Join(filepath.Dir(root), name)); !os.IsNotExist(err) {
			t.Fatalf("unexpected full-runtime staging directory: %s", name)
		}
	}
}
func TestMaohaoSpeechRepairRejectsIncompleteBundleWithoutDataLoss(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("Windows Maohao SDK runtime")
	}
	t.Setenv("OCTOP_DESKTOP_PORTABLE_ZIP", writeMaohaoRepairFixture(t, true))
	root := mockPortable(t)
	if err := repairMaohaoSpeechRuntime(root); err == nil {
		t.Fatal("incomplete archive unexpectedly accepted")
	}
	if portableHasMaohaoOffline(root) {
		t.Fatal("incomplete archive marked ready")
	}
	data, err := os.ReadFile(filepath.Join(root, "users.sqlite"))
	if err != nil || string(data) != "KEEP-USER-DATA" {
		t.Fatal("original user data changed")
	}
	if _, err := os.Stat(filepath.Join(root, "sensevoice_worker.py")); !os.IsNotExist(err) {
		t.Fatal("worker published before validation")
	}
}

func TestMaohaoSpeechRepairFromRealBundledZip(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("Windows speech bundle")
	}
	zipPath := os.Getenv("OCTOP_MAOHAO_TEST_BUNDLE")
	if zipPath == "" {
		t.Skip("set OCTOP_MAOHAO_TEST_BUNDLE for real installer archive")
	}
	t.Setenv("OCTOP_DESKTOP_PORTABLE_ZIP", zipPath)
	root := mockPortable(t)
	if err := repairMaohaoSpeechRuntime(root); err != nil {
		t.Fatal(err)
	}
	if !portableHasMaohaoOffline(root) {
		t.Fatal("full installer archive did not repair speech")
	}
	info, err := os.Stat(filepath.Join(root, "packages", "sherpa_onnx", "lib"))
	if err != nil || !info.IsDir() {
		t.Fatal("native inference package missing")
	}
	data, err := os.ReadFile(filepath.Join(root, "users.sqlite"))
	if err != nil || string(data) != "KEEP-USER-DATA" {
		t.Fatal("portable user data altered")
	}
}
