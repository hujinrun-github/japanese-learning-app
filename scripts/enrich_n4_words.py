#!/usr/bin/env python3
"""
Enrich N4 words with Chinese meanings and example sentences using LLM API.

Reads N4 words from the database that are missing Chinese meanings or examples,
sends them in batches to the API, and updates the database with the results.
"""

import json
import os
import sqlite3
import sys
import time
import re
from pathlib import Path

import requests

# Configuration
API_URL = f"{os.environ.get('ANTHROPIC_BASE_URL', 'https://api.anthropic.com')}/messages"
API_TOKEN = os.environ.get('ANTHROPIC_AUTH_TOKEN', '')
MODEL = os.environ.get('ANTHROPIC_MODEL', 'deepseek-v4-pro')

DB_PATH = Path(__file__).resolve().parent.parent / 'data' / 'app.db'
BATCH_SIZE = 10
SLEEP_BETWEEN_BATCHES = 3
MAX_RETRIES = 3

PROMPT_TEMPLATE = """You are a Japanese-Chinese dictionary editor. For each Japanese word below, provide:
1. A Chinese meaning (自然的中文释义，参考给出的英文定义但不要直接翻译英文)
2. Part of speech in Chinese (if empty)
3. Two example sentences: Japanese sentence + Chinese translation

Return ONLY a valid JSON array, no other text.

Words:
{words_json}

Return format:
[
  {{
    "kanji_form": "...",
    "reading": "...",
    "meaning_zh": "...",
    "part_of_speech_zh": "名词/动词/形容词/副词/...",
    "examples": [
      {{"japanese": "...", "chinese": "..."}},
      {{"japanese": "...", "chinese": "..."}}
    ]
  }},
  ...
]

Important rules:
- meaning_zh: Chinese meaning focused on how this word is actually used in context
- part_of_speech_zh: use 名词/动词/形容词/副词/感叹词/连词/助词/接头词/接尾词/短语/其他
- Each example: a natural sentence SHOWING the word in context (not just the word itself)
- Ensure examples are appropriate for N4 (beginner-intermediate) level
- Do NOT use overly complex kanji in example sentences"""


def get_words_to_enrich(db_path: str) -> list[dict]:
    """Get N4 words that need enrichment."""
    conn = sqlite3.connect(db_path)
    conn.row_factory = sqlite3.Row
    rows = conn.execute("""
        SELECT id, kanji_form, reading, meaning, part_of_speech, examples_json
        FROM words
        WHERE jlpt_level = 'N4'
        AND (examples_json IS NULL OR examples_json = 'null' OR examples_json = '[]')
    """).fetchall()
    conn.close()

    words = []
    for r in rows:
        # Check if meaning looks like English (not Chinese)
        meaning = r['meaning'] or ''
        has_chars = any('一' <= c <= '鿿' or '㐀' <= c <= '䶿' for c in meaning)
        words.append({
            'id': r['id'],
            'kanji_form': r['kanji_form'],
            'reading': r['reading'],
            'meaning': meaning,
            'part_of_speech': r['part_of_speech'] or '',
            'needs_meaning': not has_chars or len(meaning) < 2,
            'examples_json': r['examples_json'],
        })
    return words


def build_batch(words_batch: list[dict]) -> str:
    """Build the JSON string for a batch of words to send to the API."""
    compact = []
    for w in words_batch:
        compact.append({
            'kanji_form': w['kanji_form'],
            'reading': w['reading'],
            'meaning_en': w['meaning'],
            'part_of_speech': w['part_of_speech'],
        })
    return json.dumps(compact, ensure_ascii=False, indent=2)


def call_api(prompt: str) -> str | None:
    """Call the LLM API and return the response text."""
    headers = {
        'Authorization': f'Bearer {API_TOKEN}',
        'Content-Type': 'application/json',
        'anthropic-version': '2023-06-01',
    }

    payload = {
        'model': MODEL,
        'max_tokens': 8192,
        'messages': [
            {'role': 'user', 'content': prompt}
        ],
    }

    for attempt in range(MAX_RETRIES):
        try:
            resp = requests.post(API_URL, headers=headers, json=payload, timeout=120)
            if resp.status_code == 200:
                data = resp.json()
                content = data.get('content', [{}])
                if content and isinstance(content, list):
                    text = ''.join(block.get('text', '') for block in content if block.get('type') == 'text')
                    return text
                return content[0].get('text', '') if content else ''
            elif resp.status_code == 429:
                wait = int(resp.headers.get('retry-after', '30'))
                print(f"  Rate limited, waiting {wait}s...")
                time.sleep(wait)
            else:
                print(f"  API error ({resp.status_code}): {resp.text[:200]}")
                time.sleep(5 * (attempt + 1))
        except Exception as e:
            print(f"  Request failed: {e}")
            time.sleep(5 * (attempt + 1))
    return None


def parse_response(text: str, batch_words: list[dict]) -> list[dict]:
    """Parse API response JSON, matching back to database IDs."""
    # Try to extract JSON array from response
    json_match = re.search(r'\[.*\]', text, re.DOTALL)
    if not json_match:
        print(f"  No JSON array found in response: {text[:300]}")
        return []

    try:
        parsed = json.loads(json_match.group())
    except json.JSONDecodeError:
        print(f"  Failed to parse JSON: {json_match.group()[:300]}")
        return []

    # Build lookup by kanji_form + reading
    lookup = {(w['kanji_form'], w['reading']): w for w in batch_words}

    results = []
    for item in parsed:
        key = (item.get('kanji_form', ''), item.get('reading', ''))
        word = lookup.get(key)
        if not word:
            # Try reading-only match
            for k, v in lookup.items():
                if k[1] == key[1]:
                    word = v
                    break
        if word:
            results.append({
                'id': word['id'],
                'meaning_zh': item.get('meaning_zh', ''),
                'part_of_speech_zh': item.get('part_of_speech_zh', ''),
                'examples': item.get('examples', []),
            })
        else:
            print(f"  Warning: couldn't match response item {key}")

    return results


def update_database(db_path: str, results: list[dict]) -> int:
    """Update the database with enriched data. Returns count of updated words."""
    conn = sqlite3.connect(db_path)
    updated = 0
    for r in results:
        examples = r.get('examples', [])
        examples_json = json.dumps(examples, ensure_ascii=False) if examples else '[]'
        meaning = r.get('meaning_zh', '')
        pos = r.get('part_of_speech_zh', '')

        conn.execute("""
            UPDATE words
            SET meaning = CASE WHEN ? != '' THEN ? ELSE meaning END,
                part_of_speech = CASE WHEN ? != '' THEN ? ELSE part_of_speech END,
                examples_json = ?
            WHERE id = ?
        """, (meaning, meaning, pos, pos, examples_json, r['id']))
        updated += 1
    conn.commit()
    conn.close()
    return updated


def main():
    if not API_TOKEN:
        print("Error: ANTHROPIC_AUTH_TOKEN not set")
        sys.exit(1)

    print(f"Using API: {API_URL}")
    print(f"Model: {MODEL}")

    db_path = str(DB_PATH)
    if not os.path.exists(db_path):
        print(f"Error: database not found at {db_path}")
        sys.exit(1)

    words = get_words_to_enrich(db_path)
    print(f"Words to enrich: {len(words)}")

    if not words:
        print("Nothing to do!")
        return

    total_updated = 0
    batches = [words[i:i + BATCH_SIZE] for i in range(0, len(words), BATCH_SIZE)]
    print(f"Processing in {len(batches)} batches of ~{BATCH_SIZE}")

    for i, batch in enumerate(batches):
        print(f"\nBatch {i + 1}/{len(batches)} ({len(batch)} words): ", end='', flush=True)
        batch_str = build_batch(batch)
        prompt = PROMPT_TEMPLATE.format(words_json=batch_str)

        response = call_api(prompt)
        if not response:
            print("SKIPPED (API error)")
            continue

        results = parse_response(response, batch)
        if not results:
            print("SKIPPED (parse error)")
            continue

        updated = update_database(db_path, results)
        total_updated += updated
        print(f"OK ({updated} updated)")

        if i < len(batches) - 1:
            time.sleep(SLEEP_BETWEEN_BATCHES)

    print(f"\nDone! Updated {total_updated} words total.")


if __name__ == '__main__':
    main()
