/**
 * speaking.ts — MediaRecorder, multipart audio upload, score rendering.
 */

import { apiFetch, getToken } from "./api.js";

interface SpeakingMaterial {
  id: number;
  type: "shadow" | "free";
  title: string;
  text: string;
  audio_url: string;
  jlpt_level: string;
}

interface ScoreResult {
  score: number;
  detail: string;
}

// ── DOM refs ─────────────────────────────────────────────────────────────────
const materialList = document.getElementById("speaking-material-list") as HTMLUListElement;
const practicePanel = document.getElementById("practice-panel") as HTMLDivElement;
const practiceTitle = document.getElementById("practice-title") as HTMLHeadingElement;
const practiceText = document.getElementById("practice-text") as HTMLParagraphElement;
const referenceBlock = document.getElementById("reference-audio-block") as HTMLDivElement;
const referenceAudio = document.getElementById("reference-audio") as HTMLAudioElement;
const recordBtn = document.getElementById("record-btn") as HTMLButtonElement;
const recordStatus = document.getElementById("record-status") as HTMLSpanElement;
const scoreResult = document.getElementById("score-result") as HTMLDivElement;
const scoreValue = document.getElementById("score-value") as HTMLParagraphElement;
const scoreDetail = document.getElementById("score-detail") as HTMLDivElement;
const retryBtn = document.getElementById("retry-btn") as HTMLButtonElement;
const nextBtn = document.getElementById("next-material-btn") as HTMLButtonElement;
const tabButtons = document.querySelectorAll<HTMLButtonElement>(".tab-bar__tab");

// ── State ────────────────────────────────────────────────────────────────────
let currentMode: "shadow" | "free" = "shadow";
let currentMaterial: SpeakingMaterial | null = null;
let materials: SpeakingMaterial[] = [];
let mediaRecorder: MediaRecorder | null = null;
let chunks: Blob[] = [];
let recording = false;

// ── Init ─────────────────────────────────────────────────────────────────────
async function loadMaterials(mode: "shadow" | "free"): Promise<void> {
  materialList.innerHTML = "<li>読み込み中…</li>";
  practicePanel.hidden = true;
  try {
    materials = await apiFetch<SpeakingMaterial[]>(`/api/v1/speaking/materials?type=${mode}`);
    renderMaterialList();
  } catch (err) {
    materialList.innerHTML = `<li>エラー: ${(err as Error).message}</li>`;
  }
}

function renderMaterialList(): void {
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

function selectMaterial(m: SpeakingMaterial): void {
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
  recordStatus.textContent = "待機中";
  practicePanel.hidden = false;
}

// ── Recording ─────────────────────────────────────────────────────────────────
async function startRecording(): Promise<void> {
  const stream = await navigator.mediaDevices.getUserMedia({ audio: true });
  mediaRecorder = new MediaRecorder(stream);
  chunks = [];

  mediaRecorder.ondataavailable = (e: BlobEvent) => {
    if (e.data.size > 0) chunks.push(e.data);
  };

  mediaRecorder.onstop = () => {
    stream.getTracks().forEach(t => t.stop());
    const blob = new Blob(chunks, { type: mediaRecorder!.mimeType || "audio/webm" });
    uploadRecording(blob);
  };

  mediaRecorder.start();
  recording = true;
  recordBtn.textContent = "録音停止";
  recordStatus.textContent = "録音中…";
}

function stopRecording(): void {
  if (mediaRecorder && mediaRecorder.state !== "inactive") {
    mediaRecorder.stop();
  }
  recording = false;
  recordBtn.textContent = "録音開始";
  recordStatus.textContent = "アップロード中…";
  recordBtn.disabled = true;
}

async function uploadRecording(blob: Blob): Promise<void> {
  if (!currentMaterial) return;

  const formData = new FormData();
  formData.append("audio", blob, "recording.webm");
  formData.append("material_id", String(currentMaterial.id));
  formData.append("type", currentMaterial.type);

  const headers: HeadersInit = {};
  const token = getToken();
  if (token) headers["Authorization"] = `Bearer ${token}`;

  try {
    const response = await fetch("/api/v1/speaking/score", {
      method: "POST",
      headers,
      body: formData,
    });
    if (!response.ok) throw new Error(`HTTP ${response.status}`);
    const body = await response.json() as { data: ScoreResult };
    showScore(body.data);
  } catch (err) {
    recordStatus.textContent = `エラー: ${(err as Error).message}`;
    recordBtn.disabled = false;
  }
}

function showScore(result: ScoreResult): void {
  scoreValue.textContent = String(result.score);
  scoreDetail.textContent = result.detail;
  scoreResult.hidden = false;
  recordBtn.disabled = false;
  recordStatus.textContent = "完了";
}

// ── Event listeners ──────────────────────────────────────────────────────────
recordBtn.addEventListener("click", () => {
  if (recording) {
    stopRecording();
  } else {
    startRecording().catch(err => {
      recordStatus.textContent = `マイクエラー: ${(err as Error).message}`;
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

tabButtons.forEach(btn => {
  btn.addEventListener("click", () => {
    tabButtons.forEach(b => b.classList.remove("tab-bar__tab--active"));
    btn.classList.add("tab-bar__tab--active");
    currentMode = btn.dataset["mode"] as "shadow" | "free";
    loadMaterials(currentMode);
  });
});

// ── Bootstrap ────────────────────────────────────────────────────────────────
loadMaterials(currentMode);
