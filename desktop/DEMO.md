# Octop Demo — Windows x64

This branch makes an isolated, chat-first **demo edition** of the existing Octop desktop. The full Octop backend is bundled, rather than replaced by fake AI responses.

## Differences from standard Octop

- **Octop Demo** appears separately in Windows Start menu and installed apps; its NSIS directory and uninstall registry key are distinct.
- Runtime, SQLite database, sessions, model credentials, and settings default to `%USERPROFILE%\\.octop-demo`, not `%USERPROFILE%\\.octop`.
- Default local service port is **18088**, rather than **8088**.
- Default interface language is Chinese; existing saved locale still wins.
- Chat-first **minimal** navigation is the default; users can change layout.
- The Windows evaluation bundles the vendor uMouse SDK 0.4.20 bridge and a resident mouse agent; it does not yet bundle a ready-to-use offline ASR model.

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


## Maohao mouse SDK Windows evaluation

This test client uses the **customer supplied uMouse SDK v0.4.20**, pinned to `baicai/mouse-sdk` commit `b2a7355`. The isolated native helper and signed-off Windows build procedure are reused from `baicai/deepseek-harness` DSH-Maohao. Only the SDK version and binary ABI are verified in CI; **real hardware must still be tested with the user's Maohao mouse**.

With a configured Octop STT provider: AI key opens the existing Octop chat, speech/AI key hold captures **mouse SDK PCM** (not the PC mic), release transcodes to WAV, existing authenticated `/voice/stt` transcribes, and the text is automatically sent to the currently selected Agent. Multi-user and Agent management remain unchanged.

**Limitations:** Existing Octop STT adapters require a configured provider (currently no bundled offline SenseVoice). The native bridge runs while Octop is open or minimized, not after full quit. Do not publicly redistribute vendor SDK binaries without confirming vendor commercial permission; this is an evaluation build for the customer.


## Resident Maohao mouse agent
On Windows the SDK is now owned by a separate `Octop.exe --maohao-agent` process, not by the Wails window. This is a **per-user, interactive-session resident agent**, not a Session 0 NT service. First GUI start registers an HKCU Run entry for the next Windows sign-in. Its authenticated loopback event stream is for GUI notifications only; recording is triggered by hardware key-down events, never by IPC commands. The agent continues running if Octop's window is fully closed. A physical AI-key click starts/wakes the Octop window.

**Offline background ASR:** Windows Demo now packages a pinned `sherpa-onnx==2.0.1` CPU binding and a persistent Python SenseVoice worker. On the first voice request, the resident agent downloads DSH's revision-pinned `model.int8.onnx` (~239 MB) and `tokens.txt` into `.octop-demo/speech-models/sensevoice-int8/`, verifies file lengths and SHA-256, and reuses verified assets after restart. Model downloads and inference are asynchronous relative to the Wails GUI. A failed download emits a readiness error rather than blocking the client. Existing fully verified model files require no network access. Model preparation still depends on network access for missing assets and has not yet been verified on real Windows mouse hardware.

For authorized private installations, `OCTOP_MAOHAO_MODEL_DIR` may reference another absolute directory containing the pinned model files, and `OCTOP_MAOHAO_LOCAL_STT_URL` may override the built-in recognizer with a loopback-only STT endpoint. The resident agent recognizes exact "打开微信" utterances; other spoken text is inserted using Windows Unicode SendInput only when the original foreground window remains active. These OS actions need real Windows hardware testing.

Resident agent authentication uses a random **per-process-session** loopback token under `.octop-demo/maohao-agent.token`. The agent replaces it on startup and deletes it on clean exit. This is an IPC authorization secret, not a server identity or a telemetry device fingerprint.

Uninstall sends an authenticated local shutdown request to the agent before deleting the installed application, then removes the HKCU Run entry. It does not kill other programs by executable name.

The foreground Octop chat receives locally recognized text through the `octop:maohao-transcript` event and submits it to the selected Agent; there is no implicit remote speech upload. The old `/voice/stt` path remains available for Octop's own browser microphone action.

The NSIS installer registers the per-user mouse agent and starts it immediately after installation; opening Octop's UI is not required for hardware event detection. If the bundled Python runtime is not yet extracted, the first background ASR request extracts it from the installer payload without requiring a separate Python installation. Offline model files are downloaded only on first voice use and SHA-256 verified; later runs reuse the cache.
