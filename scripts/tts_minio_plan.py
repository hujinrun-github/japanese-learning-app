#!/usr/bin/env python3
"""Dry-run planner for regenerating Japanese TTS audio into the MinIO audio bucket.

Read-only against PostgreSQL. Writes only a local CSV manifest and a Markdown
report. No MinIO upload and no database mutation happens in this script.

Connection is taken from standard PG* environment variables, e.g.:
  PGHOST=... PGPORT=... PGUSER=... PGDATABASE=... PGPASSWORD=... python3 scripts/tts_minio_plan.py

Object-key rule (verified against examples/001ba7ef732eea56.wav):
  object_key = <prefix> + sha256(text)[:16] + ".wav"
  prefix: words/ for word readings, examples/ for examples and speaking text.
"""

from __future__ import annotations

import argparse
import csv
import hashlib
import io
import os
import subprocess
from collections import defaultdict
from pathlib import Path

BUCKET = "japanese"  # MINIO_BUCKET_AUDIO

SOURCES = [
    # (label, table, text column, object prefix)
    ("words.reading", "words", "reading", "words/"),
    ("word_examples.japanese", "word_examples", "japanese", "examples/"),
    ("grammar_examples.japanese", "grammar_examples", "japanese", "examples/"),
    ("speaking_materials.text", "speaking_materials", "text", "examples/"),
]


def psql_copy(sql: str) -> list[list[str]]:
    """Run COPY (...) TO STDOUT WITH CSV and return parsed rows."""
    cmd = ["psql", "-w", "-c", f"COPY ({sql}) TO STDOUT WITH CSV"]
    result = subprocess.run(
        cmd, check=True, capture_output=True, text=True, env=os.environ
    )
    return [row for row in csv.reader(io.StringIO(result.stdout)) if row]


def sha16(text: str) -> str:
    return hashlib.sha256(text.encode("utf-8")).hexdigest()[:16]


def collect_texts() -> dict[tuple[str, str], dict]:
    """Return {(prefix, text): {sources, rows}} with cross-table dedup."""
    targets: dict[tuple[str, str], dict] = {}
    for label, table, column, prefix in SOURCES:
        rows = psql_copy(
            f"SELECT {column}, count(*) FROM {table} "
            f"WHERE {column} <> '' GROUP BY {column}"
        )
        for text, count in rows:
            key = (prefix, text)
            entry = targets.setdefault(key, {"sources": set(), "rows": 0})
            entry["sources"].add(label)
            entry["rows"] += int(count)
    return targets


def collect_lessons() -> dict[int, str]:
    """Concatenate lesson_sentences token surfaces into one text per lesson."""
    rows = psql_copy(
        "SELECT s.lesson_id, s.position, "
        "string_agg(x.elem->>'surface', '' ORDER BY x.ord) "
        "FROM lesson_sentences s, "
        "jsonb_array_elements(s.tokens) WITH ORDINALITY AS x(elem, ord) "
        "GROUP BY s.lesson_id, s.position ORDER BY s.lesson_id, s.position"
    )
    lessons: dict[int, list[str]] = defaultdict(list)
    for lesson_id, _position, sentence in rows:
        lessons[int(lesson_id)].append(sentence)
    return {lid: "".join(parts) for lid, parts in lessons.items()}


def main() -> int:
    parser = argparse.ArgumentParser()
    parser.add_argument("--out-dir", default="data/audio/tts_plan")
    parser.add_argument(
        "--minio-keys",
        default="",
        help="optional local file with one MinIO object key per line",
    )
    parser.add_argument(
        "--seconds-per-item", type=float, default=2.5,
        help="estimated wall time per synthesized+converted+uploaded item",
    )
    args = parser.parse_args()

    out_dir = Path(args.out_dir)
    out_dir.mkdir(parents=True, exist_ok=True)

    ao_keys = {
        row[0]
        for row in psql_copy("SELECT object_key FROM audio_objects WHERE deleted_at IS NULL")
    }
    prefix_counts: dict[str, int] = defaultdict(int)
    for key in ao_keys:
        prefix_counts[key.split("/")[0] if "/" in key else "<根目录>"] += 1
    kind_rows = psql_copy(
        "SELECT kind, count(*) FROM audio_objects "
        "WHERE deleted_at IS NULL GROUP BY kind ORDER BY 2 DESC"
    )
    minio_keys: set[str] = set()
    if args.minio_keys and Path(args.minio_keys).exists():
        minio_keys = set(Path(args.minio_keys).read_text().splitlines())

    targets = collect_texts()
    lessons = collect_lessons()

    manifest: list[dict] = []
    for (prefix, text), info in targets.items():
        object_key = prefix + sha16(text) + ".wav"
        in_ao = object_key in ao_keys
        in_minio = object_key in minio_keys if minio_keys else ""
        manifest.append({
            "object_key": object_key,
            "bucket": BUCKET,
            "prefix": prefix,
            "source_tables": "|".join(sorted(info["sources"])),
            "rows": info["rows"],
            "text": text,
            "status": "EXISTS" if in_ao else "MISSING",
            "in_audio_objects": in_ao,
            "in_minio": in_minio,
        })
    manifest.sort(key=lambda r: (r["prefix"], r["status"], r["object_key"]))

    csv_path = out_dir / "dry_run_manifest.csv"
    with csv_path.open("w", newline="", encoding="utf-8") as f:
        writer = csv.DictWriter(f, fieldnames=list(manifest[0].keys()))
        writer.writeheader()
        writer.writerows(manifest)

    by_prefix: dict[str, dict[str, int]] = defaultdict(lambda: defaultdict(int))
    for row in manifest:
        by_prefix[row["prefix"]][row["status"]] += 1
        by_prefix[row["prefix"]]["TOTAL"] += 1

    total_missing = sum(1 for r in manifest if r["status"] == "MISSING")
    total_all = len(manifest)
    orphan_keys = ao_keys - {r["object_key"] for r in manifest}

    lines = [
        "# TTS 全量生成 · Dry-run 清单",
        "",
        f"- MinIO bucket: `{BUCKET}`（由 `MINIO_BUCKET_AUDIO` 指定）",
        "- 对象键: `<prefix>sha256(原始文本)[:16].wav`；哈希规则已反查验证"
        "（`びょういん` → `001ba7ef732eea56.wav`，原在 `examples/`，现已规范化到 `words/`）",
        "- 应用侧 `GET /audio/{key...}` 为通配路由，按 `audio_objects.object_key` 查，无前缀白名单",
        "- **本脚本为只读 dry-run**：清单生成不上传对象、不修改 PostgreSQL。"
        "历史对象搬移是独立步骤，明细见 `words_move_manifest.csv`",
        "",
        "## 目标对象统计（跨表去重后）",
        "",
        "| 前缀 | 目标对象 | 已存在 | 待生成 |",
        "|---|---:|---:|---:|",
    ]
    for prefix in ("examples/", "words/"):
        s = by_prefix[prefix]
        lines.append(
            f"| `{prefix}` | {s['TOTAL']} | {s['EXISTS']} | {s['MISSING']} |"
        )
    lines += [
        f"| **合计** | **{total_all}** | **{total_all - total_missing}** | **{total_missing}** |",
        "",
        "## 数据来源",
        "",
        "| 来源 | 去重文本数 | 落点 |",
        "|---|---:|---|",
    ]
    for label, table, column, prefix in SOURCES:
        rows = psql_copy(
            f"SELECT count(DISTINCT {column}) FROM {table} WHERE {column} <> ''"
        )
        lines.append(f"| `{label}` | {rows[0][0]} | `{prefix}` |")
    lines += [
        "",
        "## 现状与风险",
        "",
        "- 现有对象前缀分布：" + "、".join(
            f"`{p}/` {n}" if not p.startswith("<") else f"{p} {n}"
            for p, n in sorted(prefix_counts.items())
        ) + f"（共 {len(ao_keys)} 个）。单词读音与例句已按 `words/` / `examples/` 分开。",
        f"- 现有对象中有 {len(orphan_keys)} 个与当前 PG 文本对不上（历史遗留孤儿对象），不会删除。",
        "- `audio_objects.kind` 分布：" + "、".join(
            f"`{kind}`({count})" for kind, count in kind_rows
        ) + "；新生成的 `words/` 对象应使用 `word_pronunciation`。",
        "- 已存在的对象保持旧音质（含带呼吸声/静音的历史 Qwen3-TTS 音频）。"
        "若目标是「所有文本都换成人感语音」，需要选择全量覆盖，而不是只补缺失。",
        f"- 课文/跟读：{len(lessons)} 篇课文、"
        f"{sum(1 for _ in psql_copy('SELECT id FROM lesson_sentences'))} 句；"
        "桶里只有 `lessons/shadowing-pilot-n5-day.wav` 一个课文音频。"
        "`lesson_sentences` 带 `start_ms`/`end_ms` 跟读时间戳，整篇重新合成会让时间戳失准，"
        "本清单**未把课文计入待生成**，等策略确认。",
        "",
        "## 两种执行口径",
        "",
        "| 口径 | 需合成条数 | 说明 |",
        "|---|---:|---|",
        f"| 只补缺失（dry-run 默认） | {total_missing} | 不覆盖任何已有对象；"
        "但已存在的仍是旧音质 |",
        f"| 全量重生成（覆盖） | {total_all} | 所有文本都换成新人感语音，"
        "同名对象被覆盖，不可逆 |",
        "",
        f"按 {args.seconds_per_item:.1f} 秒/条估算：只补缺失约 "
        f"{total_missing * args.seconds_per_item / 60:.0f} 分钟，全量约 "
        f"{total_all * args.seconds_per_item / 60:.0f} 分钟（含在线 TTS 请求、转码、上传）。",
        "",
        "## 明细",
        "",
        f"见 `dry_run_manifest.csv`（{total_all} 行，含每条文本、来源表、对象键、状态）。",
        "",
    ]
    report_path = out_dir / "dry_run_report.md"
    report_path.write_text("\n".join(lines), encoding="utf-8")

    print(f"bucket={BUCKET} objects={total_all} exists={total_all - total_missing} missing={total_missing}")
    print(f"orphan_existing_objects={len(orphan_keys)} lessons={len(lessons)}")
    print(f"report={report_path}")
    print(f"manifest={csv_path}")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
