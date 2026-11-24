# Prefer a validated previous Octop Demo installation for upgrades.
# Old packages stored DisplayIcon but not InstallLocation.
Var OctopExistingInstall

Function DetectExistingOctopDemo
    StrCpy $OctopExistingInstall 0
    SetRegView 64

    !if "${WAILS_INSTALL_SCOPE}" == "user"
        ReadRegStr $0 HKCU "${UNINST_KEY}" "InstallLocation"
    !else
        ReadRegStr $0 HKLM "${UNINST_KEY}" "InstallLocation"
    !endif

    StrCmp $0 "" legacyDisplayIcon
    IfFileExists "$0\${PRODUCT_EXECUTABLE}" 0 legacyDisplayIcon
    IfFileExists "$0\uninstall.exe" 0 legacyDisplayIcon
    StrCpy $INSTDIR $0
    StrCpy $OctopExistingInstall 1
    Return

    legacyDisplayIcon:
    !if "${WAILS_INSTALL_SCOPE}" == "user"
        ReadRegStr $0 HKCU "${UNINST_KEY}" "DisplayIcon"
    !else
        ReadRegStr $0 HKLM "${UNINST_KEY}" "DisplayIcon"
    !endif

    StrCmp $0 "" done
    ${GetParent} "$0" $1
    IfFileExists "$1\${PRODUCT_EXECUTABLE}" 0 done
    IfFileExists "$1\uninstall.exe" 0 done
    StrCpy $INSTDIR $1
    StrCpy $OctopExistingInstall 1
    done:
FunctionEnd

Function SkipDirectoryForOctopUpgrade
    StrCmp $OctopExistingInstall 1 0 done
    Abort
    done:
FunctionEnd
