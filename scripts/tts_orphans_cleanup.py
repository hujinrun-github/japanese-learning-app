#!/usr/bin/env python3
"""Delete orphan audio objects (no current text, no FK references).

Reads data/audio/tts_plan/orphan_objects.csv. Refuses to run if any business
table still references one of the objects. Deletes PostgreSQL rows first, then
removes the MinIO objects; the CSV remains as the audit/rollback record.

Default is a preview; pass --apply to delete.
"""

from __future__ import annotations

import argparse
import csv
import io
import os
import subprocess
import sys
from pathlib import Path

from minio import Minio
from minio.deleteobjects import DeleteObject

ROOT = Path(__file__).resolve().parents[1]
PLAN_DIR = ROOT / "data" / "audio" / "tts_plan"
BUCKET = os.environ.get("MINIO_BUCKET_AUDIO", "japanese")
TABLES = ("words", "word_examples", "grammar_examples", "speaking_materials", "lessons")


def pg_copy(sql: str) -> list[list[str]]:
    out = subprocess.check_output(
        ["psql", "-w", "-c", f"COPY ({sql}) TO STDOUT WITH CSV"], text=True
    )
    return [r for r in csv.reader(io.StringIO(out)) if r]


def build_client() -> Minio:
    endpoint = os.environ["MINIO_ENDPOINT"].replace("http://", "").replace("https://", "")
    return Minio(
        endpoint,
        access_key=os.environ["MINIO_ACCESS_KEY"],
        secret_key=os.environ["MINIO_SECRET_KEY"],
        secure=False,
    )


def main() -> int:
    parser = argparse.ArgumentParser()
    parser.add_argument("--apply", action="store_true")
    args = parser.parse_args()

    orphans = list(csv.DictReader((PLAN_DIR / "orphan_objects.csv").open(encoding="utf-8")))
    ids = [int(r["id"]) for r in orphans]
    keys = [r["object_key"] for r in orphans]
    print(f"orphans={len(orphans)} apply={args.apply}", flush=True)

    # Safety gate: abort if any FK still points at an orphan.
    id_set = set(ids)
    for table in TABLES:
        rows = pg_copy(f"SELECT audio_object_id FROM {table} WHERE audio_object_id IS NOT NULL")
        hit = sum(1 for r in rows if int(r[0]) in id_set)
        if hit:
            sys.exit(f"ABORT: {table} still references {hit} orphan objects")
    print("FK 引用检查: 0，可安全删除", flush=True)

    if not args.apply:
        print("preview only, pass --apply to delete")
        return 0

    # PostgreSQL rows first, so a MinIO failure leaves no dangling records.
    id_list = ",".join(str(i) for i in ids)
    sql_path = Path("/tmp/tts_orphans_delete.sql")
    sql_path.write_text(
        f"BEGIN;\nDELETE FROM audio_objects WHERE id IN ({id_list});\nCOMMIT;\n",
        encoding="utf-8",
    )
    subprocess.run(
        ["psql", "-w", "-v", "ON_ERROR_STOP=1", "-f", str(sql_path)],
        check=True, capture_output=True, text=True,
    )
    remaining = pg_copy(
        "SELECT count(*) FROM audio_objects "
        f"WHERE bucket = '{BUCKET}' AND deleted_at IS NULL"
    )
    print(f"PG 删除完成，剩余 audio_objects: {remaining[0][0] if remaining else '?'}", flush=True)

    client = build_client()
    errors = list(client.remove_objects(BUCKET, (DeleteObject(k) for k in keys)))
    for err in errors[:10]:
        print(f"  MinIO delete error: {err}", file=sys.stderr, flush=True)
    print(f"MinIO 删除: {len(keys) - len(errors)} 成功, {len(errors)} 失败", flush=True)

    live = [o.object_name for o in client.list_objects(BUCKET, recursive=True)]
    print(f"MinIO 剩余对象: {len(live)}", flush=True)
    return 1 if errors else 0


if __name__ == "__main__":
    raise SystemExit(main())
