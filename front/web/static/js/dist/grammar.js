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

  // front/web/static/js/grammar.ts
  var isDetail = document.getElementById("grammar-detail") !== null;
  if (isDetail) {
    initDetail();
  } else {
    initList();
  }
  async function initList() {
    const list = document.getElementById("grammar-list");
    const empty = document.getElementById("grammar-empty");
    const filter = document.getElementById("jlpt-filter");
    async function load(level) {
      const path = level ? `/api/v1/grammar?jlpt_level=${level}` : "/api/v1/grammar";
      try {
        const points = await apiFetch(path);
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
        list.innerHTML = `<li>\u30A8\u30E9\u30FC: ${err.message}</li>`;
      }
    }
    filter.addEventListener("change", () => load(filter.value));
    await load("");
  }
  async function initDetail() {
    const id = location.pathname.split("/").pop();
    if (!id) return;
    let gp;
    try {
      gp = await apiFetch(`/api/v1/grammar/${id}`);
    } catch (err) {
      document.querySelector(".page").innerHTML = `<p>\u30A8\u30E9\u30FC: ${err.message}</p>`;
      return;
    }
    const detail = document.getElementById("grammar-detail");
    document.getElementById("gp-name-breadcrumb").textContent = gp.name;
    document.getElementById("gp-name").textContent = gp.name;
    document.getElementById("gp-jlpt").textContent = gp.jlpt_level;
    document.getElementById("gp-meaning").textContent = gp.meaning;
    document.getElementById("gp-conjunction").textContent = gp.conjunction_rule;
    document.getElementById("gp-usage").textContent = gp.usage_note;
    const exList = document.getElementById("gp-examples");
    for (const ex of gp.examples ?? []) {
      const li = document.createElement("li");
      li.innerHTML = `<span lang="ja">${ex.japanese}</span><span class="reading">\uFF08${ex.reading}\uFF09</span> \u2014 ${ex.translation}`;
      exList.appendChild(li);
    }
    detail.hidden = false;
    const startBtn = document.getElementById("start-quiz-btn");
    startBtn.addEventListener("click", () => runQuiz(gp.quiz_questions ?? []));
    const addBtn = document.getElementById("add-review-btn");
    addBtn.addEventListener("click", async () => {
      try {
        await apiFetch(`/api/v1/grammar/${id}/review`, { method: "POST" });
        addBtn.textContent = "\u8FFD\u52A0\u6E08\u307F \u2713";
        addBtn.disabled = true;
      } catch (err) {
        alert(`\u30A8\u30E9\u30FC: ${err.message}`);
      }
    });
  }
  function runQuiz(questions) {
    const section = document.getElementById("quiz-section");
    const questionText = document.getElementById("quiz-question-text");
    const optionsList = document.getElementById("quiz-options");
    const feedback = document.getElementById("quiz-feedback");
    const nextBtn = document.getElementById("quiz-next-btn");
    const result = document.getElementById("quiz-result");
    const resultText = document.getElementById("quiz-result-text");
    const retryBtn = document.getElementById("quiz-retry-btn");
    section.hidden = false;
    document.getElementById("start-quiz-btn").remove();
    let qi = 0;
    let correct = 0;
    function showQuestion(index) {
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
          optionsList.querySelectorAll("button").forEach((b) => b.disabled = true);
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
            result.hidden = false;
            resultText.textContent = `${questions.length} \u554F\u4E2D ${correct} \u554F\u6B63\u89E3\uFF01`;
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
})();
