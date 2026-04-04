/**
 * word.ts — Flashcard review: fetch queue, flip card, submit rating.
 */

import { apiFetch } from "./api.js";

interface WordExample {
  japanese: string;
  reading: string;
  translation: string;
}

interface WordCard {
  word_id: number;
  kanji_form: string;
  reading: string;
  part_of_speech: string;
  meaning: string;
  examples: WordExample[];
  jlpt_level: string;
}

interface ReviewRequest {
  word_id: number;
  rating: number; // 1-4
}

// ── DOM refs ────────────────────────────────────────────────────────────────
const flashcard = document.getElementById("flashcard") as HTMLDivElement;
const flashcardInner = document.getElementById("flashcard-inner") as HTMLDivElement;
const ratingButtons = document.getElementById("rating-buttons") as HTMLDivElement;
const emptyState = document.getElementById("empty-state") as HTMLDivElement;
const queueInfo = document.getElementById("word-queue-info") as HTMLParagraphElement;

const cardKanji = document.getElementById("card-kanji") as HTMLParagraphElement;
const cardJlpt = document.getElementById("card-jlpt") as HTMLParagraphElement;
const cardReading = document.getElementById("card-reading") as HTMLParagraphElement;
const cardPos = document.getElementById("card-pos") as HTMLParagraphElement;
const cardMeaning = document.getElementById("card-meaning") as HTMLParagraphElement;
const cardExamples = document.getElementById("card-examples") as HTMLUListElement;

// ── State ────────────────────────────────────────────────────────────────────
let queue: WordCard[] = [];
let currentIndex = 0;
let flipped = false;

// ── Init ─────────────────────────────────────────────────────────────────────
async function init(): Promise<void> {
  try {
    queue = await apiFetch<WordCard[]>("/api/v1/words/queue");
  } catch (err) {
    queueInfo.textContent = `エラー: ${(err as Error).message}`;
    return;
  }

  if (queue.length === 0) {
    showEmpty();
    return;
  }

  queueInfo.textContent = `今日のカード: ${queue.length} 枚`;
  showCard(0);
}

// ── Helpers ──────────────────────────────────────────────────────────────────
function showCard(index: number): void {
  const card = queue[index];
  flipped = false;
  flashcardInner.classList.remove("is-flipped");
  ratingButtons.hidden = true;

  cardKanji.textContent = card.kanji_form;
  cardJlpt.textContent = card.jlpt_level;
  cardReading.textContent = card.reading;
  cardPos.textContent = card.part_of_speech;
  cardMeaning.textContent = card.meaning;

  cardExamples.innerHTML = "";
  for (const ex of card.examples ?? []) {
    const li = document.createElement("li");
    li.textContent = `${ex.japanese}（${ex.reading}）— ${ex.translation}`;
    cardExamples.appendChild(li);
  }
}

function showEmpty(): void {
  flashcard.hidden = true;
  ratingButtons.hidden = true;
  emptyState.hidden = false;
}

function flipCard(): void {
  if (flipped) return;
  flipped = true;
  flashcardInner.classList.add("is-flipped");
  ratingButtons.hidden = false;
}

async function submitRating(rating: number): Promise<void> {
  const card = queue[currentIndex];
  try {
    await apiFetch<void>("/api/v1/words/review", {
      method: "POST",
      body: JSON.stringify({ word_id: card.word_id, rating } as ReviewRequest),
    });
  } catch (err) {
    console.error("review submit failed", err);
    // non-fatal: continue to next card
  }

  currentIndex++;
  if (currentIndex >= queue.length) {
    showEmpty();
  } else {
    queueInfo.textContent = `今日のカード: ${queue.length - currentIndex} 枚残り`;
    showCard(currentIndex);
  }
}

// ── Event listeners ──────────────────────────────────────────────────────────
flashcard.addEventListener("click", flipCard);
flashcard.addEventListener("keydown", (e: KeyboardEvent) => {
  if (e.key === "Enter" || e.key === " ") flipCard();
});

ratingButtons.addEventListener("click", (e: Event) => {
  const btn = (e.target as HTMLElement).closest("[data-rating]");
  if (!btn) return;
  const rating = parseInt((btn as HTMLElement).dataset["rating"] ?? "0", 10);
  if (rating) submitRating(rating);
});

// ── Bootstrap ────────────────────────────────────────────────────────────────
init();
