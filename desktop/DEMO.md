# Octop Demo — Windows x64

This branch makes an isolated, chat-first **demo edition** of the existing Octop desktop. The full Octop backend is bundled, rather than replaced by fake AI responses.

## Differences from standard Octop

- **Octop Demo** appears separately in Windows Start menu and installed apps; its NSIS directory and uninstall registry key are distinct.
- Runtime, SQLite database, sessions, model credentials, and settings default to `%USERPROFILE%\\.octop-demo`, not `%USERPROFILE%\\.octop`.
- Default local service port is **18088**, rather than **8088**.
- Default interface language is Chinese; existing saved locale still wins.
- Chat-first **minimal** navigation is the default; users can change layout.
- No voice recognition, hardware/mouse SDK or offline ASR model is bundled.

## Build

This branch configures pull-request packaging to build **windows-amd64** on a native Windows GitHub Actions runner. The job prepares a bundled Python 3.12 runtime, builds the React dashboard and Wails 3 GUI, and creates the native NSIS installer. The downloadable workflow artifact is `Octop-Demo-windows-amd64-<version>.exe`.

See `desktop/README.md` and `.github/workflows/octop-desktop.yml` for tool versions and packaging steps. The full GitHub Release process is unchanged for official tags; do not merge this demo-only CI adjustment into upstream without reviewing it.

## First-run acceptance

1. Install the Windows x64 installer; check that **Octop Demo** is separate from Octop.
2. Open Octop Demo. First startup extracts its bundled runtime; no speech model is downloaded.
3. Complete the upstream **first-run administrator setup**. Configure an actual model provider/API key in model settings; credentials are **not** shipped in the package.
4. Send a chat message, check an actual model response, start another chat, reopen history, and try switching experts (if configured).
5. Check that minimizing/closing goes to the tray and that quitting stops its own service.
6. Check `.octop-demo` is used, `.octop` is untouched, and the demo uses port 18088 unless occupied.
7. Uninstall the demo and confirm the standard Octop installation remains.

## Notes

Explicit `OCTOP_HOME` and `OCTOP_DESKTOP_URL` environment variables still override defaults for developer use. The normal demo uses neither. The data directory should not be deleted automatically on uninstall: it may contain user chats and credentials.

This is a real functional demo **baseline**, not a completely redesigned DSH interface. A CI build alone is not proof of real Windows installation, live AI provider connectivity, or mouse hardware compatibility; those require separate Windows acceptance.
