//go:build windows

package main

// Serialize portable directory mutations across the Windows GUI and the
// resident Maohao agent, which run as independent processes.
import (
	"errors"
	"fmt"
	"runtime"
	"syscall"
	"unsafe"
)

func withPortableMutationLock(action func() error) error {
	// Windows mutex ownership is OS-thread-affine; Go goroutines may migrate.
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	kernel := syscall.NewLazyDLL("kernel32.dll")
	create := kernel.NewProc("CreateMutexW")
	wait := kernel.NewProc("WaitForSingleObject")
	release := kernel.NewProc("ReleaseMutex")
	closeHandle := kernel.NewProc("CloseHandle")
	name, err := syscall.UTF16PtrFromString("Local\\OctopDemoPortableMutation")
	if err != nil {
		return err
	}
	handle, _, callErr := create.Call(0, 0, uintptr(unsafe.Pointer(name)))
	if handle == 0 {
		return fmt.Errorf("create portable runtime mutex: %w", callErr)
	}
	defer closeHandle.Call(handle)
	state, _, waitErr := wait.Call(handle, 600000)
	if state != 0 && state != 0x80 {
		if state == 0x102 {
			return errors.New("portable runtime update lock timed out")
		}
		return fmt.Errorf("wait for portable runtime lock (%d): %w", state, waitErr)
	}
	defer release.Call(handle)
	return action()
}
