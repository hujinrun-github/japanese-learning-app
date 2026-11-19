"""Prepare and validate the user-supplied Standard Japanese speaking materials.

No PDF contents or credentials are embedded here. The proofread material pack and
generated audio live in the ignored data/ directory.
"""
from __future__ import annotations

import hashlib
import re
import unicodedata


def normalize_lines(lines: list[str]) -> list[str]:
    result: list[str] = []
    for raw in lines:
        line = unicodedata.normalize("NFKC", raw).strip().replace(":", "：")
        line = re.sub(r"(?<!\d),|,(?!\d)", "、", line)
        line = line.replace("?", "？").replace("!", "！")
        line = re.sub(r"\.{2,}", "…", line)
        line = re.sub(r"^[A-D1]+(?=(?:甲|乙|李)\s*：)", "", line)
        if not line:
            continue
        if result and not re.match(r"^(?:[1-4]\.\s*|[^：\s]{1,8}：)", line):
            result[-1] += line
        else:
            result.append(line)
    return result


def spoken_text(text: str) -> str:
    lines = []
    for line in text.splitlines():
        line = re.sub(r"^\s*[1-4][.．]\s*", "", line)
        line = re.sub(r"^[^：:\n]{1,8}[：:]\s*", "", line)
        if line.strip():
            lines.append(line.strip())
    return "\n".join(lines)


def canonical_text(text: str) -> str:
    text = spoken_text(unicodedata.normalize("NFKC", text))
    return "".join(c for c in text if c.isalnum())


def audio_filename(text: str) -> str:
    return hashlib.sha256(text.encode("utf-8")).hexdigest()[:16] + ".wav"


def plan_import(items: list[dict], existing: list[dict]) -> tuple[list[dict], list[dict]]:
    seen = {canonical_text(x["text"]): x for x in existing}
    titles = {x["title"]: canonical_text(x["text"]) for x in existing}
    pending, skipped = [], []
    for item in items:
        canonical = canonical_text(item["text"])
        if item["title"] in titles and titles[item["title"]] != canonical:
            raise ValueError(f"existing title has different text: {item['title']}")
        if canonical in seen:
            skipped.append({"title": item["title"], "existing_id": seen[canonical].get("id"),
                            "duplicate_of": seen[canonical]["title"]})
            continue
        pending.append(item)
        seen[canonical] = item
        titles[item["title"]] = canonical
    return pending, skipped


def validate_materials(items: list[dict]) -> None:
    if len(items) != 96:
        raise ValueError(f"expected 96 materials, got {len(items)}")
    expected = {(lesson, kind) for lesson in range(1, 49) for kind in ("basic", "application")}
    actual = {(x["lesson"], x["kind"]) for x in items}
    if actual != expected:
        raise ValueError(f"missing or duplicate lesson sections: {expected - actual}")
    titles = set()
    for item in items:
        if item["title"] in titles:
            raise ValueError(f"duplicate title: {item['title']}")
        titles.add(item["title"])
        if not item.get("reviewed") or not item.get("text", "").strip():
            raise ValueError(f"unreviewed or empty text: {item['title']}")
        if item["jlpt_level"] != ("N5" if item["lesson"] <= 24 else "N4"):
            raise ValueError(f"incorrect level: {item['title']}")
        if item["kind"] == "basic":
            lines = item["text"].splitlines()
            if len(lines) < 12 or any(not lines[i].startswith(f"{i + 1}.") for i in range(4)):
                raise ValueError(f"incomplete basic text: {item['title']}")
        if not item.get("source", {}).get("pdf_page"):
            raise ValueError(f"missing PDF provenance: {item['title']}")
