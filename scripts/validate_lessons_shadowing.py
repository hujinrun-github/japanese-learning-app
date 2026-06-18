#!/usr/bin/env python3
"""Validate manual shadowing lesson material packs."""

from __future__ import annotations

import argparse
import json
import sys
from pathlib import Path
from typing import Any


REQUIRED_SOURCE_FIELDS = (
    "source_type",
    "source_name",
    "license",
    "permission_note",
    "subtitle_source",
)


def main() -> int:
    parser = argparse.ArgumentParser(description="Validate shadowing lesson JSON.")
    parser.add_argument("--file", required=True, help="Path to a JSON array of lessons.")
    args = parser.parse_args()

    path = Path(args.file)
    try:
        lessons = json.loads(path.read_text(encoding="utf-8"))
    except OSError as exc:
        print(f"ERROR: {path}: read failed: {exc}")
        return 1
    except json.JSONDecodeError as exc:
        print(f"ERROR: {path}: invalid JSON: {exc}")
        return 1

    if not isinstance(lessons, list):
        print(f"ERROR: {path}: root must be a JSON array")
        return 1

    errors: list[tuple[str, str]] = []
    for lesson in lessons:
        if not isinstance(lesson, dict):
            errors.append(("<unknown>", "lesson item must be an object"))
            continue
        if lesson.get("shadowing_enabled") is True:
            errors.extend(validate_lesson(lesson))

    for title, message in errors:
        print(f"ERROR: {title}: {message}")
    return 1 if errors else 0


def validate_lesson(lesson: dict[str, Any]) -> list[tuple[str, str]]:
    title = lesson_title(lesson)
    errors: list[tuple[str, str]] = []

    if not non_empty_string(lesson.get("audio_url")):
        errors.append((title, "audio_url is required"))

    version = lesson.get("shadowing_version")
    if not is_int(version) or version < 1:
        errors.append((title, "shadowing_version must be >= 1"))

    config = lesson.get("shadowing_config")
    if not isinstance(config, dict):
        errors.append((title, "shadowing_config must be an object"))
        config = {}

    media_duration_ms = config.get("media_duration_ms")
    if not is_positive_number(media_duration_ms):
        errors.append((title, "shadowing_config.media_duration_ms must be > 0"))
        media_duration_ms = None

    source = config.get("source")
    if not isinstance(source, dict):
        errors.append((title, "shadowing_config.source must be an object"))
        source = {}
    for field in REQUIRED_SOURCE_FIELDS:
        if not non_empty_string(source.get(field)):
            errors.append((title, f"shadowing_config.source.{field} is required"))

    sentences = lesson.get("sentences")
    if not isinstance(sentences, list) or len(sentences) == 0:
        errors.append((title, "sentences must be a non-empty array"))
        return errors

    previous_end: float | None = None
    last_end: float | None = None
    for index, sentence in enumerate(sentences):
        if not isinstance(sentence, dict):
            errors.append((title, f"sentences[{index}] must be an object"))
            continue

        tokens = sentence.get("tokens")
        if not isinstance(tokens, list) or len(tokens) == 0:
            errors.append((title, f"sentences[{index}].tokens is required"))
        if not non_empty_string(sentence.get("chinese")):
            errors.append((title, f"sentences[{index}].chinese is required"))

        start_ms = sentence.get("start_ms")
        end_ms = sentence.get("end_ms")
        if not is_number(start_ms):
            errors.append((title, f"sentences[{index}].start_ms is required"))
            start_ms = None
        if not is_number(end_ms):
            errors.append((title, f"sentences[{index}].end_ms is required"))
            end_ms = None
        if start_ms is None or end_ms is None:
            continue

        if not (0 <= start_ms < end_ms):
            errors.append((title, f"sentences[{index}] must satisfy 0 <= start_ms < end_ms"))
        if previous_end is not None and start_ms < previous_end:
            errors.append((title, f"sentences[{index}] overlaps or is out of order"))
        previous_end = end_ms
        last_end = end_ms

    if (
        media_duration_ms is not None
        and last_end is not None
        and last_end > media_duration_ms + 1000
    ):
        errors.append((title, "last sentence end_ms exceeds media_duration_ms + 1000"))

    return errors


def lesson_title(lesson: dict[str, Any]) -> str:
    title = lesson.get("title")
    if isinstance(title, str) and title:
        return title
    return "<untitled>"


def non_empty_string(value: Any) -> bool:
    return isinstance(value, str) and value.strip() != ""


def is_positive_number(value: Any) -> bool:
    return is_number(value) and value > 0


def is_number(value: Any) -> bool:
    return isinstance(value, (int, float)) and not isinstance(value, bool)


def is_int(value: Any) -> bool:
    return isinstance(value, int) and not isinstance(value, bool)


if __name__ == "__main__":
    sys.exit(main())
