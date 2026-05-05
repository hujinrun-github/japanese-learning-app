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

  // front/web/static/js/summary.ts
  var summaryList = document.getElementById("summary-list");
  var summaryEmpty = document.getElementById("summary-empty");
  var summaryDetail = document.getElementById("summary-detail");
  var summaryDetailTitle = document.getElementById("summary-detail-title");
  var summaryScoreBlock = document.getElementById("summary-score-block");
  var summaryStrengths = document.getElementById("summary-strengths");
  var summaryWeaknesses = document.getElementById("summary-weaknesses");
  var summarySuggestions = document.getElementById("summary-suggestions");
  var backBtn = document.getElementById("summary-back-btn");
  async function init() {
    try {
      const summaries = await apiFetch("/api/v1/summary");
      if (!summaries || summaries.length === 0) {
        summaryEmpty.hidden = false;
        return;
      }
      renderList(summaries);
    } catch (err) {
      summaryList.innerHTML = `<li>\u30A8\u30E9\u30FC: ${err.message}</li>`;
    }
  }
  function renderList(summaries) {
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
      li.querySelector("button").addEventListener("click", () => showDetail(s));
      summaryList.appendChild(li);
    }
  }
  function moduleLabel(module) {
    const labels = {
      word: "\u5358\u8A9E",
      grammar: "\u6587\u6CD5",
      lesson: "\u8AB2\u6587",
      speaking: "\u53E3\u8A9E",
      writing: "\u30E9\u30A4\u30C6\u30A3\u30F3\u30B0"
    };
    return labels[module] ?? module;
  }
  function showDetail(s) {
    summaryList.hidden = true;
    summaryEmpty.hidden = true;
    summaryDetailTitle.textContent = `${moduleLabel(s.module)} \u2014 ${new Date(s.generated_at).toLocaleDateString("ja-JP")}`;
    summaryScoreBlock.innerHTML = `
    <p>${s.score_summary.correct} / ${s.score_summary.total} \u6B63\u89E3 (${s.score_summary.accuracy_pct}%)</p>`;
    renderStringList(summaryStrengths, s.strengths);
    renderStringList(summaryWeaknesses, s.weaknesses);
    renderStringList(summarySuggestions, s.suggestions);
    summaryDetail.hidden = false;
  }
  function renderStringList(el, items) {
    el.innerHTML = "";
    if (!items || items.length === 0) {
      el.innerHTML = "<li>\u30C7\u30FC\u30BF\u306A\u3057</li>";
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
  init();
})();
