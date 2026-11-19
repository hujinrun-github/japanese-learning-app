"""Checks for the textbook material import's completeness and duplicate policy."""
import unittest

from biaori_speaking import audio_filename, canonical_text, normalize_lines, plan_import, spoken_text, validate_materials


class BiaoriSpeakingTests(unittest.TestCase):
    def test_spoken_text_excludes_labels_but_keeps_sentence_numbers(self):
        cases = [
            ("1. 今4時です。\n甲：何時ですか。\n乙：4時です。", "今4時です。\n何時ですか。\n4時です。"),
            ("小野：李さん、こんにちは。\n李：こんにちは。", "李さん、こんにちは。\nこんにちは。"),
        ]
        for text, expected in cases:
            with self.subTest(text=text):
                self.assertEqual(spoken_text(text), expected)

    def test_line_continuations_and_numbers(self):
        self.assertEqual(normalize_lines(["甲:はい,そうです。", "よろしくお願いします。"]),
                         ["甲：はい、そうです。よろしくお願いします。"])
        self.assertEqual(normalize_lines(["乙:5,800円です。"]), ["乙：5,800円です。"])

    def test_duplicates_ignore_labels_spaces_and_punctuation(self):
        self.assertEqual(canonical_text("甲：こんにちは。\n乙：はい。"),
                         canonical_text("A: こんにちは！\nB:はい。"))
        self.assertNotEqual(canonical_text("1. 4時です。"), canonical_text("1. 5時です。"))

    def test_skip_duplicates_without_removing_existing_rows(self):
        existing = [{"id": 7, "title": "existing", "text": "甲：こんにちは。"}]
        items = [
            {"title": "new name", "text": "こんにちは。"},
            {"title": "second", "text": "さようなら。"},
            {"title": "third", "text": "さようなら！"},
        ]
        pending, skipped = plan_import(items, existing)
        self.assertEqual([x["title"] for x in pending], ["second"])
        self.assertEqual(len(skipped), 2)
        self.assertEqual(existing[0]["id"], 7)

    def test_changed_existing_title_is_an_error(self):
        with self.assertRaisesRegex(ValueError, "title"):
            plan_import([{"title": "same", "text": "こんばんは。"}],
                        [{"id": 1, "title": "same", "text": "こんにちは。"}])

    def test_content_addressed_audio_filename(self):
        import hashlib
        text = "甲：こんにちは。"
        self.assertEqual(audio_filename(text), hashlib.sha256(text.encode()).hexdigest()[:16] + ".wav")

    def test_incomplete_book_is_rejected(self):
        with self.assertRaisesRegex(ValueError, "96"):
            validate_materials([])


if __name__ == "__main__":
    unittest.main()
