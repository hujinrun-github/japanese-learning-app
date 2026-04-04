/**
 * summary.ts — Session summary list + detail rendering.
 */

import { apiFetch } from "./api.js";

interface ScoreSummary {
  total: number;
  correct: number;
  accuracy_pct: number;
}

interface SessionSummary {
  session_id: string;
  module: string;
  score_summary: ScoreSummary;
  strengths: string[];
  weaknesses: string[];
  suggestions: string[];
  generated_at: string;
}

// ── DOM refs ─────────────────────────────────────────────────────────────────
const summaryList = document.getElementById("summary-list") as HTMLUListElement;
const summaryEmpty = document.getElementById("summary-empty") as HTMLDivElement;
const summaryDetail = document.getElementById("summary-detail") as HTMLDivElement;
const summaryDetailTitle = document.getElementById("summary-detail-title") as HTMLHeadingElement;
const summaryScoreBlock = document.getElementById("summary-score-block") as HTMLDivElement;
const summaryStrengths = document.getElementById("summary-strengths") as HTMLUListElement;
const summaryWeaknesses = document.getElementById("summary-weaknesses") as HTMLUListElement;
const summarySuggestions = document.getElementById("summary-suggestions") as HTMLUListElement;
const backBtn = document.getElementById("summary-back-btn") as HTMLButtonElement;

// ── Init ─────────────────────────────────────────────────────────────────────
async function init(): Promise<void> {
  try {
    const summaries = await apiFetch<SessionSummary[]>("/api/v1/summary");
    if (!summaries || summaries.length === 0) {
      summaryEmpty.hidden = false;
      return;
    }
    renderList(summaries);
  } catch (err) {
    summaryList.innerHTML = `<li>エラー: ${(err as Error).message}</li>`;
  }
}

function renderList(summaries: SessionSummary[]): void {
  summaryList.innerHTML = "";
  for (const s of summaries) {
    const li = document.createElement("li");
    li.className = "summary-list__item card-list__item";
    const date = new Date(s.generated_at).toLocaleDateString("ja-JP");
    li.innerHTML = `
      <button class="card-list__link btn--reset">
        <span class="card-list__title">${moduleLabel(s.module)}</span>
        <span class="summary-list__date">${date}</span>
        <span class="badge">${s.score_summary.accuracy_pct}%</span>
      </button>`;
    li.querySelector("button")!.addEventListener("click", () => showDetail(s));
    summaryList.appendChild(li);
  }
}

function moduleLabel(module: string): string {
  const labels: Record<string, string> = {
    word: "単語",
    grammar: "文法",
    lesson: "課文",
    speaking: "口語",
    writing: "ライティング",
  };
  return labels[module] ?? module;
}

function showDetail(s: SessionSummary): void {
  summaryList.hidden = true;
  summaryEmpty.hidden = true;

  summaryDetailTitle.textContent = `${moduleLabel(s.module)} — ${new Date(s.generated_at).toLocaleDateString("ja-JP")}`;

  summaryScoreBlock.innerHTML = `
    <p>${s.score_summary.correct} / ${s.score_summary.total} 正解 (${s.score_summary.accuracy_pct}%)</p>`;

  renderStringList(summaryStrengths, s.strengths);
  renderStringList(summaryWeaknesses, s.weaknesses);
  renderStringList(summarySuggestions, s.suggestions);

  summaryDetail.hidden = false;
}

function renderStringList(el: HTMLUListElement, items: string[]): void {
  el.innerHTML = "";
  if (!items || items.length === 0) {
    el.innerHTML = "<li>データなし</li>";
    return;
  }
  for (const item of items) {
    const li = document.createElement("li");
    li.textContent = item;
    el.appendChild(li);
  }
}

backBtn.addEventListener("click", () => {
  summaryDetail.hidden = true;
  summaryList.hidden = false;
});

// ── Bootstrap ─────────────────────────────────────────────────────────────────
init();
