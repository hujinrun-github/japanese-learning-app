-- Reviewed questions for the N5 pilot lesson. Check every sentence and its
-- position so a lesson with the same title cannot receive unrelated questions.
WITH matched_lesson AS (
    SELECT l.id
    FROM lessons l
    WHERE l.title = '私の一日' AND l.jlpt_level = 'N5'
      AND (
          SELECT jsonb_agg(jsonb_build_array(s.position, (
              SELECT string_agg(token->>'surface', '' ORDER BY ord)
              FROM jsonb_array_elements(s.tokens) WITH ORDINALITY AS t(token, ord)
          )) ORDER BY s.position)
          FROM lesson_sentences s
          WHERE s.lesson_id = l.id
      ) = '[
        [0, "毎朝、七時に起きます。"],
        [1, "朝ごはんはパンとコーヒーです。"],
        [2, "八時半に学校へ行きます。"],
        [3, "午後は図書館で勉強します。"],
        [4, "夜は家族と一緒に晩ごはんを食べます。"],
        [5, "十一時ごろに寝ます。"]
      ]'::jsonb
)
INSERT INTO listening_questions (
    id, lesson_id, position, prompt, options_json, correct_option_index, explanation, evidence_sentence_index
)
SELECT q.id, m.id, q.position, q.prompt, q.options, q.correct, q.explanation, q.evidence
FROM matched_lesson m
CROSS JOIN (VALUES
    ('n5-my-day-q1', 0, '毎朝、何時に起きますか。',
     '["六時","七時","八時","九時"]'::jsonb, 1, '第一句「七時に起きます」说明每天早上七点起床。', 0),
    ('n5-my-day-q2', 1, '朝ごはんは何ですか。',
     '["ごはんとお茶","パンと牛乳","パンとコーヒー","おにぎりと水"]'::jsonb, 2, '第二句「パンとコーヒーです」说明早餐是面包和咖啡。', 1),
    ('n5-my-day-q3', 2, '午後、どこで勉強しますか。',
     '["図書館","家","教室","公園"]'::jsonb, 0, '第四句「図書館で勉強します」说明下午在图书馆学习。', 3)
) AS q(id, position, prompt, options, correct, explanation, evidence)
ON CONFLICT DO NOTHING;
