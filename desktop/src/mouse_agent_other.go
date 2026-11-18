//go:build !windows
package main
import "errors"
func maohaoRegisterLoginStart()error{return nil}
func maohaoForegroundWindow()uintptr{return 0}
func maohaoIsOctopForeground(uintptr)bool{return false}
func maohaoTypeUnicode(string)error{return errors.New("Windows only")}
func maohaoOpenWeChat()error{return errors.New("Windows only")}
