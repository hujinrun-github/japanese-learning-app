#!/usr/bin/env python3
"""Re-synthesize lesson audio and realign lesson_sentences start_ms/end_ms.

edge-tts SentenceBoundary events give per-sentence offsets in 100ns units,
measured from the start of the untrimmed audio. The stored WAV has its leading
silence trimmed, so the trimmed lead (ms) is subtracted from every offset.

Requires the same PYTHONPATH / PG* / MINIO_* environment as tts_minio_apply.py.
Default is a preview; pass --apply to write audio, audio_objects and timestamps.
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
import wave
from collections import defaultdict
from pathlib import Path

import edge_tts

sys.path.insert(0, str(Path(__file__).resolve().parent))
import tts_minio_apply as base  # noqa: E402

ROOT = Path(__file__).resolve().parents[1]
PLAN_DIR = ROOT / "data" / "audio" / "tts_plan"
BUCKET = os.environ.get("MINIO_BUCKET_AUDIO", "japanese")


def pg_copy(sql: str) -> list[list[str]]:
    out = subprocess.check_output(
        ["psql", "-w", "-c", f"COPY ({sql}) TO STDOUT WITH CSV"], text=True
    )
    return [r for r in csv.reader(io.StringIO(out)) if r]


def lesson_sentences() -> dict[int, list[str]]:
    rows = pg_copy(
        "SELECT s.lesson_id, s.position, "
        "string_agg(x.elem->>'surface', '' ORDER BY x.ord) "
        "FROM lesson_sentences s, "
        "jsonb_array_elements(s.tokens) WITH ORDINALITY AS x(elem, ord) "
        "GROUP BY s.lesson_id, s.position ORDER BY s.lesson_id, s.position"
    )
    grouped: dict[int, list[str]] = defaultdict(list)
    for lesson_id, _position, sentence in rows:
        grouped[int(lesson_id)].append(sentence)
    return dict(grouped)


def align_ranges(
    sentences: list[str],
    boundaries: list[dict],
    lead_ms: float,
    duration_ms: float,
) -> tuple[list[tuple[int, int]] | None, int]:
    """Map boundary events onto sentences, merging split boundaries.

    The neural voice sometimes emits two SentenceBoundary events for one
    sentence. Boundaries are greedily grouped until their accumulated text
    covers the sentence; leftover boundaries extend the final sentence.
    Returns (ranges, exact_text_mismatch_count).
    """
    normalize = lambda s: "".join(ch for ch in s if not ch.isspace())
    targets = [normalize(s) for s in sentences]
    boundary_texts = [normalize(b.get("text", "")) for b in boundaries]
    cap = round(duration_ms)
    ranges: list[tuple[int, int]] = []
    mismatch = 0
    index = 0

    for target in targets:
        if index >= len(boundaries):
            return None, mismatch
        group: list[dict] = []
        acc_len = 0
        acc_text = ""
        while index < len(boundaries) and (acc_len < len(target) or not group):
            group.append(boundaries[index])
            acc_text += boundary_texts[index]
            acc_len += len(boundary_texts[index])
            index += 1
        if normalize(acc_text) != target:
            mismatch += 1
        first, last = group[0], group[-1]
        start = max(0, round(first["offset"] / 10000 - lead_ms))
        end = round((last["offset"] + last["duration"]) / 10000 - lead_ms)
        end = min(max(end, start), cap)
        ranges.append((start, end))

    if index < len(boundaries):
        last = boundaries[-1]
        end = round((last["offset"] + last["duration"]) / 10000 - lead_ms)
        end = min(max(end, ranges[-1][0]), cap)
        ranges[-1] = (ranges[-1][0], max(ranges[-1][1], end))

    # Boundary durations include the following pause, so adjacent sentences can
    # overlap by a few tens of ms. Clamp each end to the next sentence start.
    for i in range(len(ranges) - 1):
        if ranges[i][1] > ranges[i + 1][0]:
            ranges[i] = (ranges[i][0], max(ranges[i][0], ranges[i + 1][0]))
    return ranges, mismatch


async def one_lesson(
    lesson_id: int,
    sentences: list[str],
    sem: asyncio.Semaphore,
    client,
    work_dir: Path,
) -> dict:
    key = f"lessons/lesson-{lesson_id}.wav"
    text = base.tts_input("".join(sentences))
    audio = bytearray()
    boundaries: list[dict] = []

    for attempt in range(3):
        try:
            async with sem:
                audio.clear()
                boundaries.clear()
                comm = edge_tts.Communicate(
                    text, base.VOICE, rate=base.RATE, pitch=base.PITCH,
                    boundary="SentenceBoundary",
                )
                async for chunk in comm.stream():
                    if chunk["type"] == "audio":
                        audio.extend(chunk["data"])
                    elif chunk["type"] == "SentenceBoundary":
                        boundaries.append(chunk)
                mp3 = work_dir / f"lesson-{lesson_id}.mp3"
                mp3.write_bytes(audio)
                wav, lead_ms = await asyncio.to_thread(
                    base.convert_and_trim_ex, mp3, work_dir
                )
            break
        except Exception as exc:
            if attempt == 2:
                raise RuntimeError(f"lesson {lesson_id}: {exc}") from exc
            await asyncio.sleep(0.8 * (attempt + 1))

    with wave.open(io.BytesIO(wav), "rb") as src:
        duration_ms = src.getnframes() / src.getframerate() * 1000.0

    ranges, mismatch = align_ranges(sentences, boundaries, lead_ms, duration_ms)
    if ranges is None:
        return {
            "lesson_id": lesson_id, "ok": False,
            "reason": f"only {len(boundaries)} boundaries for {len(sentences)} sentences",
        }

    await asyncio.to_thread(
        client.put_object, BUCKET, key,
        io.BytesIO(wav), len(wav), content_type="audio/wav",
    )
    return {
        "lesson_id": lesson_id, "ok": True, "key": key,
        "sha256": hashlib.sha256(wav).hexdigest(), "size": len(wav),
        "duration_ms": round(duration_ms), "lead_ms": round(lead_ms, 1),
        "ranges": ranges, "text_mismatch": mismatch,
    }


def sync_pg(results: list[dict]) -> None:
    lines = ["BEGIN;"]
    for r in results:
        lines.append(
            "UPDATE audio_objects SET "
            f"content_sha256 = {base.quote(r['sha256'])}, size_bytes = {r['size']}, "
            "updated_at = now() "
            f"WHERE bucket = {base.quote(BUCKET)} AND object_key = {base.quote(r['key'])};"
        )
        for position, (start, end) in enumerate(r["ranges"]):
            lines.append(
                "UPDATE lesson_sentences "
                f"SET start_ms = {start}, end_ms = {end} "
                f"WHERE lesson_id = {r['lesson_id']} AND position = {position};"
            )
    lines.append("COMMIT;")
    path = Path("/tmp/tts_lessons_realign.sql")
    path.write_text("\n".join(lines) + "\n", encoding="utf-8")
    subprocess.run(
        ["psql", "-w", "-v", "ON_ERROR_STOP=1", "-f", str(path)],
        check=True, capture_output=True, text=True,
    )


def main() -> int:
    parser = argparse.ArgumentParser()
    parser.add_argument("--apply", action="store_true")
    parser.add_argument("--concurrency", type=int, default=4)
    parser.add_argument("--only", default="", help="comma separated lesson ids")
    args = parser.parse_args()

    sentences = lesson_sentences()
    if args.only:
        wanted = {int(x) for x in args.only.split(",") if x.strip()}
        sentences = {k: v for k, v in sentences.items() if k in wanted}
    print(f"lessons={len(sentences)} apply={args.apply}", flush=True)
    if not args.apply:
        for lid, parts in sorted(sentences.items())[:3]:
            print(f"  {lid}: {len(parts)} 句 / {len(''.join(parts))} 字")
        return 0

    client = base.build_client()
    sem = asyncio.Semaphore(args.concurrency)

    async def go():
        with tempfile.TemporaryDirectory(prefix="tts_realign_") as tmp:
            work_dir = Path(tmp)
            tasks = [
                asyncio.create_task(one_lesson(lid, parts, sem, client, work_dir))
                for lid, parts in sorted(sentences.items())
            ]
            return await asyncio.gather(*tasks, return_exceptions=True)

    results = asyncio.run(go())
    failures = [r for r in results if isinstance(r, Exception) or not r.get("ok")]
    ok = [r for r in results if isinstance(r, dict) and r.get("ok")]
    for f in failures:
        print(f"  FAILED: {f}", file=sys.stderr, flush=True)
    print(f"realign: ok={len(ok)} failed={len(failures)}", flush=True)
    for r in sorted(ok, key=lambda x: x["lesson_id"]):
        print(f"  lesson {r['lesson_id']:2d}: {r['duration_ms']}ms, "
              f"lead={r['lead_ms']}ms, text_mismatch={r['text_mismatch']}, "
              f"first={r['ranges'][0]} last={r['ranges'][-1]}", flush=True)

    if ok:
        sync_pg(ok)
        print("PG sync OK", flush=True)
        with (PLAN_DIR / "lessons_realign.csv").open("w", newline="", encoding="utf-8") as f:
            writer = csv.writer(f)
            writer.writerow(["lesson_id", "object_key", "duration_ms",
                             "start_ms", "end_ms", "text_mismatch"])
            for r in sorted(ok, key=lambda x: x["lesson_id"]):
                for start, end in r["ranges"]:
                    writer.writerow([r["lesson_id"], r["key"],
                                     r["duration_ms"], start, end, r["text_mismatch"]])
    return 1 if failures else 0


if __name__ == "__main__":
    raise SystemExit(main())
