"""AI short-film factory.

A "short film" = 60-180s vertical narrative video:
  LLM script -> scene breakdown -> image per scene (injected image_fn) ->
  Ken Burns motion -> TTS voiceover -> subtitles -> 1080x1920 mp4.

Fully assembled with FFmpeg (no GPU needed). The image_fn is injected:
in production it calls an image model; in tests it renders color slides.
Nothing here pretends to be Sora/Veo-class video generation — this is the
honest, shippable v1: illustrated narration films.
"""
from __future__ import annotations

import json
import subprocess
import sys
import wave
from pathlib import Path
sys.path.insert(0, ".")

W, H, FPS = 1080, 1920, 30


# ---------------------------------------------------------------- planning
def plan_film(llm, topic: str, target_seconds: int = 90,
              max_scenes: int = 6) -> dict:
    """Ask the LLM for a scene-by-scene film plan. Returns
    {"title": str, "scenes": [{"narration": str, "image_prompt": str,
    "seconds": int}]}."""
    n = max(2, min(max_scenes, max(2, target_seconds // 15)))
    res = llm.run(
        system=("Bạn viết kịch bản phim ngắn dọc TikTok, tiếng Việt. "
                "Chỉ trả lời JSON thuần: "
                "{\"title\": \"...\", \"scenes\": [{\"narration\": \"lời dẫn "
                "khoảng 20-30 từ\", \"image_prompt\": \"mô tả hình ảnh "
                "minh hoạ bằng tiếng Anh\", \"seconds\": 15}]}."),
        prompt=(f"Chủ đề phim: {topic}. Viết kịch bản {n} cảnh, "
                f"tổng khoảng {target_seconds} giây, có mở đầu gây tò mò "
                f"và kết thúc đọng lại."),
    )
    if not res.ok:
        raise RuntimeError(f"LLM failed: {res.error}")
    raw = str(res.payload).strip()
    if raw.startswith("```"):
        raw = raw.split("```")[1]
        if raw.lstrip().startswith("json"):
            raw = raw.lstrip()[4:]
    plan = json.loads(raw.strip())
    scenes = [s for s in plan.get("scenes", [])
              if s.get("narration")][:max_scenes]
    if not scenes:
        raise RuntimeError("LLM returned no scenes")
    return {"title": str(plan.get("title", topic))[:100], "scenes": scenes}


# ---------------------------------------------------------------- audio
def wav_seconds(path: Path) -> float:
    with wave.open(str(path), "rb") as w:
        return w.getnframes() / float(w.getframerate() or 24000)


def synth_narration(tts, text: str, voice: str, out: Path) -> Path:
    """TTS -> WAV file. Accepts payload as bytes or existing path."""
    res = tts.run(text, voice=voice)
    if not res.ok:
        raise RuntimeError(f"TTS failed: {res.error}")
    if isinstance(res.payload, (bytes, bytearray)):
        out.write_bytes(bytes(res.payload))
    else:
        src = Path(str(res.payload))
        out.write_bytes(src.read_bytes())
    return out


# ---------------------------------------------------------------- video
def _ff(*args: str) -> None:
    p = subprocess.run(["ffmpeg", "-y", *args],
                       capture_output=True, text=True)
    if p.returncode != 0:
        raise RuntimeError(f"ffmpeg failed: {p.stderr[-400:]}")


def render_scene(image: Path, audio: Path, seconds: float, out: Path) -> Path:
    """Still image + Ken Burns slow zoom + narration -> scene mp4."""
    dur = max(1.0, seconds)
    _ff("-loop", "1", "-framerate", str(FPS), "-i", str(image),
        "-i", str(audio),
        "-filter_complex",
        f"[0:v]scale={W}:{H}:force_original_aspect_ratio=increase,"
        f"crop={W}:{H},"
        f"zoompan=z='min(zoom+0.0012,1.25)':d=1:"
        f"x='iw/2-(iw/zoom/2)':y='ih/2-(ih/zoom/2)':"
        f"s={W}x{H}:fps={FPS}[v]",
        "-map", "[v]", "-map", "1:a",
        "-t", f"{dur:.2f}",
        "-c:v", "libx264", "-preset", "veryfast", "-pix_fmt", "yuv420p",
        "-c:a", "aac", "-shortest", str(out))
    return out


def _srt_time(sec: float) -> str:
    ms = int(sec * 1000)
    h, ms = divmod(ms, 3600000)
    m, ms = divmod(ms, 60000)
    s, ms = divmod(ms, 1000)
    return f"{h:02d}:{m:02d}:{s:02d},{ms:03d}"


def build_srt(narrations: list[str], durations: list[float]) -> str:
    """One subtitle entry per scene, timed to the narration."""
    lines, t = [], 0.0
    for i, (text, d) in enumerate(zip(narrations, durations), 1):
        lines.append(f"{i}\n{_srt_time(t)} --> {_srt_time(t + d)}\n"
                     f"{text.strip()}\n")
        t += d
    return "\n".join(lines)


def assemble_film(scenes: list[Path], srt_text: str, out: Path) -> Path:
    """Concat scene mp4s + mux subtitles -> final vertical film."""
    lst = out.parent / "concat.txt"
    lst.write_text("".join(f"file '{s.resolve()}'\n" for s in scenes))
    srt = out.parent / "subs.srt"
    srt.write_text(srt_text, encoding="utf-8")
    _ff("-f", "concat", "-safe", "0", "-i", str(lst),
        "-i", str(srt),
        "-c:v", "copy", "-c:a", "aac", "-c:s", "mov_text",
        "-metadata:s:s:0", "language=vie",
        str(out))
    return out


# ---------------------------------------------------------------- pipeline
def make_film(topic: str, llm, tts, image_fn, workdir: Path,
              voice: str = "narrator",
              target_seconds: int = 90) -> dict:
    """End-to-end: plan -> images -> narration -> scenes -> final mp4.

    image_fn(prompt: str, out: Path) -> Path  (injected; AI image model
    in production, color slide in tests).
    Returns {"ok", "path", "title", "seconds", "scenes"}.
    """
    workdir.mkdir(parents=True, exist_ok=True)
    plan = plan_film(llm, topic, target_seconds)
    scenes, narrations, durations = [], [], []

    for i, sc in enumerate(plan["scenes"]):
        img = workdir / f"scene{i}.png"
        image_fn(sc.get("image_prompt", topic), img)
        wav = workdir / f"scene{i}.wav"
        synth_narration(tts, sc["narration"], voice, wav)
        dur = max(float(sc.get("seconds", 0) or 0), wav_seconds(wav))
        mp4 = workdir / f"scene{i}.mp4"
        render_scene(img, wav, dur, mp4)
        scenes.append(mp4)
        narrations.append(sc["narration"])
        durations.append(dur)

    out = workdir / "film.mp4"
    assemble_film(scenes, build_srt(narrations, durations), out)
    return {"ok": True, "path": str(out), "title": plan["title"],
            "seconds": round(sum(durations), 1),
            "scenes": len(scenes)}
