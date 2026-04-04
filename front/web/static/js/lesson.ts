/**
 * lesson.ts — Lesson list + detail reader with audio sync highlight and word popup.
 */

import { apiFetch } from "./api.js";

interface SentenceTimestamp {
  sentence_id: number;
  start_sec: number;
  end_sec: number;
}

interface LessonSentence {
  id: number;
  furigana_html: string; // pre-rendered <ruby> HTML
}

interface LessonDetail {
  id: number;
  title: string;
  jlpt_level: string;
  audio_url: string;
  sentences: LessonSentence[];
  sentence_timestamps: SentenceTimestamp[];
}

interface LessonSummary {
  id: number;
  title: string;
  jlpt_level: string;
  char_count: number;
}

// ── Page detection ────────────────────────────────────────────────────────────
const isDetail = document.getElementById("lesson-text") !== null;

if (isDetail) {
  initDetail();
} else {
  initList();
}

// ── List page ─────────────────────────────────────────────────────────────────
async function initList(): Promise<void> {
  const list = document.getElementById("lesson-list") as HTMLUListElement;
  const empty = document.getElementById("lesson-empty") as HTMLDivElement;
  const filter = document.getElementById("lesson-jlpt-filter") as HTMLSelectElement;

  async function load(level: string): Promise<void> {
    const path = level
      ? `/api/v1/lessons?jlpt_level=${level}`
      : "/api/v1/lessons";
    try {
      const lessons = await apiFetch<LessonSummary[]>(path);
      list.innerHTML = "";
      if (!lessons || lessons.length === 0) {
        empty.hidden = false;
        return;
      }
      empty.hidden = true;
      for (const l of lessons) {
        const li = document.createElement("li");
        li.className = "card-list__item";
        li.innerHTML = `
          <a class="card-list__link" href="/lesson/${l.id}">
            <span class="card-list__title">${l.title}</span>
            <span class="badge">${l.jlpt_level}</span>
            <span class="card-list__sub">${l.char_count} 文字</span>
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

  let lesson: LessonDetail;
  try {
    lesson = await apiFetch<LessonDetail>(`/api/v1/lessons/${id}`);
  } catch (err) {
    document.querySelector(".page")!.innerHTML = `<p>エラー: ${(err as Error).message}</p>`;
    return;
  }

  // Fill header
  (document.getElementById("lesson-title-breadcrumb") as HTMLSpanElement).textContent = lesson.title;
  (document.getElementById("lesson-title") as HTMLHeadingElement).textContent = lesson.title;
  (document.getElementById("lesson-jlpt") as HTMLSpanElement).textContent = lesson.jlpt_level;

  // Build lesson text
  const textEl = document.getElementById("lesson-text") as HTMLDivElement;
  for (const s of lesson.sentences ?? []) {
    const p = document.createElement("p");
    p.className = "lesson-sentence";
    p.dataset["sentenceId"] = String(s.id);
    p.innerHTML = s.furigana_html;
    textEl.appendChild(p);
  }

  // Audio player
  if (lesson.audio_url) {
    const audioBlock = document.getElementById("audio-player") as HTMLDivElement;
    const audio = document.getElementById("lesson-audio") as HTMLAudioElement;
    audio.src = lesson.audio_url;
    audioBlock.hidden = false;

    const autoScroll = document.getElementById("auto-scroll-toggle") as HTMLInputElement;
    const timestamps = lesson.sentence_timestamps ?? [];

    audio.addEventListener("timeupdate", () => {
      const t = audio.currentTime;
      const current = timestamps.find(ts => t >= ts.start_sec && t < ts.end_sec);
      if (!current) return;

      // Remove highlight from all sentences
      textEl.querySelectorAll(".lesson-sentence--active").forEach(el =>
        el.classList.remove("lesson-sentence--active")
      );

      const activeSentence = textEl.querySelector(
        `[data-sentence-id="${current.sentence_id}"]`
      ) as HTMLElement | null;
      if (activeSentence) {
        activeSentence.classList.add("lesson-sentence--active");
        if (autoScroll.checked) {
          activeSentence.scrollIntoView({ behavior: "smooth", block: "center" });
        }
      }
    });
  }

  // Word popup on click
  const popup = document.getElementById("word-popup") as HTMLDivElement;
  textEl.addEventListener("click", async (e: Event) => {
    const target = e.target as HTMLElement;
    const wordEl = target.closest("[data-word-id]") as HTMLElement | null;
    if (!wordEl) {
      popup.hidden = true;
      return;
    }
    const wordId = wordEl.dataset["wordId"];
    try {
      const word = await apiFetch<{ kanji_form: string; reading: string; meaning: string }>(
        `/api/v1/words/${wordId}`
      );
      (document.getElementById("popup-kanji") as HTMLParagraphElement).textContent = word.kanji_form;
      (document.getElementById("popup-reading") as HTMLParagraphElement).textContent = word.reading;
      (document.getElementById("popup-meaning") as HTMLParagraphElement).textContent = word.meaning;

      const rect = wordEl.getBoundingClientRect();
      popup.style.top = `${rect.bottom + window.scrollY + 4}px`;
      popup.style.left = `${rect.left + window.scrollX}px`;
      popup.hidden = false;

      const addBtn = document.getElementById("popup-add-bookmark") as HTMLButtonElement;
      addBtn.onclick = async () => {
        try {
          await apiFetch(`/api/v1/words/${wordId}/bookmark`, { method: "POST" });
          addBtn.textContent = "追加済み ✓";
          addBtn.disabled = true;
        } catch (err) {
          alert(`エラー: ${(err as Error).message}`);
        }
      };
    } catch (err) {
      console.error("word fetch failed", err);
    }
  });

  document.addEventListener("click", (e: Event) => {
    if (!(e.target as HTMLElement).closest("#word-popup, [data-word-id]")) {
      popup.hidden = true;
    }
  });
}
