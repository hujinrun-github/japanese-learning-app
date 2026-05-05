(() => {
  // front/web/static/js/api.ts
  var TOKEN_KEY = "jla_token";
  function getToken() {
    return localStorage.getItem(TOKEN_KEY);
  }
  async function apiFetch(path, init = {}) {
    const headers = {
      "Content-Type": "application/json",
      ...init.headers
    };
    const token = getToken();
    if (token) {
      headers["Authorization"] = `Bearer ${token}`;
    }
    const response = await fetch(path, { ...init, headers });
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

  // front/web/static/js/lesson.ts
  var isDetail = document.getElementById("lesson-text") !== null;
  if (isDetail) {
    initDetail();
  } else {
    initList();
  }
  async function initList() {
    const list = document.getElementById("lesson-list");
    const empty = document.getElementById("lesson-empty");
    const filter = document.getElementById("lesson-jlpt-filter");
    async function load(level) {
      const path = level ? `/api/v1/lessons?jlpt_level=${level}` : "/api/v1/lessons";
      try {
        const lessons = await apiFetch(path);
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
            <span class="card-list__sub">${l.char_count} \u6587\u5B57</span>
          </a>`;
          list.appendChild(li);
        }
      } catch (err) {
        list.innerHTML = `<li>\u30A8\u30E9\u30FC: ${err.message}</li>`;
      }
    }
    filter.addEventListener("change", () => load(filter.value));
    await load("");
  }
  async function initDetail() {
    const id = location.pathname.split("/").pop();
    if (!id) return;
    let lesson;
    try {
      lesson = await apiFetch(`/api/v1/lessons/${id}`);
    } catch (err) {
      document.querySelector(".page").innerHTML = `<p>\u30A8\u30E9\u30FC: ${err.message}</p>`;
      return;
    }
    document.getElementById("lesson-title-breadcrumb").textContent = lesson.title;
    document.getElementById("lesson-title").textContent = lesson.title;
    document.getElementById("lesson-jlpt").textContent = lesson.jlpt_level;
    const textEl = document.getElementById("lesson-text");
    for (const s of lesson.sentences ?? []) {
      const p = document.createElement("p");
      p.className = "lesson-sentence";
      p.dataset["sentenceId"] = String(s.id);
      p.innerHTML = s.furigana_html;
      textEl.appendChild(p);
    }
    if (lesson.audio_url) {
      const audioBlock = document.getElementById("audio-player");
      const audio = document.getElementById("lesson-audio");
      audio.src = lesson.audio_url;
      audioBlock.hidden = false;
      const autoScroll = document.getElementById("auto-scroll-toggle");
      const timestamps = lesson.sentence_timestamps ?? [];
      audio.addEventListener("timeupdate", () => {
        const t = audio.currentTime;
        const current = timestamps.find((ts) => t >= ts.start_sec && t < ts.end_sec);
        if (!current) return;
        textEl.querySelectorAll(".lesson-sentence--active").forEach(
          (el) => el.classList.remove("lesson-sentence--active")
        );
        const activeSentence = textEl.querySelector(
          `[data-sentence-id="${current.sentence_id}"]`
        );
        if (activeSentence) {
          activeSentence.classList.add("lesson-sentence--active");
          if (autoScroll.checked) {
            activeSentence.scrollIntoView({ behavior: "smooth", block: "center" });
          }
        }
      });
    }
    const popup = document.getElementById("word-popup");
    textEl.addEventListener("click", async (e) => {
      const target = e.target;
      const wordEl = target.closest("[data-word-id]");
      if (!wordEl) {
        popup.hidden = true;
        return;
      }
      const wordId = wordEl.dataset["wordId"];
      try {
        const word = await apiFetch(
          `/api/v1/words/${wordId}`
        );
        document.getElementById("popup-kanji").textContent = word.kanji_form;
        document.getElementById("popup-reading").textContent = word.reading;
        document.getElementById("popup-meaning").textContent = word.meaning;
        const rect = wordEl.getBoundingClientRect();
        popup.style.top = `${rect.bottom + window.scrollY + 4}px`;
        popup.style.left = `${rect.left + window.scrollX}px`;
        popup.hidden = false;
        const addBtn = document.getElementById("popup-add-bookmark");
        addBtn.onclick = async () => {
          try {
            await apiFetch(`/api/v1/words/${wordId}/bookmark`, { method: "POST" });
            addBtn.textContent = "\u8FFD\u52A0\u6E08\u307F \u2713";
            addBtn.disabled = true;
          } catch (err) {
            alert(`\u30A8\u30E9\u30FC: ${err.message}`);
          }
        };
      } catch (err) {
        console.error("word fetch failed", err);
      }
    });
    document.addEventListener("click", (e) => {
      if (!e.target.closest("#word-popup, [data-word-id]")) {
        popup.hidden = true;
      }
    });
  }
})();
