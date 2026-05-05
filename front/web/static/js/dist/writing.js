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

  // front/web/static/js/writing.ts
  var tabButtons = document.querySelectorAll(".tab-bar__tab");
  var progressBar = document.getElementById("writing-progress");
  var progressLabel = document.getElementById("writing-progress-label");
  var questionPanel = document.getElementById("question-panel");
  var promptEl = document.getElementById("question-prompt");
  var answerInput = document.getElementById("answer-input");
  var feedbackEl = document.getElementById("answer-feedback");
  var submitBtn = document.getElementById("submit-btn");
  var aiFeedbackPanel = document.getElementById("ai-feedback-panel");
  var aiCorrected = document.getElementById("ai-corrected");
  var aiExplanation = document.getElementById("ai-explanation");
  var aiScore = document.getElementById("ai-score");
  var nextQuestionBtn = document.getElementById("next-question-btn");
  var sessionSummary = document.getElementById("session-summary");
  var sessionAccuracy = document.getElementById("session-accuracy");
  var restartBtn = document.getElementById("restart-btn");
  var currentMode = "input";
  var queue = [];
  var currentIndex = 0;
  var correctCount = 0;
  async function loadQueue(mode) {
    questionPanel.hidden = true;
    aiFeedbackPanel.hidden = true;
    sessionSummary.hidden = true;
    try {
      queue = await apiFetch(`/api/v1/writing/queue?type=${mode}`);
    } catch (err) {
      promptEl.textContent = `\u30A8\u30E9\u30FC: ${err.message}`;
      questionPanel.hidden = false;
      return;
    }
    currentIndex = 0;
    correctCount = 0;
    showQuestion();
  }
  function updateProgress() {
    const total = queue.length;
    const done = currentIndex;
    const pct = total > 0 ? Math.round(done / total * 100) : 0;
    progressBar.style.width = `${pct}%`;
    progressBar.setAttribute("aria-valuenow", String(pct));
    progressLabel.textContent = `${done} / ${total}`;
  }
  function showQuestion() {
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
  function showSessionSummary() {
    questionPanel.hidden = true;
    aiFeedbackPanel.hidden = true;
    const accuracy = queue.length > 0 ? `${Math.round(correctCount / queue.length * 100)}%` : "0%";
    sessionAccuracy.textContent = accuracy;
    sessionSummary.hidden = false;
    updateProgress();
  }
  async function submitAnswer() {
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
  async function submitInput(q, answer) {
    try {
      const result = await apiFetch("/api/v1/writing/input", {
        method: "POST",
        body: JSON.stringify({ question_id: q.id, answer })
      });
      feedbackEl.hidden = false;
      if (result.correct) {
        feedbackEl.textContent = "\u2705 \u6B63\u89E3\uFF01";
        feedbackEl.className = "question-panel__feedback question-panel__feedback--correct";
        correctCount++;
      } else {
        feedbackEl.textContent = `\u274C \u4E0D\u6B63\u89E3\u3002\u6B63\u89E3: ${result.expected_answer ?? ""}`;
        feedbackEl.className = "question-panel__feedback question-panel__feedback--wrong";
      }
      setTimeout(() => {
        currentIndex++;
        showQuestion();
      }, 1200);
    } catch (err) {
      feedbackEl.textContent = `\u30A8\u30E9\u30FC: ${err.message}`;
      feedbackEl.hidden = false;
      submitBtn.disabled = false;
    }
  }
  async function submitSentence(q, answer) {
    try {
      const result = await apiFetch("/api/v1/writing/sentence", {
        method: "POST",
        body: JSON.stringify({ question_id: q.id, answer })
      });
      questionPanel.hidden = true;
      aiCorrected.textContent = result.ai_feedback.corrected;
      aiExplanation.textContent = result.ai_feedback.explanation;
      aiScore.textContent = String(result.score);
      aiFeedbackPanel.hidden = false;
      if (result.score >= 70) correctCount++;
    } catch (err) {
      feedbackEl.textContent = `\u30A8\u30E9\u30FC: ${err.message}`;
      feedbackEl.hidden = false;
      submitBtn.disabled = false;
    }
  }
  submitBtn.addEventListener("click", submitAnswer);
  answerInput.addEventListener("keydown", (e) => {
    if (e.key === "Enter") submitAnswer();
  });
  nextQuestionBtn.addEventListener("click", () => {
    currentIndex++;
    showQuestion();
  });
  restartBtn.addEventListener("click", () => loadQueue(currentMode));
  tabButtons.forEach((btn) => {
    btn.addEventListener("click", () => {
      tabButtons.forEach((b) => b.classList.remove("tab-bar__tab--active"));
      btn.classList.add("tab-bar__tab--active");
      currentMode = btn.dataset["mode"];
      loadQueue(currentMode);
    });
  });
  loadQueue(currentMode);
})();
