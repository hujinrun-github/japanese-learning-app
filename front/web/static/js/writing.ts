/**
 * writing.ts — Writing practice: input mode (exact match) + sentence mode (AI feedback).
 */

import { apiFetch } from "./api.js";

interface WritingQuestion {
  id: number;
  type: "input" | "sentence";
  prompt: string;
  jlpt_level: string;
}

interface AIFeedback {
  corrected: string;
  explanation: string;
  score: number;
}

interface InputResult {
  score: number;   // 100 | 0
  correct: boolean;
  expected_answer?: string;
}

interface SentenceResult {
  score: number;
  ai_feedback: AIFeedback;
}

// ── DOM refs ─────────────────────────────────────────────────────────────────
const tabButtons = document.querySelectorAll<HTMLButtonElement>(".tab-bar__tab");
const progressBar = document.getElementById("writing-progress") as HTMLDivElement;
const progressLabel = document.getElementById("writing-progress-label") as HTMLSpanElement;
const questionPanel = document.getElementById("question-panel") as HTMLDivElement;
const promptEl = document.getElementById("question-prompt") as HTMLParagraphElement;
const answerInput = document.getElementById("answer-input") as HTMLInputElement;
const feedbackEl = document.getElementById("answer-feedback") as HTMLDivElement;
const submitBtn = document.getElementById("submit-btn") as HTMLButtonElement;
const aiFeedbackPanel = document.getElementById("ai-feedback-panel") as HTMLDivElement;
const aiCorrected = document.getElementById("ai-corrected") as HTMLParagraphElement;
const aiExplanation = document.getElementById("ai-explanation") as HTMLParagraphElement;
const aiScore = document.getElementById("ai-score") as HTMLElement;
const nextQuestionBtn = document.getElementById("next-question-btn") as HTMLButtonElement;
const sessionSummary = document.getElementById("session-summary") as HTMLDivElement;
const sessionAccuracy = document.getElementById("session-accuracy") as HTMLElement;
const restartBtn = document.getElementById("restart-btn") as HTMLButtonElement;

// ── State ────────────────────────────────────────────────────────────────────
let currentMode: "input" | "sentence" = "input";
let queue: WritingQuestion[] = [];
let currentIndex = 0;
let correctCount = 0;

// ── Init ─────────────────────────────────────────────────────────────────────
async function loadQueue(mode: "input" | "sentence"): Promise<void> {
  questionPanel.hidden = true;
  aiFeedbackPanel.hidden = true;
  sessionSummary.hidden = true;
  try {
    queue = await apiFetch<WritingQuestion[]>(`/api/v1/writing/queue?type=${mode}`);
  } catch (err) {
    promptEl.textContent = `エラー: ${(err as Error).message}`;
    questionPanel.hidden = false;
    return;
  }
  currentIndex = 0;
  correctCount = 0;
  showQuestion();
}

function updateProgress(): void {
  const total = queue.length;
  const done = currentIndex;
  const pct = total > 0 ? Math.round((done / total) * 100) : 0;
  progressBar.style.width = `${pct}%`;
  progressBar.setAttribute("aria-valuenow", String(pct));
  progressLabel.textContent = `${done} / ${total}`;
}

function showQuestion(): void {
  if (currentIndex >= queue.length) {
    showSessionSummary();
    return;
  }
  const q = queue[currentIndex];
  promptEl.textContent = q.prompt;
  answerInput.value = "";
  feedbackEl.hidden = true;
  aiFeedbackPanel.hidden = true;
  submitBtn.disabled = false;
  questionPanel.hidden = false;
  updateProgress();
  answerInput.focus();
}

function showSessionSummary(): void {
  questionPanel.hidden = true;
  aiFeedbackPanel.hidden = true;
  const accuracy = queue.length > 0
    ? `${Math.round((correctCount / queue.length) * 100)}%`
    : "0%";
  sessionAccuracy.textContent = accuracy;
  sessionSummary.hidden = false;
  updateProgress();
}

// ── Submit ────────────────────────────────────────────────────────────────────
async function submitAnswer(): Promise<void> {
  const q = queue[currentIndex];
  const answer = answerInput.value.trim();
  if (!answer) return;

  submitBtn.disabled = true;

  if (currentMode === "input") {
    await submitInput(q, answer);
  } else {
    await submitSentence(q, answer);
  }
}

async function submitInput(q: WritingQuestion, answer: string): Promise<void> {
  try {
    const result = await apiFetch<InputResult>("/api/v1/writing/input", {
      method: "POST",
      body: JSON.stringify({ question_id: q.id, answer }),
    });

    feedbackEl.hidden = false;
    if (result.correct) {
      feedbackEl.textContent = "✅ 正解！";
      feedbackEl.className = "question-panel__feedback question-panel__feedback--correct";
      correctCount++;
    } else {
      feedbackEl.textContent = `❌ 不正解。正解: ${result.expected_answer ?? ""}`;
      feedbackEl.className = "question-panel__feedback question-panel__feedback--wrong";
    }

    setTimeout(() => {
      currentIndex++;
      showQuestion();
    }, 1200);
  } catch (err) {
    feedbackEl.textContent = `エラー: ${(err as Error).message}`;
    feedbackEl.hidden = false;
    submitBtn.disabled = false;
  }
}

async function submitSentence(q: WritingQuestion, answer: string): Promise<void> {
  try {
    const result = await apiFetch<SentenceResult>("/api/v1/writing/sentence", {
      method: "POST",
      body: JSON.stringify({ question_id: q.id, answer }),
    });

    questionPanel.hidden = true;
    aiCorrected.textContent = result.ai_feedback.corrected;
    aiExplanation.textContent = result.ai_feedback.explanation;
    aiScore.textContent = String(result.score);
    aiFeedbackPanel.hidden = false;

    if (result.score >= 70) correctCount++;
  } catch (err) {
    feedbackEl.textContent = `エラー: ${(err as Error).message}`;
    feedbackEl.hidden = false;
    submitBtn.disabled = false;
  }
}

// ── Event listeners ──────────────────────────────────────────────────────────
submitBtn.addEventListener("click", submitAnswer);

answerInput.addEventListener("keydown", (e: KeyboardEvent) => {
  if (e.key === "Enter") submitAnswer();
});

nextQuestionBtn.addEventListener("click", () => {
  currentIndex++;
  showQuestion();
});

restartBtn.addEventListener("click", () => loadQueue(currentMode));

tabButtons.forEach(btn => {
  btn.addEventListener("click", () => {
    tabButtons.forEach(b => b.classList.remove("tab-bar__tab--active"));
    btn.classList.add("tab-bar__tab--active");
    currentMode = btn.dataset["mode"] as "input" | "sentence";
    loadQueue(currentMode);
  });
});

// ── Bootstrap ─────────────────────────────────────────────────────────────────
loadQueue(currentMode);
