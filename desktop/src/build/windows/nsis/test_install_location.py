"""Windows smoke test for the actual NSIS install-location helper.

No Octop software is installed or removed by this test. Only a temporary HKCU
fixture key is created, and deleted on exit. The installed HKLM key is read-only.
"""
import os
import shutil
import subprocess
import sys
import tempfile
import uuid
import winreg
from pathlib import Path

NSIS=Path(__file__).resolve().parent
EXECUTABLE="Octop.exe"
KEY_BASE=r"Software\Microsoft\Windows\CurrentVersion\Uninstall"

def compile_harness(nsis, scope, key_name, temp, stem):
    result = temp / (stem + ".txt")
    src = NSIS / ("_install_location_test_" + stem + ".nsi")
    exe = temp / (stem + ".exe")
    if scope=="user":
        directives='!define WAILS_INSTALL_SCOPE "user"\n'
        default = r"$LOCALAPPDATA\Programs\Octop Demo"
    else:
        directives='!define WAILS_INSTALL_SCOPE "machine"\n'
        default = r"$PROGRAMFILES64\Octop Demo"
    script=(
        '!define REQUEST_EXECUTION_LEVEL "user"\n'
        '!define ARG_WAILS_AMD64_BINARY "Octop.exe"\n'
        '!define INFO_PRODUCTNAME "Octop Demo"\n'
        +directives
        +f'!define UNINST_KEY_NAME "{key_name}"\n'
        '!include "wails_tools.nsh"\n'
        '!include "LogicLib.nsh"\n'
        '!include "install_location.nsh"\n'
        'Name "Octop Install Detection Test"\n'
        f'OutFile "{exe}"\n'
        f'InstallDir "{default}"\n'
        'SilentInstall silent\n'
        'Function .onInit\n'
        ' Call DetectExistingOctopDemo\n'
        'FunctionEnd\n'
        'Section\n'
        f' FileOpen $2 "{result}" w\n'
        ' FileWrite $2 "$INSTDIR|$OctopExistingInstall"\n'
        ' FileClose $2\n'
        'SectionEnd\n'
    )
    src.write_text(script,encoding="utf-8")
    try:
        result_build=subprocess.run([str(nsis),"-V2",str(src)],cwd=NSIS,capture_output=True,text=True,timeout=30)
    finally:
        src.unlink(missing_ok=True)
    if result_build.returncode:
        raise AssertionError("NSIS harness build: "+result_build.stdout+"\n"+result_build.stderr)
    return exe,result

def probe(exe,output):
    output.unlink(missing_ok=True)
    launched=subprocess.run([str(exe),"/S"],capture_output=True,text=True,timeout=15)
    if launched.returncode or not output.is_file():
        raise AssertionError(f"harness failed code={launched.returncode} stdout={launched.stdout} stderr={launched.stderr}")
    return output.read_text(encoding="utf-8")

def set_reg(name,value):
    with winreg.CreateKeyEx(winreg.HKEY_CURRENT_USER,KEY_BASE+r"\OctopDemoInstallLocationSmoke",0,winreg.KEY_WRITE|winreg.KEY_WOW64_64KEY) as k:
        winreg.SetValueEx(k,name,0,winreg.REG_SZ,str(value))

def del_reg(name):
    with winreg.CreateKeyEx(winreg.HKEY_CURRENT_USER,KEY_BASE+r"\OctopDemoInstallLocationSmoke",0,winreg.KEY_WRITE|winreg.KEY_WOW64_64KEY) as k:
        try:winreg.DeleteValue(k,name)
        except FileNotFoundError:pass

def main():
    installer=(NSIS/"project.nsi").read_text(encoding="utf-8-sig")
    assert "MUI_PAGE_CUSTOMFUNCTION_PRE SkipDirectoryForOctopUpgrade" in installer
    for hive in ("HKLM","HKCU"):
        assert f'WriteRegStr {hive} "'+chr(36)+'{UNINST_KEY}" "InstallLocation" "'+chr(36)+'INSTDIR"' in installer
    candidate=Path(sys.argv[1]) if len(sys.argv)>1 else None
    nsis=candidate or Path(shutil.which("makensis") or "")
    assert nsis.is_file(),f"makensis not found: {nsis}"
    with tempfile.TemporaryDirectory(prefix="octop_install_smoke_") as folder:
        temp=Path(folder)
        a=temp/"previous install with spaces"
        b=temp/"new saved install directory"
        for path in [a,b]:
            path.mkdir()
            (path/"Octop.exe").write_bytes(b"fixture")
            (path/"uninstall.exe").write_bytes(b"fixture")
        try:
            exe,output=compile_harness(nsis,"user","OctopDemoInstallLocationSmoke",temp,"fixture")
            expected=str(Path(os.environ["LOCALAPPDATA"])/"Programs"/"Octop Demo")
            actual=probe(exe,output)
            assert actual==expected+"|0",("fresh install",actual,expected)
            set_reg("DisplayIcon",a/"Octop.exe")
            actual=probe(exe,output)
            assert actual==str(a)+"|1",("legacy fallback",actual)
            set_reg("InstallLocation",b)
            actual=probe(exe,output)
            assert actual==str(b)+"|1",("persisted install path",actual)
            set_reg("InstallLocation",temp/"invalid folder")
            actual=probe(exe,output)
            assert actual==str(a)+"|1",("stale location recovery",actual)
            del_reg("DisplayIcon")
            actual=probe(exe,output)
            assert actual==expected+"|0",("stale registry ignored",actual)
            print("PASS five HKCU installer path cases")
        finally:
            try:winreg.DeleteKeyEx(winreg.HKEY_CURRENT_USER,KEY_BASE+r"\OctopDemoInstallLocationSmoke",winreg.KEY_WOW64_64KEY,0)
            except FileNotFoundError:pass
        # Read only: validate the actual HKLM Demo installation, if present.
        try:
            with winreg.OpenKey(winreg.HKEY_LOCAL_MACHINE,KEY_BASE+r"\OctopOctop Demo",0,winreg.KEY_READ|winreg.KEY_WOW64_64KEY) as k:
                _,_ = winreg.QueryValueEx(k,"DisplayIcon")
        except FileNotFoundError:
            print("SKIP actual machine-install key not present")
        else:
            exe,output=compile_harness(nsis,"machine","OctopOctop Demo",temp,"installed")
            actual=probe(exe,output)
            path,flag=actual.rsplit("|",1)
            assert flag=="1" and (Path(path)/"Octop.exe").is_file(),("actual installed path",actual)
            print("PASS machine HKLM existing installation:",path)

if __name__=="__main__":
    main()
