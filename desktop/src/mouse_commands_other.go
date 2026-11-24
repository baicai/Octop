//go:build !windows

package main
import("os";"errors")
func replaceMouseCommandFile(src,dst string)error{return os.Rename(src,dst)}
func mouseLaunchApp(string)error{return errors.New("Windows only")}
func mouseOpenURL(string)error{return errors.New("Windows only")}
func mouseCloseApp(string)error{return errors.New("Windows only")}
func mouseNoticeSet(string,string){}
func mouseNoticeClearError(string){}

func mouseNoticeLoop(){}
