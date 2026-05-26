const state = {
  mode: "practice",
  session: null,
  stream: null,
  lastEventId: "",
  busy: false,
};

const $ = (id) => document.getElementById(id);

const els = {
  healthDot: $("healthDot"),
  healthText: $("healthText"),
  streamState: $("streamState"),
  userId: $("userId"),
  jdText: $("jdText"),
  resumeText: $("resumeText"),
  startBtn: $("startBtn"),
  resetBtn: $("resetBtn"),
  refreshSessions: $("refreshSessions"),
  sessionList: $("sessionList"),
  practiceModeBtn: $("practiceModeBtn"),
  examModeBtn: $("examModeBtn"),
  modeDescription: $("modeDescription"),
  setupView: $("setupView"),
  interviewView: $("interviewView"),
  reportView: $("reportView"),
  progressBar: $("progressBar"),
  sessionMeta: $("sessionMeta"),
  sessionTitle: $("sessionTitle"),
  statusPill: $("statusPill"),
  conversation: $("conversation"),
  answerForm: $("answerForm"),
  answerText: $("answerText"),
  answerBtn: $("answerBtn"),
  endInterviewBtn: $("endInterviewBtn"),
  newInterviewBtn: $("newInterviewBtn"),
  backToSetupBtn: $("backToSetupBtn"),
  reportScore: $("reportScore"),
  skillBreakdown: $("skillBreakdown"),
  reportHighlights: $("reportHighlights"),
  reportImprovements: $("reportImprovements"),
  reportNextSteps: $("reportNextSteps"),
  reportRounds: $("reportRounds"),
};

const phaseText = {
  preparing: "准备题目",
  answering: "等待作答",
  evaluating: "正在评估",
  reporting: "生成报告",
  completed: "已完成",
  failed: "异常中断",
};

async function api(path, options = {}) {
  const res = await fetch(path, {
    ...options,
    headers: {
      "Content-Type": "application/json",
      ...(options.headers || {}),
    },
  });
  const text = await res.text();
  const data = text ? JSON.parse(text) : {};
  if (!res.ok) {
    throw new Error(data.error || `${res.status} ${res.statusText}`);
  }
  return data;
}

function setBusy(busy, label = "") {
  state.busy = busy;
  els.startBtn.disabled = busy;
  els.answerBtn.disabled = busy;
  els.answerText.disabled = busy;
  els.answerBtn.textContent = busy ? "处理中" : "发送";
  if (label) {
    els.statusPill.textContent = label;
  }
}

function setMode(mode) {
  state.mode = mode === "exam" ? "exam" : "practice";
  els.practiceModeBtn.classList.toggle("active", state.mode === "practice");
  els.examModeBtn.classList.toggle("active", state.mode === "exam");
  els.modeDescription.textContent = state.mode === "practice"
    ? "模拟模式会在每题后给出评分、命中要点和改进建议。"
    : "考试模式答题中隐藏评分和参考要点，结束后统一出报告。";
}

function setHealth(ok, text) {
  els.healthDot.classList.toggle("ok", ok);
  els.healthDot.classList.toggle("bad", !ok);
  els.healthText.textContent = text;
}

async function checkHealth() {
  try {
    const data = await api("/api/ping");
    setHealth(true, `已连接 · ${data.llm_mode}`);
  } catch (err) {
    setHealth(false, "未连接");
  }
}

async function startInterview() {
  const payload = {
    mode: state.mode,
    user_id: els.userId.value.trim(),
    jd_text: els.jdText.value.trim(),
    resume_text: els.resumeText.value.trim(),
  };
  if (!payload.jd_text || !payload.resume_text) {
    showLocalNotice("JD 和简历都不能为空");
    return;
  }
  setBusy(true, "准备题目");
  try {
    const session = await api("/api/interview/start", {
      method: "POST",
      body: JSON.stringify(payload),
    });
    applySession(session);
    openStream(session.session_id);
    await loadSessions();
  } catch (err) {
    showLocalNotice(err.message);
  } finally {
    setBusy(false);
  }
}

async function submitAnswer(evt) {
  evt.preventDefault();
  if (!state.session || state.busy) return;
  const answer = els.answerText.value.trim();
  if (!answer) return;

  setBusy(true, "正在评估回答");
  try {
    const session = await api("/api/interview/answer", {
      method: "POST",
      body: JSON.stringify({
        session_id: state.session.session_id,
        user_id: els.userId.value.trim(),
        answer,
      }),
    });
    els.answerText.value = "";
    autoSizeAnswer();
    applySession(session);
    await loadSessions();
  } catch (err) {
    showLocalNotice(err.message);
  } finally {
    setBusy(false);
  }
}

async function loadSession(sessionId) {
  const userId = els.userId.value.trim();
  const suffix = userId ? `?user_id=${encodeURIComponent(userId)}` : "";
  try {
    const session = await api(`/api/interview/sessions/${encodeURIComponent(sessionId)}${suffix}`);
    setMode(session.mode || "exam");
    applySession(session);
    openStream(session.session_id);
  } catch (err) {
    showLocalNotice(err.message);
  }
}

async function loadSessions() {
  const userId = els.userId.value.trim();
  if (!userId) {
    els.sessionList.innerHTML = "";
    return;
  }
  try {
    const data = await api(`/api/interview/sessions?user_id=${encodeURIComponent(userId)}&limit=20`);
    renderSessions(data.sessions || []);
  } catch (err) {
    els.sessionList.innerHTML = `<div class="empty-state">无法读取历史会话</div>`;
  }
}

function openStream(sessionId) {
  closeStream();
  if (!window.EventSource) {
    els.streamState.textContent = "不支持";
    return;
  }
  const userId = els.userId.value.trim();
  const params = new URLSearchParams({ session_id: sessionId });
  if (userId) params.set("user_id", userId);
  if (state.lastEventId) params.set("last_event_id", state.lastEventId);

  state.stream = new EventSource(`/api/interview/stream?${params.toString()}`);
  els.streamState.textContent = "同步中";
  state.stream.onmessage = handleStreamMessage;
  for (const name of ["snapshot", "interview.progress", "interview.completed", "interview.failed"]) {
    state.stream.addEventListener(name, handleStreamMessage);
  }
  state.stream.onerror = () => {
    els.streamState.textContent = "重连中";
  };
}

function closeStream() {
  if (state.stream) {
    state.stream.close();
    state.stream = null;
  }
  els.streamState.textContent = "idle";
}

function handleStreamMessage(evt) {
  if (evt.lastEventId) state.lastEventId = evt.lastEventId;
  const data = JSON.parse(evt.data);
  if (data.error) {
    showLocalNotice(data.error);
  }
  if (data.session_id) {
    applySession(data);
  }
}

function applySession(session) {
  const prev = state.session || {};
  state.session = { ...prev, ...session };
  setMode(state.session.mode || state.mode);
  renderProgress(state.session.progress || []);

  els.setupView.classList.add("hidden");
  els.statusPill.textContent = phaseText[state.session.phase] || "进行中";
  els.sessionMeta.textContent = `${state.session.session_id || ""} · ${modeLabel(state.session.mode)}`;
  els.sessionTitle.textContent = state.session.mode === "practice" ? "模拟训练" : "正式考试";

  if (state.session.status === "completed" || state.session.report) {
    els.interviewView.classList.add("hidden");
    els.reportView.classList.remove("hidden");
    renderReport(state.session);
    closeStream();
    return;
  }

  els.reportView.classList.add("hidden");
  els.interviewView.classList.remove("hidden");
  renderConversation(state.session);
  window.requestAnimationFrame(() => {
    els.conversation.scrollTop = els.conversation.scrollHeight;
  });
}

function renderProgress(progress) {
  const steps = progress.length ? progress : [
    { key: "jd", label: "JD 分析", status: "pending" },
    { key: "resume", label: "简历匹配", status: "pending" },
    { key: "rag", label: "题库检索", status: "pending" },
    { key: "question", label: "出题规划", status: "pending" },
    { key: "interview", label: "面试进行", status: "pending" },
    { key: "report", label: "评估报告", status: "pending" },
  ];
  els.progressBar.innerHTML = steps.map((step) => `
    <div class="progress-step ${escapeHtml(step.status)}">
      <span>${escapeHtml(step.label)}</span>
    </div>
  `).join("");
}

function renderConversation(session) {
  const parts = [];
  const rounds = session.rounds || [];
  for (const round of rounds) {
    parts.push(renderQuestionBubble(round.number, round.question));
    if (round.answer) {
      parts.push(renderAnswerBubble(round.answer));
    }
    if (round.feedback) {
      parts.push(renderFeedbackCard(round.feedback));
    }
    for (const follow of round.follow_ups || []) {
      parts.push(renderFollowUpBubble(follow.question));
      if (follow.answer) parts.push(renderAnswerBubble(follow.answer));
      if (follow.feedback) parts.push(renderFeedbackCard(follow.feedback));
    }
  }
  const currentQuestion = session.question;
  const lastRound = rounds[rounds.length - 1];
  const shouldShowCurrent = currentQuestion && (!lastRound || lastRound.question?.content !== currentQuestion.content || lastRound.answer);
  if (shouldShowCurrent) {
    parts.push(renderQuestionBubble((rounds.length || 0) + 1, currentQuestion));
  }
  if (state.busy) {
    parts.push(`<div class="system-line">正在评估回答，准备下一题...</div>`);
  }
  els.conversation.innerHTML = parts.length ? parts.join("") : `<div class="empty-board">题目生成中...</div>`;
}

function renderQuestionBubble(number, question) {
  const tags = (question?.tags || []).map((tag) => `<span>${escapeHtml(tag)}</span>`).join("");
  return `
    <article class="bubble question">
      <div class="bubble-meta">第 ${number || 1} 题</div>
      <p>${escapeHtml(question?.content || "等待题目")}</p>
      ${tags ? `<div class="tags">${tags}</div>` : ""}
    </article>
  `;
}

function renderFollowUpBubble(question) {
  return `
    <article class="bubble question follow">
      <div class="bubble-meta">追问</div>
      <p>${escapeHtml(question || "请补充说明")}</p>
    </article>
  `;
}

function renderAnswerBubble(answer) {
  return `
    <article class="bubble answer">
      <p>${escapeHtml(answer)}</p>
    </article>
  `;
}

function renderFeedbackCard(feedback) {
  return `
    <article class="feedback-card">
      <div class="feedback-head">
        <span>评分</span>
        <strong>${feedback.score ?? 0}</strong>
      </div>
      ${renderPills("命中要点", feedback.hit_points, "hit")}
      ${renderPills("遗漏要点", feedback.missed_points, "miss")}
      ${renderPills("参考要点", feedback.expected_points, "neutral")}
      ${feedback.suggestion ? `<p class="suggestion">${escapeHtml(feedback.suggestion)}</p>` : ""}
    </article>
  `;
}

function renderPills(title, items, tone) {
  if (!items || !items.length) return "";
  return `
    <div class="pill-block">
      <strong>${escapeHtml(title)}</strong>
      <div>${items.map((item) => `<span class="${tone}">${escapeHtml(item)}</span>`).join("")}</div>
    </div>
  `;
}

function renderReport(session) {
  const report = session.report || {};
  els.reportScore.textContent = report.overall_score ?? 0;
  const breakdown = report.skill_breakdown || {};
  els.skillBreakdown.innerHTML = Object.entries(breakdown).map(([skill, score]) => `
    <div class="score-card">
      <strong>${escapeHtml(skill)}</strong>
      <span>${score}</span>
      <div class="meter"><i style="width:${clamp(score, 0, 100)}%"></i></div>
    </div>
  `).join("") || `<div class="empty-state">暂无技能分布</div>`;
  renderList(els.reportHighlights, report.highlights || []);
  renderList(els.reportImprovements, report.improvements || []);
  renderList(els.reportNextSteps, report.next_steps || []);
  els.reportRounds.innerHTML = (session.rounds || []).map((round) => `
    <article class="review-card">
      <div class="review-head">
        <strong>第 ${round.number} 题</strong>
        ${round.feedback ? `<span>${round.feedback.score} 分</span>` : ""}
      </div>
      <p class="review-question">${escapeHtml(round.question?.content || "")}</p>
      ${round.answer ? `<p class="review-answer">${escapeHtml(round.answer)}</p>` : ""}
      ${round.feedback ? renderFeedbackCard(round.feedback) : ""}
    </article>
  `).join("") || `<div class="empty-state">暂无逐题记录</div>`;
}

function renderList(el, items) {
  el.innerHTML = items.length
    ? items.map((item) => `<li>${escapeHtml(item)}</li>`).join("")
    : "<li>暂无</li>";
}

function renderSessions(sessions) {
  if (!sessions.length) {
    els.sessionList.innerHTML = `<div class="empty-state">暂无历史会话</div>`;
    return;
  }
  els.sessionList.innerHTML = sessions.map((s) => `
    <button class="session-item" type="button" data-session="${escapeHtml(s.session_id)}">
      <strong>${escapeHtml(modeLabel(s.mode))} · ${escapeHtml(s.status)}</strong>
      <span>${formatTime(s.updated_at)}</span>
    </button>
  `).join("");
  els.sessionList.querySelectorAll("[data-session]").forEach((btn) => {
    btn.addEventListener("click", () => loadSession(btn.dataset.session));
  });
}

function showLocalNotice(message) {
  els.conversation.innerHTML += `<div class="system-line error">${escapeHtml(message)}</div>`;
}

function resetCurrent() {
  closeStream();
  state.session = null;
  state.lastEventId = "";
  els.answerText.value = "";
  els.conversation.innerHTML = "";
  els.setupView.classList.remove("hidden");
  els.interviewView.classList.add("hidden");
  els.reportView.classList.add("hidden");
  renderProgress([]);
}

function autoSizeAnswer() {
  els.answerText.style.height = "auto";
  els.answerText.style.height = `${Math.min(140, Math.max(44, els.answerText.scrollHeight))}px`;
}

function modeLabel(mode) {
  return mode === "practice" ? "模拟" : "考试";
}

function escapeHtml(value) {
  return String(value ?? "")
    .replaceAll("&", "&amp;")
    .replaceAll("<", "&lt;")
    .replaceAll(">", "&gt;")
    .replaceAll('"', "&quot;")
    .replaceAll("'", "&#039;");
}

function clamp(n, min, max) {
  return Math.max(min, Math.min(max, Number(n) || 0));
}

function formatTime(value) {
  if (!value) return "-";
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) return "-";
  return date.toLocaleString();
}

els.startBtn.addEventListener("click", startInterview);
els.answerForm.addEventListener("submit", submitAnswer);
els.resetBtn.addEventListener("click", resetCurrent);
els.newInterviewBtn.addEventListener("click", resetCurrent);
els.backToSetupBtn.addEventListener("click", resetCurrent);
els.endInterviewBtn.addEventListener("click", resetCurrent);
els.refreshSessions.addEventListener("click", loadSessions);
els.userId.addEventListener("change", loadSessions);
els.answerText.addEventListener("input", autoSizeAnswer);
els.practiceModeBtn.addEventListener("click", () => setMode("practice"));
els.examModeBtn.addEventListener("click", () => setMode("exam"));

setMode("practice");
renderProgress([]);
checkHealth();
loadSessions();
