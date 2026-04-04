/**
 * grammar.ts — Grammar list + detail / quiz interaction.
 */

import { apiFetch } from "./api.js";

interface GrammarExample {
  japanese: string;
  reading: string;
  translation: string;
}

interface QuizOption {
  text: string;
  is_correct: boolean;
  explanation: string;
}

interface QuizQuestion {
  question: string;
  options: QuizOption[];
}

interface GrammarPoint {
  id: number;
  name: string;
  meaning: string;
  conjunction_rule: string;
  usage_note: string;
  examples: GrammarExample[];
  quiz_questions: QuizQuestion[];
  jlpt_level: string;
}

// ── Page detection ────────────────────────────────────────────────────────────
const isDetail = document.getElementById("grammar-detail") !== null;

if (isDetail) {
  initDetail();
} else {
  initList();
}

// ── List page ─────────────────────────────────────────────────────────────────
async function initList(): Promise<void> {
  const list = document.getElementById("grammar-list") as HTMLUListElement;
  const empty = document.getElementById("grammar-empty") as HTMLDivElement;
  const filter = document.getElementById("jlpt-filter") as HTMLSelectElement;

  async function load(level: string): Promise<void> {
    const path = level
      ? `/api/v1/grammar?jlpt_level=${level}`
      : "/api/v1/grammar";
    try {
      const points = await apiFetch<GrammarPoint[]>(path);
      list.innerHTML = "";
      if (!points || points.length === 0) {
        empty.hidden = false;
        return;
      }
      empty.hidden = true;
      for (const p of points) {
        const li = document.createElement("li");
        li.className = "card-list__item";
        li.innerHTML = `
          <a class="card-list__link" href="/grammar/${p.id}">
            <span class="card-list__title">${p.name}</span>
            <span class="badge">${p.jlpt_level}</span>
            <span class="card-list__sub">${p.meaning}</span>
          </a>`;
        list.appendChild(li);
      }
    } catch (err) {
      list.innerHTML = `<li>エラー: ${(err as Error).message}</li>`;
    }
  }

  filter.addEventListener("change", () => load(filter.value));
  await load("");
}

// ── Detail page ───────────────────────────────────────────────────────────────
async function initDetail(): Promise<void> {
  const id = location.pathname.split("/").pop();
  if (!id) return;

  let gp: GrammarPoint;
  try {
    gp = await apiFetch<GrammarPoint>(`/api/v1/grammar/${id}`);
  } catch (err) {
    document.querySelector(".page")!.innerHTML = `<p>エラー: ${(err as Error).message}</p>`;
    return;
  }

  // Populate detail card
  const detail = document.getElementById("grammar-detail") as HTMLDivElement;
  (document.getElementById("gp-name-breadcrumb") as HTMLSpanElement).textContent = gp.name;
  (document.getElementById("gp-name") as HTMLHeadingElement).textContent = gp.name;
  (document.getElementById("gp-jlpt") as HTMLSpanElement).textContent = gp.jlpt_level;
  (document.getElementById("gp-meaning") as HTMLParagraphElement).textContent = gp.meaning;
  (document.getElementById("gp-conjunction") as HTMLParagraphElement).textContent = gp.conjunction_rule;
  (document.getElementById("gp-usage") as HTMLParagraphElement).textContent = gp.usage_note;

  const exList = document.getElementById("gp-examples") as HTMLOListElement;
  for (const ex of gp.examples ?? []) {
    const li = document.createElement("li");
    li.innerHTML = `<span lang="ja">${ex.japanese}</span><span class="reading">（${ex.reading}）</span> — ${ex.translation}`;
    exList.appendChild(li);
  }
  detail.hidden = false;

  // Quiz
  const startBtn = document.getElementById("start-quiz-btn") as HTMLButtonElement;
  startBtn.addEventListener("click", () => runQuiz(gp.quiz_questions ?? []));

  // Add to review queue
  const addBtn = document.getElementById("add-review-btn") as HTMLButtonElement;
  addBtn.addEventListener("click", async () => {
    try {
      await apiFetch(`/api/v1/grammar/${id}/review`, { method: "POST" });
      addBtn.textContent = "追加済み ✓";
      addBtn.disabled = true;
    } catch (err) {
      alert(`エラー: ${(err as Error).message}`);
    }
  });
}

function runQuiz(questions: QuizQuestion[]): void {
  const section = document.getElementById("quiz-section") as HTMLDivElement;
  const questionText = document.getElementById("quiz-question-text") as HTMLParagraphElement;
  const optionsList = document.getElementById("quiz-options") as HTMLUListElement;
  const feedback = document.getElementById("quiz-feedback") as HTMLDivElement;
  const nextBtn = document.getElementById("quiz-next-btn") as HTMLButtonElement;
  const result = document.getElementById("quiz-result") as HTMLDivElement;
  const resultText = document.getElementById("quiz-result-text") as HTMLParagraphElement;
  const retryBtn = document.getElementById("quiz-retry-btn") as HTMLButtonElement;

  section.hidden = false;
  document.getElementById("start-quiz-btn")!.remove();

  let qi = 0;
  let correct = 0;

  function showQuestion(index: number): void {
    const q = questions[index];
    questionText.textContent = q.question;
    optionsList.innerHTML = "";
    feedback.hidden = true;
    nextBtn.hidden = true;

    for (const opt of q.options) {
      const li = document.createElement("li");
      li.className = "quiz__option";
      const btn = document.createElement("button");
      btn.className = "btn btn--option";
      btn.textContent = opt.text;
      btn.addEventListener("click", () => {
        // disable all options
        optionsList.querySelectorAll("button").forEach(b => (b as HTMLButtonElement).disabled = true);

        if (opt.is_correct) {
          btn.classList.add("btn--option--correct");
          correct++;
        } else {
          btn.classList.add("btn--option--wrong");
        }
        feedback.textContent = opt.explanation;
        feedback.hidden = false;
        nextBtn.hidden = index < questions.length - 1 ? false : true;

        if (index === questions.length - 1) {
          // show result
          result.hidden = false;
          resultText.textContent = `${questions.length} 問中 ${correct} 問正解！`;
        }
      });
      li.appendChild(btn);
      optionsList.appendChild(li);
    }
  }

  nextBtn.addEventListener("click", () => {
    qi++;
    if (qi < questions.length) showQuestion(qi);
  });

  retryBtn.addEventListener("click", () => {
    qi = 0;
    correct = 0;
    result.hidden = true;
    showQuestion(0);
  });

  showQuestion(0);
}
