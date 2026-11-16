#!/usr/bin/env python3
"""Generate human-sounding Japanese TTS for every planned object and upload to MinIO.

Reads data/audio/tts_plan/dry_run_manifest.csv. Object keys are
`<prefix>sha256(original_text)[:16].wav`; the audio itself is synthesized from
the text with a trailing 。 when the text has no sentence-final punctuation.

Requires:
  PYTHONPATH pointing at a directory containing edge_tts and minio
  PG* env vars for psql
  MINIO_ENDPOINT / MINIO_ACCESS_KEY / MINIO_SECRET_KEY / MINIO_BUCKET_AUDIO

Default is a no-op preview. Pass --apply to actually synthesize/upload/write PG.
Progress is recorded in apply_progress.jsonl so the run can be resumed safely.
"""

from __future__ import annotations

import argparse
import asyncio
import csv
import hashlib
import io
import json
import os
import subprocess
import sys
import tempfile
import wave
from array import array
from datetime import datetime, timezone
from pathlib import Path

import edge_tts
from minio import Minio

ROOT = Path(__file__).resolve().parents[1]
PLAN_DIR = ROOT / "data" / "audio" / "tts_plan"
BUCKET = os.environ.get("MINIO_BUCKET_AUDIO", "japanese")
VOICE = "ja-JP-NanamiNeural"
RATE = "-12%"
PITCH = "-3Hz"
END_PUNCT = ("。", "！", "？", "!", "?", ".", "、")


def quote(text: str) -> str:
    return "'" + text.replace("'", "''") + "'"


def tts_input(text: str) -> str:
    stripped = text.strip()
    # A single kana such as こ produces silent audio when a sentence-final
    # 。 is appended by the neural voice, so keep very short tokens bare.
    if len(stripped) <= 2:
        return stripped
    return stripped if stripped.endswith(END_PUNCT) else stripped + "。"


def convert_and_trim_ex(mp3_path: Path, work_dir: Path) -> tuple[bytes, float]:
    """MP3 -> WAV 16bit/24kHz/mono; return (wav bytes, trimmed lead ms).

    The trimmed lead is needed to translate edge-tts SentenceBoundary offsets
    (measured from the start of the untrimmed audio) onto the stored WAV.
    """
    wav_path = work_dir / (mp3_path.stem + ".wav")
    subprocess.run(
        ["afconvert", "-f", "WAVE", "-d", "LEI16@24000", "-c", "1",
         str(mp3_path), str(wav_path)],
        check=True, capture_output=True,
    )
    with wave.open(str(wav_path), "rb") as src:
        rate = src.getframerate()
        params = src.getparams()
        samples = array("h")
        samples.frombytes(src.readframes(src.getnframes()))
    mp3_path.unlink(missing_ok=True)
    wav_path.unlink(missing_ok=True)

    loud = [i for i, value in enumerate(samples) if abs(value) > 250]
    if not loud:
        raise ValueError("synthesized audio is silent")
    start = max(0, loud[0] - int(rate * 0.08))
    end = min(len(samples), loud[-1] + int(rate * 0.18))

    buffer = io.BytesIO()
    with wave.open(buffer, "wb") as out:
        out.setnchannels(params.nchannels)
        out.setsampwidth(params.sampwidth)
        out.setframerate(rate)
        out.writeframes(samples[start:end].tobytes())
    return buffer.getvalue(), start / rate * 1000.0


def convert_and_trim(mp3_path: Path, work_dir: Path) -> bytes:
    """MP3 -> trimmed WAV, discarding the lead-offset metadata."""
    wav, _lead_ms = convert_and_trim_ex(mp3_path, work_dir)
    return wav


def build_client() -> Minio:
    endpoint = os.environ["MINIO_ENDPOINT"].replace("http://", "").replace("https://", "")
    return Minio(
        endpoint,
        access_key=os.environ["MINIO_ACCESS_KEY"],
        secret_key=os.environ["MINIO_SECRET_KEY"],
        secure=False,
    )


def load_manifest() -> list[dict]:
    with (PLAN_DIR / "dry_run_manifest.csv").open(encoding="utf-8") as f:
        return list(csv.DictReader(f))


def load_progress() -> dict[str, dict]:
    path = PLAN_DIR / "apply_progress.jsonl"
    if not path.exists():
        return {}
    out = {}
    for line in path.read_text(encoding="utf-8").splitlines():
        if line.strip():
            rec = json.loads(line)
            out[rec["object_key"]] = rec
    return out


def sync_pg(progress: dict[str, dict], manifest: list[dict]) -> None:
    """Upsert audio_objects and backfill all FK columns in one transaction."""
    by_key = {row["object_key"]: row for row in manifest}
    lines = ["BEGIN;"]
    for key, rec in progress.items():
        kind = "word_pronunciation" if key.startswith("words/") else "example_audio"
        lines.append(
            "INSERT INTO audio_objects "
            "(bucket, object_key, kind, visibility, content_sha256, size_bytes, mime_type) "
            f"VALUES ({quote(BUCKET)}, {quote(key)}, {quote(kind)}, 'public', "
            f"{quote(rec['sha256'])}, {rec['size']}, 'audio/wav') "
            "ON CONFLICT (bucket, object_key) DO UPDATE SET "
            "kind = EXCLUDED.kind, visibility = 'public', "
            "content_sha256 = EXCLUDED.content_sha256, "
            "size_bytes = EXCLUDED.size_bytes, mime_type = EXCLUDED.mime_type, "
            "updated_at = now();"
        )
    lines.append(
        "CREATE TEMP TABLE _tts_map "
        "(text TEXT, object_key TEXT, prefix TEXT) ON COMMIT DROP;"
    )
    for key, rec in progress.items():
        row = by_key.get(key)
        if not row or key.startswith("lessons/"):
            continue
        lines.append(
            f"INSERT INTO _tts_map VALUES ({quote(row['text'])}, "
            f"{quote(key)}, {quote(row['prefix'])});"
        )
    join = (
        "FROM _tts_map m JOIN audio_objects ao "
        f"ON ao.object_key = m.object_key AND ao.bucket = {quote(BUCKET)} "
    )
    lines.append(
        "UPDATE words t SET audio_object_id = ao.id " + join +
        "WHERE m.prefix = 'words/' AND t.reading = m.text;"
    )
    for table, column in (
        ("word_examples", "japanese"),
        ("grammar_examples", "japanese"),
        ("speaking_materials", "text"),
    ):
        lines.append(
            f"UPDATE {table} t SET audio_object_id = ao.id " + join +
            f"WHERE m.prefix = 'examples/' AND t.{column} = m.text;"
        )
    lines.append("COMMIT;")

    sql_path = Path("/tmp/tts_sync.sql")
    sql_path.write_text("\n".join(lines) + "\n", encoding="utf-8")
    subprocess.run(
        ["psql", "-w", "-v", "ON_ERROR_STOP=1", "-f", str(sql_path)],
        check=True, capture_output=True, text=True,
    )


async def process_one(
    row: dict,
    sem: asyncio.Semaphore,
    client: Minio,
    work_dir: Path,
) -> dict:
    key = row["object_key"]
    digest = key.split("/")[-1].removesuffix(".wav")
    text = tts_input(row["text"])
    last_error = None

    for attempt in range(3):
        try:
            async with sem:
                mp3_path = work_dir / f"{digest}.mp3"
                await edge_tts.Communicate(
                    text, VOICE, rate=RATE, pitch=PITCH
                ).save(str(mp3_path))
                wav_bytes = await asyncio.to_thread(
                    convert_and_trim, mp3_path, work_dir
                )
                await asyncio.to_thread(
                    client.put_object,
                    BUCKET, key, io.BytesIO(wav_bytes), len(wav_bytes),
                    content_type="audio/wav",
                )
            return {
                "object_key": key,
                "sha256": hashlib.sha256(wav_bytes).hexdigest(),
                "size": len(wav_bytes),
                "ts": datetime.now(timezone.utc).isoformat(),
            }
        except Exception as exc:  # retry with a brief pause
            last_error = exc
            print(f"  retry {attempt + 1}/3 {key}: {type(exc).__name__}: {exc}",
                  file=sys.stderr, flush=True)
            await asyncio.sleep(0.8 * (attempt + 1))
    raise RuntimeError(f"{key}: {last_error}")


async def run(args, pending: list[dict], client: Minio, progress: dict[str, dict]) -> None:
    sem = asyncio.Semaphore(args.concurrency)
    progress_path = PLAN_DIR / "apply_progress.jsonl"
    failures: list[dict] = []
    done = 0

    with tempfile.TemporaryDirectory(prefix="tts_apply_") as tmp:
        work_dir = Path(tmp)
        tasks = [
            asyncio.create_task(process_one(row, sem, client, work_dir))
            for row in pending
        ]
        for coro in asyncio.as_completed(tasks):
            try:
                rec = await coro
            except Exception as exc:
                failures.append({"object_key": str(exc), "error": str(exc)})
                continue
            progress[rec["object_key"]] = rec
            with progress_path.open("a", encoding="utf-8") as f:
                f.write(json.dumps(rec, ensure_ascii=False) + "\n")
            done += 1
            if done % 100 == 0:
                print(f"  done {done}/{len(pending)}", flush=True)

    if failures:
        with (PLAN_DIR / "apply_failures.csv").open("w", newline="", encoding="utf-8") as f:
            writer = csv.DictWriter(f, fieldnames=["object_key", "error"])
            writer.writeheader()
            writer.writerows(failures)
    print(f"synth+upload: ok={done} failed={len(failures)}", flush=True)
    return failures


def main() -> int:
    parser = argparse.ArgumentParser()
    parser.add_argument("--apply", action="store_true", help="actually write to MinIO and PG")
    parser.add_argument("--limit", type=int, default=0, help="0 = all pending rows")
    parser.add_argument("--mixed", action="store_true", help="sample both EXISTS and MISSING rows")
    parser.add_argument("--concurrency", type=int, default=12)
    args = parser.parse_args()

    manifest = load_manifest()
    progress = load_progress()
    pending = [row for row in manifest if row["object_key"] not in progress]

    if args.mixed:
        exists = [r for r in pending if r["status"] == "EXISTS"]
        missing = [r for r in pending if r["status"] == "MISSING"]
        half = max(1, args.limit // 2)
        pending = (exists[:half] + missing[:half])[:args.limit]
    elif args.limit:
        pending = pending[:args.limit]

    print(f"bucket={BUCKET} manifest={len(manifest)} already_done={len(progress)} "
          f"pending={len(pending)} apply={args.apply} concurrency={args.concurrency}", flush=True)
    if not args.apply:
        print("preview only, pass --apply to execute")
        return 0
    if not pending:
        print("nothing to do")
        return 0

    client = build_client()
    failures = asyncio.run(run(args, pending, client, progress))

    print("syncing PostgreSQL metadata...", flush=True)
    try:
        sync_pg(progress, manifest)
        print("PG sync OK", flush=True)
    except subprocess.CalledProcessError as exc:
        print(f"PG sync FAILED: {exc.stderr}", file=sys.stderr, flush=True)
        return 1
    return 1 if failures else 0


if __name__ == "__main__":
    raise SystemExit(main())
