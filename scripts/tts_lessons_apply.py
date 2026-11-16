#!/usr/bin/env python3
"""Synthesize one full human-sounding audio per lesson and register it.

Lesson text is concatenated from lesson_sentences.tokens surfaces. Object keys
are lessons/lesson-<id>.wav because the legacy SQLite lessons.audio_url mapping
is empty, so there is no historical filename to preserve.

NOTE: lesson_sentences.start_ms / end_ms are the old fixed 3-second-per-sentence
markers. They are intentionally left untouched; re-synthesizing a full lesson
changes the real timing, so shadowing alignment must be recalibrated later.
"""

from __future__ import annotations

import argparse
import asyncio
import csv
import hashlib
import io
import os
import subprocess
import sys
import tempfile
from collections import defaultdict
from pathlib import Path

import edge_tts

sys.path.insert(0, str(Path(__file__).resolve().parent))
import tts_minio_apply as base  # noqa: E402

ROOT = Path(__file__).resolve().parents[1]
BUCKET = os.environ.get("MINIO_BUCKET_AUDIO", "japanese")
PREFIX = "lessons/"


def psql_copy(sql: str) -> list[list[str]]:
    out = subprocess.check_output(
        ["psql", "-w", "-c", f"COPY ({sql}) TO STDOUT WITH CSV"], text=True
    )
    return [row for row in csv.reader(io.StringIO(out)) if row]


def lesson_texts() -> dict[int, str]:
    rows = psql_copy(
        "SELECT s.lesson_id, s.position, "
        "string_agg(x.elem->>'surface', '' ORDER BY x.ord) "
        "FROM lesson_sentences s, "
        "jsonb_array_elements(s.tokens) WITH ORDINALITY AS x(elem, ord) "
        "GROUP BY s.lesson_id, s.position ORDER BY s.lesson_id, s.position"
    )
    grouped: dict[int, list[str]] = defaultdict(list)
    for lesson_id, _position, sentence in rows:
        grouped[int(lesson_id)].append(sentence)
    return {lesson_id: "".join(parts) for lesson_id, parts in grouped.items()}


async def synth_one(
    lesson_id: int, text: str, sem: asyncio.Semaphore, client, work_dir: Path
) -> dict:
    key = f"{PREFIX}lesson-{lesson_id}.wav"
    for attempt in range(3):
        try:
            async with sem:
                mp3 = work_dir / f"lesson-{lesson_id}.mp3"
                await edge_tts.Communicate(
                    base.tts_input(text), base.VOICE,
                    rate=base.RATE, pitch=base.PITCH,
                ).save(str(mp3))
                wav = await asyncio.to_thread(base.convert_and_trim, mp3, work_dir)
                await asyncio.to_thread(
                    client.put_object, BUCKET, key,
                    io.BytesIO(wav), len(wav), content_type="audio/wav",
                )
            return {
                "lesson_id": lesson_id,
                "object_key": key,
                "sha256": hashlib.sha256(wav).hexdigest(),
                "size": len(wav),
            }
        except Exception as exc:
            print(f"  retry {attempt + 1}/3 lesson {lesson_id}: {exc}",
                  file=sys.stderr, flush=True)
            await asyncio.sleep(0.8 * (attempt + 1))
    raise RuntimeError(f"lesson {lesson_id} failed")


def sync_pg(records: list[dict]) -> None:
    lines = ["BEGIN;"]
    for rec in records:
        lines.append(
            "INSERT INTO audio_objects "
            "(bucket, object_key, kind, visibility, content_sha256, size_bytes, mime_type) "
            f"VALUES ({base.quote(BUCKET)}, {base.quote(rec['object_key'])}, "
            f"'lesson_audio', 'public', {base.quote(rec['sha256'])}, "
            f"{rec['size']}, 'audio/wav') "
            "ON CONFLICT (bucket, object_key) DO UPDATE SET "
            "kind = EXCLUDED.kind, visibility = 'public', "
            "content_sha256 = EXCLUDED.content_sha256, "
            "size_bytes = EXCLUDED.size_bytes, mime_type = EXCLUDED.mime_type, "
            "updated_at = now();"
        )
        lines.append(
            f"UPDATE lessons SET audio_object_id = ao.id FROM audio_objects ao "
            f"WHERE ao.bucket = {base.quote(BUCKET)} "
            f"AND ao.object_key = {base.quote(rec['object_key'])} "
            f"AND lessons.id = {rec['lesson_id']};"
        )
    lines.append("COMMIT;")
    path = Path("/tmp/tts_lessons_sync.sql")
    path.write_text("\n".join(lines) + "\n", encoding="utf-8")
    subprocess.run(
        ["psql", "-w", "-v", "ON_ERROR_STOP=1", "-f", str(path)],
        check=True, capture_output=True, text=True,
    )


def main() -> int:
    parser = argparse.ArgumentParser()
    parser.add_argument("--apply", action="store_true")
    parser.add_argument("--concurrency", type=int, default=4)
    args = parser.parse_args()

    texts = lesson_texts()
    print(f"lessons={len(texts)} apply={args.apply}", flush=True)
    if not args.apply:
        for lesson_id, text in sorted(texts.items())[:3]:
            print(f"  {lesson_id}: {text[:60]}...")
        return 0

    client = base.build_client()
    sem = asyncio.Semaphore(args.concurrency)

    async def go():
        with tempfile.TemporaryDirectory(prefix="tts_lessons_") as tmp:
            work_dir = Path(tmp)
            tasks = [
                asyncio.create_task(synth_one(lid, text, sem, client, work_dir))
                for lid, text in sorted(texts.items())
            ]
            return await asyncio.gather(*tasks, return_exceptions=True)

    results = asyncio.run(go())
    failures = [r for r in results if isinstance(r, Exception)]
    records = [r for r in results if isinstance(r, dict)]
    for exc in failures:
        print(f"  FAILED: {exc}", file=sys.stderr, flush=True)
    print(f"lesson audio: ok={len(records)} failed={len(failures)}", flush=True)

    if records:
        sync_pg(records)
        print("PG sync OK", flush=True)
    return 1 if failures else 0


if __name__ == "__main__":
    raise SystemExit(main())
