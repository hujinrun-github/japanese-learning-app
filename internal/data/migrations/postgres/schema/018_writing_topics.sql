CREATE TABLE writing_topics (
    id TEXT PRIMARY KEY,
    title TEXT NOT NULL CHECK (btrim(title) <> ''),
    jlpt_level TEXT NOT NULL CHECK (jlpt_level IN ('N5','N4','N3','N2','N1')),
    essay_prompt TEXT NOT NULL CHECK (btrim(essay_prompt) <> ''),
    sentence_prompt TEXT NOT NULL CHECK (btrim(sentence_prompt) <> ''),
    position INTEGER NOT NULL CHECK (position >= 0),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

INSERT INTO writing_topics (id,title,jlpt_level,essay_prompt,sentence_prompt,position) VALUES
    ('weekend', '我的周末', 'N5', '用日语写 4～6 句，介绍上个周末。写清楚你去了哪里、做了什么，以及当时的感受。试着用「それから」串起两件事。', '用「〜から」写 1 个完整的日语句子，说明你周末做了什么，以及为什么这么做。', 0),
    ('favorite-place', '我喜欢的一个地方', 'N5', '用日语写 4～6 句，向朋友介绍一个你喜欢的地方。写出它在哪里、能在那里做什么，以及你喜欢它的理由。', '用「〜が好きです」和一个表示理由的表达，写 1 个完整的日语句子，推荐一个你喜欢的地方。', 1),
    ('trip-plan', '给朋友的旅行提议', 'N4', '用日语给朋友写一条 4～6 句的消息，提议一起去旅行。说明目的地、出发时间、想做的事，以及出发前要准备什么。', '用「〜ておく」写 1 个完整的日语句子，告诉朋友出发前你会提前准备什么。', 2),
    ('new-habit', '我养成的一个新习惯', 'N4', '用日语写 4～6 句，介绍你最近养成的一个习惯。比较过去和现在的做法，并举例说明这个习惯给你带来的变化。', '用「〜ようになった」写 1 个完整的日语句子，描述养成这个习惯后你现在能做到什么。', 3),
    ('study-choice', '独自学习还是一起学习？', 'N3', '用日语写 4～6 句，说明你更喜欢哪种学习方式。比较两种方式，举一个亲身经历的例子，最后总结你的选择。', '用「〜だけでなく〜も」写 1 个完整的日语句子，说明你偏好的学习方式有哪两个好处。', 4);
