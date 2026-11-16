#!/usr/bin/env python3
"""Backfill lesson paragraph markers from seed files into PostgreSQL.

Only lesson_sentences.paragraph is updated. This deliberately does NOT
re-import the seed lessons: import-lessons-postgres deletes and reinserts every
lesson_sentences row, which would overwrite the start_ms/end_ms values that were
recomputed from TTS sentence boundaries.

Connection comes from standard PG* environment variables, e.g.:
  PGHOST=... PGPORT=... PGUSER=... PGDATABASE=... PGPASSWORD=... \
    python3 scripts/backfill_lesson_paragraphs.py

Modes:
  (default)      dry-run: read-only checks, print the per-lesson paragraph plan
  --apply        run the UPDATE, then verify timestamps are unchanged
  --print-sql    offline: print the generated UPDATE SQL without connecting

Prerequisite for --apply: migration 010_lesson_paragraph.sql must have been
applied (service startup runs embedded migrations automatically).
"""

from __future__ import annotations

import argparse
import json
import subprocess
import sys
from collections import Counter, defaultdict
from pathlib import Path

ROOT = Path(__file__).resolve().parent.parent
SEEDS = [
    "data/seed/lessons_n5.json",
    "data/seed/lessons_n4.json",
    "data/seed/lessons_n3.json",
    "data/seed/lessons_shadowing_pilot.json",
]
SEP = "\x1f"


def read_plan() -> dict[tuple[str, str, int], int]:
    plan: dict[tuple[str, str, int], int] = {}
    for rel in SEEDS:
        raw = json.loads((ROOT / rel).read_text(encoding="utf-8"))
        lessons = raw if isinstance(raw, list) else raw["lessons"]
        for lesson in lessons:
            for sentence in lesson["sentences"]:
                key = (lesson["title"], lesson["jlpt_level"], int(sentence["index"]))
                value = int(sentence["paragraph"])
                if key in plan and plan[key] != value:
                    sys.exit(f"abort: conflicting paragraph for {key}: {plan[key]} vs {value}")
                plan[key] = value
    return plan


def build_update_sql(plan: dict[tuple[str, str, int], int]) -> str:
    rows = []
    for (title, level, pos), paragraph in sorted(plan.items()):
        rows.append(f"  ({sql_literal(title)}, {sql_literal(level)}, {pos}, {paragraph})")
    values = ",\n".join(rows)
    return (
        "WITH plan(title, jlpt_level, pos, paragraph) AS (\n"
        "  VALUES\n" + values + "\n)\n"
        "UPDATE lesson_sentences AS s\n"
        "SET paragraph = p.paragraph\n"
        "FROM plan AS p\n"
        "JOIN lessons AS l ON l.title = p.title AND l.jlpt_level = p.jlpt_level\n"
        "WHERE s.lesson_id = l.id AND s.position = p.pos\n"
        "  AND s.paragraph IS DISTINCT FROM p.paragraph"
    )


def sql_literal(value: str) -> str:
    return "'" + value.replace("'", "''") + "'"


def psql(sql: str) -> list[list[str]]:
    cmd = ["psql", "-w", "-v", "ON_ERROR_STOP=1", "-At", "-F", SEP, "-c", sql]
    result = subprocess.run(cmd, capture_output=True, text=True)
    if result.returncode != 0:
        sys.exit(f"abort: psql failed: {result.stderr.strip()}")
    return [line.split(SEP) for line in result.stdout.strip().splitlines() if line]


def read_timestamps() -> dict[tuple[str, str, int], tuple[str, str]]:
    rows = psql(
        "SELECT l.title, l.jlpt_level, s.position, s.start_ms, s.end_ms "
        "FROM lesson_sentences s JOIN lessons l ON l.id = s.lesson_id "
        "ORDER BY l.title, l.jlpt_level, s.position"
    )
    return {(r[0], r[1], int(r[2])): (r[3], r[4]) for r in rows}


def has_paragraph_column() -> bool:
    rows = psql(
        "SELECT 1 FROM information_schema.columns "
        "WHERE table_schema = 'public' AND table_name = 'lesson_sentences' "
        "AND column_name = 'paragraph'"
    )
    return len(rows) > 0


def main() -> None:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--apply", action="store_true", help="write paragraph values to PostgreSQL")
    parser.add_argument("--print-sql", action="store_true", help="print SQL and exit without connecting")
    args = parser.parse_args()

    plan = read_plan()
    sql = build_update_sql(plan)
    if args.print_sql:
        print(sql)
        return

    by_lesson: dict[tuple[str, str], Counter] = defaultdict(Counter)
    for (title, level, _), paragraph in plan.items():
        by_lesson[(title, level)][paragraph] += 1
    print(f"seed plan: {len(plan)} sentences / {len(by_lesson)} lessons")
    for (title, level), counts in sorted(by_lesson.items()):
        shape = ", ".join(f"p{p}={n}" for p, n in sorted(counts.items()))
        print(f"  {level} {title}: {shape}")

    before = read_timestamps()
    missing = sorted(set(plan) - set(before))
    if missing:
        sys.exit(f"abort: {len(missing)} seed sentences have no PostgreSQL row, e.g. {missing[:3]}")
    extra = sorted(set(before) - set(plan))
    if extra:
        print(f"note: {len(extra)} database sentences are not in the seed plan and will be left untouched")

    if not args.apply:
        print(f"dry-run only: would update paragraph for at most {len(plan)} rows")
        print("timestamps are never touched; run with --apply to write")
        return

    if not has_paragraph_column():
        sys.exit("abort: lesson_sentences.paragraph is missing; apply migration 010_lesson_paragraph.sql first")

    psql(sql)
    after = read_timestamps()
    changed = [key for key in before if before[key] != after.get(key)]
    if changed:
        sys.exit(f"abort: timestamps changed for {len(changed)} sentences, e.g. {changed[:3]}")
    if set(before) != set(after):
        sys.exit("abort: lesson_sentences row set changed during the update")

    rows = psql(
        "SELECT l.title, l.jlpt_level, s.position, s.paragraph "
        "FROM lesson_sentences s JOIN lessons l ON l.id = s.lesson_id"
    )
    actual = {(r[0], r[1], int(r[2])): int(r[3]) for r in rows}
    mismatch = [key for key, want in plan.items() if actual.get(key) != want]
    if mismatch:
        sys.exit(f"abort: {len(mismatch)} paragraph values did not persist, e.g. {mismatch[:3]}")
    print(f"apply ok: {len(plan)} paragraph values verified; {len(before)} timestamps unchanged")


if __name__ == "__main__":
    main()
