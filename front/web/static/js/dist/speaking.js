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

  // front/web/static/js/speaking.ts
  var materialList = document.getElementById("speaking-material-list");
  var practicePanel = document.getElementById("practice-panel");
  var practiceTitle = document.getElementById("practice-title");
  var practiceText = document.getElementById("practice-text");
  var referenceBlock = document.getElementById("reference-audio-block");
  var referenceAudio = document.getElementById("reference-audio");
  var recordBtn = document.getElementById("record-btn");
  var recordStatus = document.getElementById("record-status");
  var scoreResult = document.getElementById("score-result");
  var scoreValue = document.getElementById("score-value");
  var scoreDetail = document.getElementById("score-detail");
  var retryBtn = document.getElementById("retry-btn");
  var nextBtn = document.getElementById("next-material-btn");
  var tabButtons = document.querySelectorAll(".tab-bar__tab");
  var currentMode = "shadow";
  var currentMaterial = null;
  var materials = [];
  var mediaRecorder = null;
  var chunks = [];
  var recording = false;
  async function loadMaterials(mode) {
    materialList.innerHTML = "<li>\u8AAD\u307F\u8FBC\u307F\u4E2D\u2026</li>";
    practicePanel.hidden = true;
    try {
      materials = await apiFetch(`/api/v1/speaking/materials?type=${mode}`);
      renderMaterialList();
    } catch (err) {
      materialList.innerHTML = `<li>\u30A8\u30E9\u30FC: ${err.message}</li>`;
    }
  }
  function renderMaterialList() {
    materialList.innerHTML = "";
    for (const m of materials) {
      const li = document.createElement("li");
      li.className = "card-list__item";
      const btn = document.createElement("button");
      btn.className = "card-list__link btn--reset";
      btn.innerHTML = `
      <span class="card-list__title">${m.title}</span>
      <span class="badge">${m.jlpt_level}</span>`;
      btn.addEventListener("click", () => selectMaterial(m));
      li.appendChild(btn);
      materialList.appendChild(li);
    }
  }
  function selectMaterial(m) {
    currentMaterial = m;
    practiceTitle.textContent = m.title;
    practiceText.textContent = m.text;
    if (m.type === "shadow" && m.audio_url) {
      referenceAudio.src = m.audio_url;
      referenceBlock.hidden = false;
    } else {
      referenceBlock.hidden = true;
    }
    scoreResult.hidden = true;
    recordBtn.disabled = false;
    recordStatus.textContent = "\u5F85\u6A5F\u4E2D";
    practicePanel.hidden = false;
  }
  async function startRecording() {
    const stream = await navigator.mediaDevices.getUserMedia({ audio: true });
    mediaRecorder = new MediaRecorder(stream);
    chunks = [];
    mediaRecorder.ondataavailable = (e) => {
      if (e.data.size > 0) chunks.push(e.data);
    };
    mediaRecorder.onstop = () => {
      stream.getTracks().forEach((t) => t.stop());
      const blob = new Blob(chunks, { type: mediaRecorder.mimeType || "audio/webm" });
      uploadRecording(blob);
    };
    mediaRecorder.start();
    recording = true;
    recordBtn.textContent = "\u9332\u97F3\u505C\u6B62";
    recordStatus.textContent = "\u9332\u97F3\u4E2D\u2026";
  }
  function stopRecording() {
    if (mediaRecorder && mediaRecorder.state !== "inactive") {
      mediaRecorder.stop();
    }
    recording = false;
    recordBtn.textContent = "\u9332\u97F3\u958B\u59CB";
    recordStatus.textContent = "\u30A2\u30C3\u30D7\u30ED\u30FC\u30C9\u4E2D\u2026";
    recordBtn.disabled = true;
  }
  async function uploadRecording(blob) {
    if (!currentMaterial) return;
    const formData = new FormData();
    formData.append("audio", blob, "recording.webm");
    formData.append("material_id", String(currentMaterial.id));
    formData.append("type", currentMaterial.type);
    const headers = {};
    const token = getToken();
    if (token) headers["Authorization"] = `Bearer ${token}`;
    try {
      const response = await fetch("/api/v1/speaking/score", {
        method: "POST",
        headers,
        body: formData
      });
      if (!response.ok) throw new Error(`HTTP ${response.status}`);
      const body = await response.json();
      showScore(body.data);
    } catch (err) {
      recordStatus.textContent = `\u30A8\u30E9\u30FC: ${err.message}`;
      recordBtn.disabled = false;
    }
  }
  function showScore(result) {
    scoreValue.textContent = String(result.score);
    scoreDetail.textContent = result.detail;
    scoreResult.hidden = false;
    recordBtn.disabled = false;
    recordStatus.textContent = "\u5B8C\u4E86";
  }
  recordBtn.addEventListener("click", () => {
    if (recording) {
      stopRecording();
    } else {
      startRecording().catch((err) => {
        recordStatus.textContent = `\u30DE\u30A4\u30AF\u30A8\u30E9\u30FC: ${err.message}`;
      });
    }
  });
  retryBtn.addEventListener("click", () => {
    if (!currentMaterial) return;
    selectMaterial(currentMaterial);
  });
  nextBtn.addEventListener("click", () => {
    practicePanel.hidden = true;
    currentMaterial = null;
  });
  tabButtons.forEach((btn) => {
    btn.addEventListener("click", () => {
      tabButtons.forEach((b) => b.classList.remove("tab-bar__tab--active"));
      btn.classList.add("tab-bar__tab--active");
      currentMode = btn.dataset["mode"];
      loadMaterials(currentMode);
    });
  });
  loadMaterials(currentMode);
})();
