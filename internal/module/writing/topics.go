package writing

// WritingTopic offers two ways to practice writing about the same situation.
type WritingTopic struct {
	ID             string `json:"id"`
	Title          string `json:"title"`
	JLPTLevel      string `json:"jlpt_level"`
	EssayPrompt    string `json:"essay_prompt"`
	SentencePrompt string `json:"sentence_prompt"`
}

func writingTopics() []WritingTopic {
	return []WritingTopic{
		{
			ID:             "weekend",
			Title:          "我的周末",
			JLPTLevel:      "N5",
			EssayPrompt:    "用日语写 4～6 句，介绍上个周末。写清楚你去了哪里、做了什么，以及当时的感受。试着用「それから」串起两件事。",
			SentencePrompt: "用「〜から」写 1 个完整的日语句子，说明你周末做了什么，以及为什么这么做。",
		},
		{
			ID:             "favorite-place",
			Title:          "我喜欢的一个地方",
			JLPTLevel:      "N5",
			EssayPrompt:    "用日语写 4～6 句，向朋友介绍一个你喜欢的地方。写出它在哪里、能在那里做什么，以及你喜欢它的理由。",
			SentencePrompt: "用「〜が好きです」和一个表示理由的表达，写 1 个完整的日语句子，推荐一个你喜欢的地方。",
		},
		{
			ID:             "trip-plan",
			Title:          "给朋友的旅行提议",
			JLPTLevel:      "N4",
			EssayPrompt:    "用日语给朋友写一条 4～6 句的消息，提议一起去旅行。说明目的地、出发时间、想做的事，以及出发前要准备什么。",
			SentencePrompt: "用「〜ておく」写 1 个完整的日语句子，告诉朋友出发前你会提前准备什么。",
		},
		{
			ID:             "new-habit",
			Title:          "我养成的一个新习惯",
			JLPTLevel:      "N4",
			EssayPrompt:    "用日语写 4～6 句，介绍你最近养成的一个习惯。比较过去和现在的做法，并举例说明这个习惯给你带来的变化。",
			SentencePrompt: "用「〜ようになった」写 1 个完整的日语句子，描述养成这个习惯后你现在能做到什么。",
		},
		{
			ID:             "study-choice",
			Title:          "独自学习还是一起学习？",
			JLPTLevel:      "N3",
			EssayPrompt:    "用日语写 4～6 句，说明你更喜欢哪种学习方式。比较两种方式，举一个亲身经历的例子，最后总结你的选择。",
			SentencePrompt: "用「〜だけでなく〜も」写 1 个完整的日语句子，说明你偏好的学习方式有哪两个好处。",
		},
	}
}
