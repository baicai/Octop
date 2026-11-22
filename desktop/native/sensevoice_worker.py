"""Octop Demo offline SenseVoice CPU worker (Windows x64).

The resident mouse agent owns this local process and communicates over pipes.
No microphone access, remote service, HTTP port or user credentials are used.
Each line on stdin contains {"wavBase64": "<WAV>"}; responses are JSON lines.
The caller verifies all model weights against the pinned DSH manifest.
"""
from __future__ import annotations

import base64
import io
import json
import os
import site
import sys
import wave
from pathlib import Path

ROOT = Path(__file__).resolve().parent
site.addsitedir(str(ROOT / "packages"))


def _decode_wave(encoded: str):
    import numpy as np

    if len(encoded) > 4_500_000:
        raise ValueError("Audio is too large")
    data = base64.b64decode(encoded, validate=True)
    if len(data) < 46 or len(data) > 16000 * 2 * 45 + 44:
        raise ValueError("Invalid recording size")
    with wave.open(io.BytesIO(data), "rb") as wav:
        if (wav.getnchannels(), wav.getsampwidth(), wav.getframerate(), wav.getcomptype()) != (
            1, 2, 16000, "NONE"
        ):
            raise ValueError("Expected 16-kHz, mono, PCM16 WAV")
        frames = wav.readframes(wav.getnframes())
    if len(frames) < 3200:
        raise ValueError("Recording too short")
    return np.frombuffer(frames, dtype="<i2").astype("float32") / 32768.0


def main() -> None:
    if len(sys.argv) != 2:
        raise ValueError("Expected the model cache directory")
    from sherpa_onnx import OfflineRecognizer

    model_root = Path(sys.argv[1]).resolve()
    # Directories are selected by the native agent; no model paths are supplied
    # over the worker's IPC channel.
    recognizer = OfflineRecognizer.from_sense_voice(
        model=str(model_root / "model.int8.onnx"),
        tokens=str(model_root / "tokens.txt"),
        num_threads=2,
        language="auto",
        use_itn=True,
        debug=False,
    )
    print(json.dumps({"ready": True}), flush=True)

    for line in sys.stdin:
        try:
            if len(line) > 4_500_000:
                raise ValueError("Oversized request")
            request = json.loads(line)
            samples = _decode_wave(request["wavBase64"])
            stream = recognizer.create_stream()
            stream.accept_waveform(16000, samples)
            recognizer.decode_stream(stream)
            result = str(stream.result.text).strip()
            # ASCII-only wire JSON stays intact across legacy Windows code pages.
            print(json.dumps({"text": result}, ensure_ascii=True), flush=True)
        except Exception:
            # Do not reflect paths, transcripts, machine identifiers, or
            # exceptions to the caller. Log failure status without audio.
            print(json.dumps({"error": "recognition-failed"}), flush=True)


if __name__ == "__main__":
    try:
        main()
    except Exception as exc:
        print(json.dumps({"error": type(exc).__name__}), file=sys.stderr, flush=True)
        sys.exit(1)
