#!/usr/bin/env python3
"""Generate a reviewable verb_type mapping and idempotent PostgreSQL backfill SQL.

The production PostgreSQL word bank encodes transitivity inside part_of_speech
("自动词" / "他动词" / "自他動2" / "ナ形・名・自動3"), while ~610 verbs have no
transitivity at all.  This script normalises the former and classifies the latter.

Read-only exports needed (see --csv / --examples):

    PGPASSWORD=<password> psql -h <host> -p <port> -U <user> -d japanese -c "\\
    copy (SELECT w.id, w.kanji_form, w.reading, w.part_of_speech, w.meaning,
                 COALESCE(e.n_ex,0) AS n_ex, COALESCE(e.n_wo,0) AS n_wo,
                 COALESCE(e.n_ga,0) AS n_ga
          FROM words w LEFT JOIN (SELECT word_id, count(*) n_ex,
                 count(*) FILTER (WHERE japanese LIKE '%を%') n_wo,
                 count(*) FILTER (WHERE japanese LIKE '%が%') n_ga
                 FROM word_examples GROUP BY word_id) e ON e.word_id=w.id
          WHERE w.part_of_speech ~ '動|动|サ変|サ变|ザ变' ORDER BY w.id)
    TO '<csv>' WITH CSV HEADER"

    PGPASSWORD=<password> psql ... -c "\\
    copy (SELECT word_id, position, japanese FROM word_examples ORDER BY word_id, position)
    TO '<examples>' WITH CSV HEADER"

Usage:
    python3 scripts/backfill_verb_type.py --csv /tmp/jla_verbs.csv \
        --examples /tmp/jla_all_examples.csv --out-dir data/verb_type

No database connection is made here; the generated SQL is applied separately
inside a transaction after taking a backup.
"""

import argparse
import csv
import os
from collections import Counter, defaultdict

INTRANSITIVE = "自动词"
TRANSITIVE = "他动词"
BOTH = "自他两用"

# part_of_speech values that are NOT verbs even though they contain 動/动.
NON_VERB = {"形容動詞", "感動詞", "名詞・サ変", "助动词", "助動詞"}

# 例句里的「を」是离开点/经过点/原因而非宾语，这些动词仍是自动词。
FORCE_INTRANSITIVE = set("""
降りる 下りる 出る 出かける 走る 渡る 通る 過ぎる 越える 越す 超える 超す 至る 歩く 飛ぶ 曲がる 離れる
登る 上る 下る 泳ぐ 駆ける 回る 巡る 流れる 伝わる 進む 落ちる 抜ける 横切る 飛び出す 引き返す 暮らす
出くわす 近づく 遠ざかる 向かう 戻る 帰る 行く 来る 入る 卒業する 旅行する 散歩する 通う 通学する
通勤する 去る 滑る 転ぶ 抜ける 経る 駆け抜ける 通り過ぎる 散る 昇る 立つ 座る 並ぶ 住む 勤める
働く 休む 遊ぶ 会う 結婚する 死ぬ 生きる 要る 足りる 困る 揃う 残る 鳴る 空く 込む 済む 乾く
光る 太る 痩せる 慣れる 遅れる 汚れる 濡れる 焼ける 折れる 割れる 壊れる 倒れる 咲く 枯れる 凍る
溶ける 晴れる 曇る 降る 震える 揺れる 輝く 匂う 響く 聞こえる 見える 疲れる 増える 減る 決まる
集まる 止まる 上がる 下がる 直る 治る 続く 付く 開く 閉まる 締まる 寝る 起きる 生まれる 怒る
驚く 騒ぐ 逃げる 似る 眠る 泣く 黙る 燃える 飽きる 溺れる 暮らす 変わる 待つ 迷う 分かる
""".split())

# 72 个「例句无信号」词的逐条人工复核结果（多为ます形/サ变与状态自动词）。
MANUAL_REVIEW = {}
for _label, _words in (
    (INTRANSITIVE, """
いらっしゃる 触る 間に合う 寄る 別れる 倒産します 発言します 優勝します 合格します
けんかします 相談します 頑張ります 遠慮します 引っ越します 出発します 転職します
留学します 入学します お邪魔します 終る ぬれる 狂う あきれる 暖まる 応ずる 落着く
凍える 射す 錆びる しぼむ 透き通る すれちがう 通ずる どなる 倣う 這う 凹む 交ざる
嵩張る 霞む 興じる 朽ちる 混む 裂ける 湿気る 準じる 聳える 背く 辿り着く 出合う
出切る 照り返す 賑わう 捗る 匹敵 辟易 謙る まごつく 面する 揉める
"""),
    (TRANSITIVE, "約束します シュートします 轢く 卸す 信ずる 繋げる 賭ける 諮る"),
    (BOTH, "注意します かまう 振舞う 堪える"),
):
    for _word in _words.split():
        MANUAL_REVIEW[_word] = _label

# 线上抽查（全库随机 + 按判定来源分层）后逐条人工定稿的结果。
# 这批词的问题在于例句里「别的动词」的 を/が 会污染信号，或名词/使役形误触发。
# 空字符串表示人工判定为非动词，不打标签（回填时会清空）。
MANUAL_AUDIT = {}
for _label, _words in (
    (INTRANSITIVE, """
動く 決る 降り出す びっくりする 代る 清む 縮れる 引っ繰り返る 殖える 植わる 準ずる
垂れる 捻じれる 笑う 蘇る 係わる 差し掛かる 隔たる 食い違う 飛び出す 辿り着く
引っ越す 逃げ出す 抜け出す
合う 謝る 移る 参る 負ける 曲る 罹る 居る 気付く 追い付く 片寄る 仕上がる 茂る
存ずる 通り掛かる 煮える 話し掛ける 放れる 塞がる 交じる 報ずる 添う 跡切れる
とろける 老ける またがる 結び付く 怯える 煙る こもる 綻びる 馴れる
呼び掛ける 着飾る 微笑む
"""),
    (TRANSITIVE, """
比べる くれる 取り替える いただく 止す 帰す 焦がす 転がす 揃える 炊く 撫でる
はがす 蒔く またぐ 揉む 呼び出す 口ずさむ 心掛ける 仕掛ける 慎む 連ねる
取り調べる 詰る 乗っ取る 引っ掻く 思い付く 立てる 蹴る 捕る 剥く 儲ける
召し上がる うかがう 申す 行う 覚える 読む 驚かす 心がける 飲み込む 咎める
訳します あびる ほめる 繋ぐ 扇ぐ あてはめる 追い掛ける おどかす 代える 言付ける
近付ける 交ぜる 宛てる 打ち合わせる 掻き回す 沈める 馴らす 見掛ける 申し入れる
持てる 弄る 言い付ける 葬る 払い戻す 悲しむ 煽ぐ
"""),
    (BOTH, "開く 吹く 振舞う 注意します かまう 堪える 踊る 踊ります かける"),
    ("", "引き出し よると"),
):
    for _word in _words.split():
        MANUAL_AUDIT[_word] = _label

FORCE_BOTH = set("""
開く 吹く する 笑う 休む 答える 注ぐ 積もる 去る 恐れる 通じる 閉じる 増やす 減らす
""".split())

FORCE_TRANSITIVE = set("""
歌う 支える 備える 用いる 悔やむ 捲る 設ける 施す 遂げる 果たす 担う 負う 恨む 羨む 責める
戒める 罰する 禁ずる 命ずる 論ずる 称する 制する 属する 適する 害する 化する 要する 接する
達する 熱する 恋する 知る 持つ 作る 使う 取る 読む 書く 飲む 食べる 見る 聞く
話す 買う 売る 教える 覚える 忘れる 考える 選ぶ 送る 捨てる 運ぶ 払う 塗る 盗む 投げる
直す 探す 調べる 伝える 届ける 迎える 助ける 守る 答える 破る 引く 押す 回す まとめる
行う 辞める 担当する 案内する 交換する 紹介する 利用する 提出する 約束する チェックする
看病する 訳する 看病 スケッチする 発言する 注意する 注意 シュートする サボる
""".split())

VERB_ENDINGS = tuple("うくぐすつぬぶむる")

# 五段动词ます形词干末假名（い段）→ 基本形（う段）。
I_TO_U = {
    "き": "く", "ぎ": "ぐ", "し": "す", "じ": "ず", "ち": "つ", "ぢ": "づ",
    "に": "ぬ", "ひ": "ふ", "び": "ぶ", "み": "む", "り": "る", "い": "う",
}


def normalize_pos(pos: str) -> str:
    """Mirror word.NormalizeVerbType in Go: extract transitivity from 词性字符串."""
    if any(k in pos for k in ("自他動", "自他动", "自他两用")):
        return BOTH
    if any(k in pos for k in ("自動", "自动词")):
        return INTRANSITIVE
    if any(k in pos for k in ("他動", "他动词")):
        return TRANSITIVE
    return ""


def is_verb(row: dict) -> bool:
    return row["part_of_speech"] not in NON_VERB


def masu_stem(reading: str) -> str:
    for suffix in ("します", "ます"):
        if reading.endswith(suffix):
            return reading[: -len(suffix)]
    return ""


def base_readings(row: dict):
    """ます形还原出的可能基本形读音，如 あつまります→あつまる、たべます→たべる。"""
    stem = masu_stem(row["reading"])
    if len(stem) < 2:
        return []
    candidates = []
    if stem[-1] in I_TO_U:
        candidates.append(stem[:-1] + I_TO_U[stem[-1]])
    candidates.append(stem + "る")
    return candidates


def verb_stems(kanji: str, reading: str):
    """目标动词的活用词干候选（汉字与假名两种表记都覆盖）。"""
    result = set()
    for word in (kanji, reading):
        for n in (1, 2):
            if len(word) > n:
                result.add(word[:-n])
    for suffix in ("します", "する", "ずる", "じる", "ます"):
        for word in (kanji, reading):
            if word.endswith(suffix) and len(word) > len(suffix):
                result.add(word[: -len(suffix)])
    return [s for s in result if s]


def precise_signal(row: dict, examples):
    """只统计「を/が 后面紧跟目标动词活用形」的搭配，忽略句中其它动词的格助词。

    旧版只看整句是否出现 を/が，像「ベティーにかわって手紙を書いた」会把「書いた」
    的宾语算到「代る」头上。位置阈值 3 允许「をそっと開いた」这类插入。
    """
    stems = verb_stems(row["kanji_form"], row["reading"])
    wo = ga = 0
    for sentence in examples.get(row["id"], []):
        for i, ch in enumerate(sentence):
            if ch not in "をが":
                continue
            after = sentence[i + 1: i + 10]
            pos = min((after.find(s) for s in stems if s in after), default=99)
            if pos <= 3:
                if ch == "を":
                    wo += 1
                else:
                    ga += 1
    if wo > 0 and ga == 0:
        return TRANSITIVE, wo, ga
    if ga > 0 and wo == 0:
        return INTRANSITIVE, wo, ga
    if wo > 0 and ga > 0:
        if wo == ga:
            return BOTH, wo, ga
        return (TRANSITIVE if wo > ga else INTRANSITIVE), wo, ga
    return None, wo, ga


def whole_sentence_signal(row: dict):
    """回退信号：整句是否含 を/が。精度较低但覆盖广，仅当精确信号无命中时使用。"""
    wo, ga = int(row["n_wo"]), int(row["n_ga"])
    if wo > 0 and ga == 0:
        return TRANSITIVE
    if ga > 0 and wo == 0:
        return INTRANSITIVE
    if wo > 0 and ga > 0:
        if wo == ga:
            return BOTH
        return TRANSITIVE if wo > ga else INTRANSITIVE
    return None


def tail_rule(kanji: str):
    if kanji.endswith(("します", "ます")):
        return None
    if kanji.endswith("す"):
        return TRANSITIVE          # 交叉验证 94.6%
    if kanji.endswith(("ある", "がる", "まる", "わる", "かる", "たる", "なる")):
        return INTRANSITIVE        # 交叉验证 91.5%
    return None


def make_record(row: dict, label: str, source: str, confidence: str, wo=None, ga=None) -> dict:
    return {
        "id": row["id"], "kanji_form": row["kanji_form"], "reading": row["reading"],
        "part_of_speech": row["part_of_speech"], "meaning": row["meaning"],
        "n_wo": row["n_wo"] if wo is None else wo,
        "n_ga": row["n_ga"] if ga is None else ga,
        "verb_type": label, "source": source, "confidence": confidence,
    }


def sql_quote(value: str) -> str:
    return "'" + value.replace("'", "''") + "'"


def load_examples(path: str):
    result = defaultdict(list)
    with open(path, encoding="utf-8", newline="") as f:
        for row in csv.DictReader(f):
            result[row["word_id"]].append(row["japanese"])
    return result


def main() -> None:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--csv", required=True, help="CSV exported from production words")
    parser.add_argument("--examples", required=True, help="CSV of word_examples (word_id, position, japanese)")
    parser.add_argument("--out-dir", default="data/verb_type", help="output directory")
    args = parser.parse_args()

    with open(args.csv, encoding="utf-8", newline="") as f:
        rows = list(csv.DictReader(f))
    examples = load_examples(args.examples)

    def classify(row: dict, reuse):
        kanji = row["kanji_form"]
        direct = normalize_pos(row["part_of_speech"])
        sig, wo, ga = precise_signal(row, examples)
        sig_source = "精确例句信号"
        if sig is None:
            sig = whole_sentence_signal(row)
            sig_source = "整句例句信号(回退)"
        tail = tail_rule(kanji)
        if direct:
            return direct, "词性已有标注", "high", wo, ga
        if kanji in MANUAL_AUDIT:
            return MANUAL_AUDIT[kanji], "线上抽查人工定稿", "high", wo, ga
        if kanji in MANUAL_REVIEW:
            return MANUAL_REVIEW[kanji], "人工逐条复核", "high", wo, ga
        if kanji in FORCE_BOTH:
            return BOTH, "人工校正(自他两用)", "high", wo, ga
        if kanji in FORCE_INTRANSITIVE:
            return INTRANSITIVE, "人工校正(移动/状态自动词)", "high", wo, ga
        if kanji in FORCE_TRANSITIVE:
            return TRANSITIVE, "人工校正(明确他动词)", "high", wo, ga

        reused, original = reuse(row)
        if reused:
            return reused, "基本形还原:" + original, "high", wo, ga
        if sig and tail and sig != tail and sig != BOTH:
            return BOTH, sig_source + "与词尾规则冲突", "medium", wo, ga
        if sig == BOTH:
            return BOTH, sig_source + "冲突", "medium", wo, ga
        if sig:
            return sig, sig_source + ("+词尾规则" if tail == sig else ""), "high" if tail == sig else "medium", wo, ga
        if tail:
            return tail, "词尾规则", "medium", wo, ga
        return BOTH, "无信号默认自他两用", "low", wo, ga

    def no_reuse(row: dict):
        return None, None

    # Pass 1: 基本形等非ます形动词先判定，结果按读音建索引。
    first_pass = {}
    results = []
    for row in rows:
        if not is_verb(row) or masu_stem(row["reading"]):
            continue
        label, source, confidence, wo, ga = classify(row, no_reuse)
        first_pass.setdefault(row["reading"], (label, row["kanji_form"]))
        results.append(make_record(row, label, source, confidence, wo, ga))

    # Pass 2: ます形动词严格还原成基本形读音后继承标签，拒绝同音异义词。
    def reuse_base(row: dict):
        for reading in base_readings(row):
            if reading in first_pass:
                label, kanji = first_pass[reading]
                return label, kanji
        return None, None

    for row in rows:
        if not is_verb(row) or not masu_stem(row["reading"]):
            continue
        label, source, confidence, wo, ga = classify(row, reuse_base)
        results.append(make_record(row, label, source, confidence, wo, ga))

    results.sort(key=lambda r: int(r["id"]))

    os.makedirs(args.out_dir, exist_ok=True)
    review_path = os.path.join(args.out_dir, "review.csv")
    with open(review_path, "w", encoding="utf-8", newline="") as f:
        writer = csv.DictWriter(f, fieldnames=list(results[0].keys()))
        writer.writeheader()
        writer.writerows(results)

    values = ",\n".join(
        f"  ({r['id']}, {sql_quote(r['verb_type'])})" for r in results
    )
    backfill = f"""-- Generated by scripts/backfill_verb_type.py. Apply inside one transaction.
-- Backup first: CREATE TABLE words_backup_verb_type AS SELECT * FROM words;
BEGIN;
ALTER TABLE words ADD COLUMN IF NOT EXISTS verb_type TEXT NOT NULL DEFAULT '';
CREATE TEMP TABLE _verb_type_map (id BIGINT PRIMARY KEY, verb_type TEXT NOT NULL) ON COMMIT DROP;
INSERT INTO _verb_type_map (id, verb_type) VALUES
{values};
UPDATE words w SET verb_type = m.verb_type FROM _verb_type_map m WHERE w.id = m.id;
UPDATE words SET verb_type = '' WHERE verb_type <> '' AND id NOT IN (SELECT id FROM _verb_type_map);
COMMIT;
"""
    with open(os.path.join(args.out_dir, "backfill.sql"), "w", encoding="utf-8") as f:
        f.write(backfill)

    rollback = """-- Rollback of backfill_verb_type.py (restores verb_type to empty).
-- The column was newly added, so every row was empty before the backfill.
BEGIN;
UPDATE words SET verb_type = '';
COMMIT;
-- To remove the column entirely: ALTER TABLE words DROP COLUMN IF EXISTS verb_type;
"""
    with open(os.path.join(args.out_dir, "rollback.sql"), "w", encoding="utf-8") as f:
        f.write(rollback)

    anomalies = [
        r for r in rows
        if r["part_of_speech"] in ("自动词", "他动词")
        and not r["kanji_form"].endswith(VERB_ENDINGS)
        and not r["kanji_form"].endswith(("する", "ずる", "ます"))
    ]
    with open(os.path.join(args.out_dir, "anomalies.csv"), "w", encoding="utf-8", newline="") as f:
        writer = csv.DictWriter(f, fieldnames=["id", "kanji_form", "reading", "part_of_speech", "meaning"])
        writer.writeheader()
        for r in anomalies:
            writer.writerow({k: r[k] for k in writer.fieldnames})

    print(f"输入动词类词={len(rows)} 生成映射={len(results)} 疑似错标={len(anomalies)}")
    print("标签分布:")
    for k, v in Counter(r["verb_type"] or "(不打标签)" for r in results).most_common():
        print(f"  {k}: {v}")
    print("判定来源:")
    for k, v in Counter(r["source"].split(":")[0] for r in results).most_common():
        print(f"  {k}: {v}")
    print("置信度:")
    for k, v in Counter(r["confidence"] for r in results).most_common():
        print(f"  {k}: {v}")
    print(f"输出: {review_path}, backfill.sql, rollback.sql, anomalies.csv")


if __name__ == "__main__":
    main()
