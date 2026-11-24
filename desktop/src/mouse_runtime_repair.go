package main

// Repair only missing SenseVoice files. Never replace the existing Octop
// Python runtime just to enable command recording.
import (
	"archive/zip"
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

var speechPackageDirs = []string{
	"packages/sherpa_onnx",
	"packages/sherpa_onnx-2.0.1.dist-info",
	"packages/sherpa_onnx_core-2.0.1.dist-info",
}

func openSpeechBundle() (*zip.Reader, func(), error) {
	if os.Getenv("OCTOP_DESKTOP_PORTABLE_ZIP") == "" && len(embeddedPortable) > 0 {
		z, err := zip.NewReader(bytes.NewReader(embeddedPortable), int64(len(embeddedPortable)))
		return z, func() {}, err
	}
	path, err := bundledPortableZip()
	if err != nil {
		return nil, nil, err
	}
	file, err := os.Open(path)
	if err != nil {
		return nil, nil, err
	}
	stat, err := file.Stat()
	if err != nil {
		file.Close()
		return nil, nil, err
	}
	z, err := zip.NewReader(file, stat.Size())
	if err != nil {
		file.Close()
		return nil, nil, err
	}
	return z, func() { _ = file.Close() }, nil
}

func speechBundleEntry(name string) (string, bool, error) {
	const prefix = "Octop-windows-amd64/"
	if !strings.HasPrefix(name, prefix) {
		return "", false, nil
	}
	rel := strings.TrimPrefix(name, prefix)
	selected := rel == "sensevoice_worker.py"
	for _, dir := range speechPackageDirs {
		if strings.HasPrefix(rel, dir+"/") {
			selected = true
			break
		}
	}
	if !selected {
		return "", false, nil
	}
	if strings.Contains(rel, "\\") {
		return "", false, errors.New("invalid speech bundle path")
	}
	for _, segment := range strings.Split(rel, "/") {
		if segment == "" || segment == "." || segment == ".." {
			return "", false, errors.New("unsafe speech bundle path")
		}
	}
	return rel, true, nil
}

func repairMaohaoSpeechRuntime(root string) error {
	return withPortableMutationLock(func() error {
		// Recheck inside the interprocess lock: another agent may have repaired it.
		if portableHasMaohaoOffline(root) {
			return nil
		}
		if !launchReady(root) {
			return errors.New("Octop portable runtime not ready")
		}
		archive, closeBundle, err := openSpeechBundle()
		if err != nil {
			return fmt.Errorf("open bundled SenseVoice files: %w", err)
		}
		defer closeBundle()
		stage, err := os.MkdirTemp(root, ".speech-repair-*")
		if err != nil {
			return err
		}
		defer os.RemoveAll(stage)
		count := 0
		for _, entry := range archive.File {
			rel, selected, err := speechBundleEntry(entry.Name)
			if err != nil {
				return err
			}
			if !selected || entry.FileInfo().IsDir() {
				continue
			}
			if entry.Mode()&os.ModeSymlink != 0 {
				return errors.New("speech bundle symlink unsupported")
			}
			if entry.UncompressedSize64 > 200*1024*1024 {
				return errors.New("speech file exceeds safety limit")
			}
			target := filepath.Join(stage, filepath.FromSlash(rel))
			if err := os.MkdirAll(filepath.Dir(target), 0700); err != nil {
				return err
			}
			input, err := entry.Open()
			if err != nil {
				return err
			}
			output, err := os.OpenFile(target, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0600)
			if err != nil {
				_ = input.Close()
				return err
			}
			_, copyErr := io.Copy(output, input)
			closeErr := output.Close()
			_ = input.Close()
			if copyErr != nil {
				return copyErr
			}
			if closeErr != nil {
				return closeErr
			}
			count++
		}
		if count < 3 {
			return errors.New("installer missing speech runtime files")
		}
		for _, rel := range []string{
			"sensevoice_worker.py", "packages/sherpa_onnx/__init__.py",
			"packages/sherpa_onnx/lib",
		} {
			if _, err := os.Stat(filepath.Join(stage, filepath.FromSlash(rel))); err != nil {
				return fmt.Errorf("incomplete bundled speech runtime %s: %w", rel, err)
			}
		}
		// Stage and verify first; publish whole package directories and finally the
		// worker script as readiness marker. Existing Octop files remain untouched.
		for _, rel := range speechPackageDirs {
			src := filepath.Join(stage, filepath.FromSlash(rel))
			if _, err := os.Stat(src); os.IsNotExist(err) {
				continue
			}
			target := filepath.Join(root, filepath.FromSlash(rel))
			if err := os.MkdirAll(filepath.Dir(target), 0700); err != nil {
				return err
			}
			if err := os.RemoveAll(target); err != nil {
				return err
			}
			if err := os.Rename(src, target); err != nil {
				return err
			}
		}
		target := filepath.Join(root, "sensevoice_worker.py")
		if err := os.Remove(target); err != nil && !os.IsNotExist(err) {
			return err
		}
		if err := os.Rename(filepath.Join(stage, "sensevoice_worker.py"), target); err != nil {
			return err
		}
		if !portableHasMaohaoOffline(root) {
			return errors.New("SenseVoice repair incomplete")
		}
		return nil
	})
}
