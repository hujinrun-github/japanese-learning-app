(() => {
  // front/web/static/js/api.ts
  var TOKEN_KEY = "jla_token";
  function getToken() {
    return localStorage.getItem(TOKEN_KEY);
  }
  async function apiFetch(path, init2 = {}) {
    const headers = {
      "Content-Type": "application/json",
      ...init2.headers
    };
    const token = getToken();
    if (token) {
      headers["Authorization"] = `Bearer ${token}`;
    }
    const response = await fetch(path, { ...init2, headers });
    if (!response.ok) {
      let message = `HTTP ${response.status}`;
      try {
        const body2 = await response.json();
        if (body2.error?.message) {
          message = body2.error.message;
        }
      } catch {
      }
      throw new Error(message);
    }
    const body = await response.json();
    return body.data;
  }

  // front/web/static/js/word.ts
  var flashcard = document.getElementById("flashcard");
  var flashcardInner = document.getElementById("flashcard-inner");
  var ratingButtons = document.getElementById("rating-buttons");
  var emptyState = document.getElementById("empty-state");
  var queueInfo = document.getElementById("word-queue-info");
  var cardKanji = document.getElementById("card-kanji");
  var cardJlpt = document.getElementById("card-jlpt");
  var cardReading = document.getElementById("card-reading");
  var cardPos = document.getElementById("card-pos");
  var cardMeaning = document.getElementById("card-meaning");
  var cardExamples = document.getElementById("card-examples");
  var queue = [];
  var currentIndex = 0;
  var flipped = false;
  async function init() {
    try {
      queue = await apiFetch("/api/v1/words/queue");
    } catch (err) {
      queueInfo.textContent = `\u30A8\u30E9\u30FC: ${err.message}`;
      return;
    }
    if (queue.length === 0) {
      showEmpty();
      return;
    }
    queueInfo.textContent = `\u4ECA\u65E5\u306E\u30AB\u30FC\u30C9: ${queue.length} \u679A`;
    showCard(0);
  }
  function showCard(index) {
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
      li.textContent = `${ex.japanese}\uFF08${ex.reading}\uFF09\u2014 ${ex.translation}`;
      cardExamples.appendChild(li);
    }
  }
  function showEmpty() {
    flashcard.hidden = true;
    ratingButtons.hidden = true;
    emptyState.hidden = false;
  }
  function flipCard() {
    if (flipped) return;
    flipped = true;
    flashcardInner.classList.add("is-flipped");
    ratingButtons.hidden = false;
  }
  async function submitRating(rating) {
    const card = queue[currentIndex];
    try {
      await apiFetch("/api/v1/words/review", {
        method: "POST",
        body: JSON.stringify({ word_id: card.word_id, rating })
      });
    } catch (err) {
      console.error("review submit failed", err);
    }
    currentIndex++;
    if (currentIndex >= queue.length) {
      showEmpty();
    } else {
      queueInfo.textContent = `\u4ECA\u65E5\u306E\u30AB\u30FC\u30C9: ${queue.length - currentIndex} \u679A\u6B8B\u308A`;
      showCard(currentIndex);
    }
  }
  flashcard.addEventListener("click", flipCard);
  flashcard.addEventListener("keydown", (e) => {
    if (e.key === "Enter" || e.key === " ") flipCard();
  });
  ratingButtons.addEventListener("click", (e) => {
    const btn = e.target.closest("[data-rating]");
    if (!btn) return;
    const rating = parseInt(btn.dataset["rating"] ?? "0", 10);
    if (rating) submitRating(rating);
  });
  init();
})();
