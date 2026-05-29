import React, { useCallback, useEffect, useMemo, useState } from "react";
import { createRoot } from "react-dom/client";
import { apiClient } from "./apiClient";
import { analysisSummary, buildDraft, clearDraft, DRAFT_KEY, draftScopeSummary, drillJDText, loadDraft, normalizeQuestionBankFilter, saveDraft } from "./draftStore";
import { normalizeRoute, questionURL, routes, type Route } from "./routes";
import { useInterviewStream, type StreamEvent } from "./useInterviewStream";
import type {
  Draft,
  DrillPlanItem,
  InterviewFeedback,
  InterviewQuestion,
  InterviewRound,
  Mode,
  ProfileAnalyzeResponse,
  ProfileAnalysis,
  QuestionBankImportJob,
  QuestionBankImportItem,
  QuestionBankItem,
  QuestionFacets,
  Session,
  SessionSummary,
  TranscriptAnalysis,
} from "./types";
import "./styles.css";

const defaultResume = "两年 Go 后端经验，参与过秒杀活动、Redis 缓存优化、Kafka 消费链路和 PostgreSQL 查询优化。";
const defaultJD = "需要 Go 后端工程师，熟悉高并发服务、Redis、PostgreSQL、消息队列和线上问题排查。";

function App() {
  const [route, setRoute] = useState<Route>(() => normalizeRoute(window.location.pathname));
  const [draft, setDraft] = useState<Draft>(() => {
    const saved = loadDraft();
    return {
      ...saved,
      resume_text: saved.resume_text || defaultResume,
      jd_text: saved.jd_text || defaultJD,
    };
  });
  const [userId, setUserId] = useState("demo-user");
  const [mode, setMode] = useState<Mode>("practice");
  const [health, setHealth] = useState("连接中");
  const [busy, setBusy] = useState(false);
  const [notice, setNotice] = useState("");
  const [session, setSession] = useState<Session | null>(null);
  const [sessions, setSessions] = useState<SessionSummary[]>([]);
  const [events, setEvents] = useState<StreamEvent[]>([]);
  const [pendingAnswer, setPendingAnswer] = useState("");
  const [questionJump, setQuestionJump] = useState(() => new URLSearchParams(window.location.search).get("q") || "");

  const navigate = useCallback((next: Route, search = "") => {
    window.history.pushState({}, "", `${next}${search}`);
    setRoute(next);
    setQuestionJump(new URLSearchParams(search).get("q") || "");
  }, []);

  useEffect(() => {
    const onPop = () => {
      setRoute(normalizeRoute(window.location.pathname));
      setQuestionJump(new URLSearchParams(window.location.search).get("q") || "");
    };
    window.addEventListener("popstate", onPop);
    if (window.location.pathname === "/") navigate(routes.resume);
    return () => window.removeEventListener("popstate", onPop);
  }, [navigate]);

  useEffect(() => {
    apiClient.ping()
      .then((data) => setHealth(`已连接 · ${data.llm_mode}`))
      .catch(() => setHealth("未连接"));
  }, []);

  const refreshSessions = useCallback(() => {
    if (!userId.trim()) return;
    apiClient.listSessions(userId.trim())
      .then((data) => setSessions(data.sessions || []))
      .catch(() => setSessions([]));
  }, [userId]);

  useEffect(() => {
    refreshSessions();
  }, [refreshSessions]);

  const updateDraft = useCallback((patch: Partial<Draft>) => {
    setDraft((prev) => {
      const next = buildDraft(prev, patch);
      window.localStorage.setItem(DRAFT_KEY, JSON.stringify(next));
      return next;
    });
  }, []);

  const pushEvent = useCallback((event: StreamEvent) => {
    setEvents((prev) => [event, ...prev].slice(0, 6));
  }, []);

  const mergeSession = useCallback((next: Session) => {
    setSession((prev) => ({ ...(prev || {}), ...next }));
    if (next.status === "completed" || next.report) navigate(routes.report);
  }, [navigate]);

  useInterviewStream(session?.session_id || "", userId.trim(), route === routes.interview && Boolean(session), mergeSession, pushEvent);

  const analyze = async () => {
    if (!draft.resume_text.trim() || !draft.jd_text.trim()) {
      setNotice("简历和 JD 都不能为空");
      return;
    }
    setBusy(true);
    setNotice("正在分析 JD 与简历匹配度...");
    try {
      const analysis = await apiClient.analyzeProfile(draft.jd_text.trim(), draft.resume_text.trim());
      updateDraft({ analysis });
      setNotice("分析完成，可以确认后开始面试。");
    } catch (err) {
      setNotice(errorMessage(err));
    } finally {
      setBusy(false);
    }
  };

  const startInterview = async () => {
    if (!draft.resume_text.trim() || !draft.jd_text.trim()) {
      setNotice("简历和 JD 都不能为空");
      return;
    }
    setBusy(true);
    setEvents([]);
    setPendingAnswer("");
    setNotice("正在创建面试并生成第一题...");
    try {
      const next = await apiClient.startInterview({
        user_id: userId.trim(),
        mode,
        jd_text: draft.jd_text.trim(),
        resume_text: draft.resume_text.trim(),
        question_bank_filter: normalizeQuestionBankFilter(draft.question_bank_filter),
      });
      setSession(next);
      setNotice("");
      navigate(routes.interview);
      refreshSessions();
    } catch (err) {
      setNotice(errorMessage(err));
    } finally {
      setBusy(false);
    }
  };

  const submitAnswer = async (answer: string) => {
    if (!session || !answer.trim()) return;
    setBusy(true);
    setPendingAnswer(answer.trim());
    pushEvent({ type: "answer.submitted", label: "提交", detail: "回答已提交，等待评估结果", at: new Date().toLocaleTimeString() });
    try {
      const next = await apiClient.answerInterview({
        session_id: session.session_id,
        user_id: userId.trim(),
        answer: answer.trim(),
      });
      setPendingAnswer("");
      setSession(next);
      if (next.status === "completed" || next.report) navigate(routes.report);
      refreshSessions();
    } catch (err) {
      setNotice(errorMessage(err));
    } finally {
      setBusy(false);
    }
  };

  const loadSession = async (sessionId: string) => {
    try {
      const next = await apiClient.loadSession(sessionId, userId.trim());
      setSession(next);
      navigate(next.status === "completed" || next.report ? routes.report : routes.interview);
    } catch (err) {
      setNotice(errorMessage(err));
    }
  };

  const resetDraft = () => {
    setDraft(clearDraft());
    setNotice("草稿已清空。");
  };

  const startDrill = (plan: DrillPlanItem[]) => {
    const nextJD = drillJDText(session?.job_profile?.jd_raw_text || draft.jd_text, plan);
    setDraft(saveDraft({
      resume_text: session?.candidate_profile?.resume_raw_text || draft.resume_text,
      jd_text: nextJD,
      analysis: undefined,
    }));
    setNotice("已按报告训练计划预填弱项训练重点。");
    navigate(routes.jd);
  };

  const navItems = [
    ["简历", routes.resume],
    ["JD 分析", routes.jd],
    ["面试", routes.interview],
    ["报告", routes.report],
    ["题库", routes.questions],
  ] as const;

  return (
    <main className="app-shell">
      <aside className="sidebar">
        <div className="brand">
          <strong>InterviewAgent</strong>
          <span>面试准备、评估和训练工作台</span>
        </div>
        <nav className="nav-list" aria-label="主导航">
          {navItems.map(([label, href]) => (
            <button key={href} className={route === href ? "active" : ""} onClick={() => navigate(href)}>
              {label}
            </button>
          ))}
        </nav>
        <div className="mode-switch">
          <button className={mode === "practice" ? "active" : ""} onClick={() => setMode("practice")}>模拟</button>
          <button className={mode === "exam" ? "active" : ""} onClick={() => setMode("exam")}>考试</button>
        </div>
        <label className="field compact">
          <span>用户 ID</span>
          <input value={userId} onChange={(evt) => setUserId(evt.target.value)} onBlur={refreshSessions} />
        </label>
        <section className="side-section">
          <div className="section-title">
            <span>历史会话</span>
            <button onClick={refreshSessions} className="ghost-icon">↻</button>
          </div>
          <div className="session-list">
            {sessions.length ? sessions.map((item) => (
              <button key={item.session_id} onClick={() => loadSession(item.session_id)} className="session-item">
                <strong>{modeLabel(item.mode)} · {item.status}</strong>
                <span>{formatTime(item.updated_at)}</span>
              </button>
            )) : <div className="empty-state">暂无历史会话</div>}
          </div>
        </section>
        <div className="connection">
          <span className={`dot ${health.startsWith("已") ? "ok" : "bad"}`} />
          <span>{health}</span>
        </div>
      </aside>

      <section className="main">
        <ProgressBar session={session} route={route} />
        {notice && <div className="top-notice">{notice}</div>}
        {route === routes.resume && (
          <ResumePage draft={draft} busy={busy} updateDraft={updateDraft} resetDraft={resetDraft} goNext={() => navigate(routes.jd)} setNotice={setNotice} />
        )}
        {route === routes.jd && (
          <JDPage draft={draft} busy={busy} updateDraft={updateDraft} analyze={analyze} startInterview={startInterview} />
        )}
        {route === routes.interview && (
          <InterviewPage session={session} events={events} busy={busy} pendingAnswer={pendingAnswer} submitAnswer={submitAnswer} goJD={() => navigate(routes.jd)} />
        )}
        {route === routes.report && (
          <ReportPage session={session} startDrill={startDrill} jumpQuestion={(id) => navigate(routes.questions, `?q=${encodeURIComponent(id)}`)} />
        )}
        {route === routes.questions && <QuestionBankPage jumpId={questionJump} />}
      </section>
    </main>
  );
}

function ResumePage({ draft, busy, updateDraft, resetDraft, goNext, setNotice }: {
  draft: Draft;
  busy: boolean;
  updateDraft: (patch: Partial<Draft>) => void;
  resetDraft: () => void;
  goNext: () => void;
  setNotice: (message: string) => void;
}) {
  const [uploading, setUploading] = useState(false);
  const fileRef = React.useRef<HTMLInputElement>(null);
  const upload = async (file?: File) => {
    if (!file) return;
    setUploading(true);
    setNotice(`正在读取简历文档：${file.name}...`);
    try {
      const data = await apiClient.parseResume(file);
      updateDraft({ resume_text: data.text || "" });
      setNotice(`已读取 ${data.filename || file.name}，可继续编辑。`);
    } catch (err) {
      setNotice(errorMessage(err));
    } finally {
      setUploading(false);
      if (fileRef.current) fileRef.current.value = "";
    }
  };
  return (
    <section className="page resume-page">
      <PageHeader eyebrow="Step 1 · 简历档案" title="先把候选人资料整理清楚。" copy="简历是后续 JD 匹配、追问计划和报告解释的基础。上传文档后仍可手动修正。" />
      <div className="two-column">
        <label className="field tall">
          <span>候选人简历</span>
          <textarea value={draft.resume_text} onChange={(evt) => updateDraft({ resume_text: evt.target.value, analysis: undefined })} />
        </label>
        <aside className="control-panel">
          <h2>简历输入</h2>
          <p>支持 PDF、DOCX、TXT 和 Markdown。解析结果只填入草稿，不会创建面试会话。</p>
          <input ref={fileRef} className="visually-hidden" type="file" accept=".txt,.md,.markdown,.pdf,.docx,text/plain,text/markdown,application/pdf,application/vnd.openxmlformats-officedocument.wordprocessingml.document" onChange={(evt) => upload(evt.target.files?.[0])} />
          <button className="secondary" disabled={uploading} onClick={() => fileRef.current?.click()}>{uploading ? "读取中" : "读取简历文档"}</button>
          <button className="primary" disabled={busy || !draft.resume_text.trim()} onClick={goNext}>下一步：填写 JD</button>
          <button className="ghost" onClick={resetDraft}>清空草稿</button>
          <div className="metric-box">
            <strong>{draft.resume_text.trim().length}</strong>
            <span>简历字符</span>
          </div>
        </aside>
      </div>
    </section>
  );
}

function JDPage({ draft, busy, updateDraft, analyze, startInterview }: {
  draft: Draft;
  busy: boolean;
  updateDraft: (patch: Partial<Draft>) => void;
  analyze: () => Promise<void>;
  startInterview: () => Promise<void>;
}) {
  const [facets, setFacets] = useState<QuestionFacets>({ skill_categories: {}, scenarios: {}, tags: {}, difficulties: {} });
  const [facetError, setFacetError] = useState("");
  useEffect(() => {
    apiClient.questionFacets().then(setFacets).catch((err) => setFacetError(errorMessage(err)));
  }, []);
  const scope = normalizeQuestionBankFilter(draft.question_bank_filter);
  const setScope = (patch: Partial<NonNullable<Draft["question_bank_filter"]>>) => {
    updateDraft({ question_bank_filter: normalizeQuestionBankFilter({ ...(scope || {}), ...patch }) });
  };
  const difficulty = scope?.difficulty_min && scope.difficulty_min === scope.difficulty_max ? String(scope.difficulty_min) : "";
  return (
    <section className="page jd-page">
      <PageHeader eyebrow="Step 2 · JD 分析" title="再把岗位要求和简历做一次可解释匹配。" copy="分析不会创建面试；确认后才会正式启动 Graph 并生成第一题。" />
      <div className="two-column">
        <label className="field tall">
          <span>岗位 JD</span>
          <textarea value={draft.jd_text} onChange={(evt) => updateDraft({ jd_text: evt.target.value, analysis: undefined })} />
        </label>
        <aside className="control-panel">
          <h2>准备检查</h2>
          <p>{analysisSummary(draft.analysis)}</p>
          <div className="scope-panel">
            <h3>题库范围</h3>
            <p>{draftScopeSummary(scope)}</p>
            <Select value={scope?.skill_categories?.[0] || ""} onChange={(value) => setScope({ skill_categories: value ? [value] : [] })} label="全部技能" values={facets.skill_categories} />
            <Select value={scope?.scenarios?.[0] || ""} onChange={(value) => setScope({ scenarios: value ? [value] : [] })} label="全部场景" values={facets.scenarios} />
            <Select value={difficulty} onChange={(value) => setScope({ difficulty_min: value ? Number(value) : undefined, difficulty_max: value ? Number(value) : undefined })} label="全部难度" values={facets.difficulties} format={(v) => `难度 ${v}`} />
            <input value={(scope?.tags || []).join(", ")} onChange={(evt) => setScope({ tags: evt.target.value.split(",") })} placeholder="标签，用逗号分隔" />
            {facetError && <span className="scope-error">{facetError}</span>}
          </div>
          <button className="secondary" disabled={busy || !draft.jd_text.trim() || !draft.resume_text.trim()} onClick={analyze}>
            {busy ? "分析中" : "分析 JD / 简历"}
          </button>
          <button className="primary" disabled={busy || !draft.jd_text.trim() || !draft.resume_text.trim()} onClick={startInterview}>
            开始面试
          </button>
        </aside>
      </div>
      <ProfileAnalysisPanel analysis={draft.analysis?.profile_analysis} />
    </section>
  );
}

function InterviewPage({ session, events, busy, pendingAnswer, submitAnswer, goJD }: {
  session: Session | null;
  events: StreamEvent[];
  busy: boolean;
  pendingAnswer: string;
  submitAnswer: (answer: string) => Promise<void>;
  goJD: () => void;
}) {
  const [answer, setAnswer] = useState("");
  const rounds = session?.rounds || [];
  const send = async (evt: React.FormEvent) => {
    evt.preventDefault();
    const value = answer.trim();
    if (!value) return;
    setAnswer("");
    await submitAnswer(value);
  };
  if (!session) {
    return <EmptyPage title="还没有进行中的面试" action="回到 JD 分析" onAction={goJD} />;
  }
  return (
    <section className="page interview-page">
      <div className="interview-head">
        <div>
          <p className="eyebrow">{session.session_id} · {modeLabel(session.mode)}</p>
          <h1>{session.mode === "practice" ? "模拟训练" : "正式考试"}</h1>
        </div>
        <span className="status-pill">{session.phase || session.status}</span>
      </div>
      <ProfileAnalysisPanel analysis={session.profile_analysis} compact />
      <EventTimeline events={events} />
      <div className="conversation">
        {rounds.map((round) => <RoundView key={round.round_id} round={round} />)}
        {shouldShowCurrent(session.question, rounds) && <QuestionBubble number={(rounds.length || 0) + 1} question={session.question} />}
        {pendingAnswer && <article className="bubble answer pending"><div className="bubble-meta">已提交，等待评分</div><p>{pendingAnswer}</p></article>}
        {busy && <div className="system-line">正在评估回答，准备下一题...</div>}
      </div>
      <form className="answer-dock" onSubmit={send}>
        <label className="answer-box">
          <span>候选人回答 · Ctrl/⌘ + Enter 发送</span>
          <textarea value={answer} onChange={(evt) => setAnswer(evt.target.value)} onKeyDown={(evt) => {
            if ((evt.ctrlKey || evt.metaKey) && evt.key === "Enter") {
              evt.preventDefault();
              evt.currentTarget.form?.requestSubmit();
            }
          }} />
        </label>
        <button className="send" disabled={busy || !answer.trim()}>{busy ? "处理中" : "发送"}</button>
      </form>
    </section>
  );
}

function ReportPage({ session, startDrill, jumpQuestion }: {
  session: Session | null;
  startDrill: (plan: DrillPlanItem[]) => void;
  jumpQuestion: (id: string) => void;
}) {
  if (!session?.report) return <EmptyPage title="还没有可查看的报告" action="去面试页" onAction={() => window.history.pushState({}, "", routes.interview)} />;
  const report = session.report;
  return (
    <section className="page report-page">
      <div className="report-hero">
        <div>
          <p className="eyebrow">最终报告</p>
          <h1>{report.overall_score}<small>分</small></h1>
        </div>
        <div className="score-grid">
          {Object.entries(report.skill_breakdown || {}).map(([skill, score]) => <ScoreCard key={skill} skill={skill} score={score} />)}
        </div>
      </div>
      <ProfileAnalysisPanel analysis={session.profile_analysis} />
      <TranscriptPanel analysis={report.transcript_analysis} />
      <DrillPlanPanel plan={report.drill_plan || []} startDrill={startDrill} jumpQuestion={jumpQuestion} />
      <div className="report-summary">
        <ListSection title="亮点" items={report.highlights} />
        <ListSection title="改进项" items={report.improvements} />
        <ListSection title="下一步" items={report.next_steps} />
      </div>
      <section className="round-review">
        <h2>逐题评分</h2>
        {(session.rounds || []).map((round) => <RoundReview key={round.round_id} round={round} />)}
      </section>
    </section>
  );
}

function QuestionBankPage({ jumpId }: { jumpId: string }) {
  const [items, setItems] = useState<QuestionBankItem[]>([]);
  const [facets, setFacets] = useState<QuestionFacets>({ skill_categories: {}, scenarios: {}, tags: {}, difficulties: {} });
  const [imports, setImports] = useState<QuestionBankImportJob[]>([]);
  const [importItems, setImportItems] = useState<QuestionBankImportItem[]>([]);
  const [selectedImportId, setSelectedImportId] = useState("");
  const [importSource, setImportSource] = useState<"question_bank" | "document">("question_bank");
  const [importBusy, setImportBusy] = useState(false);
  const [selectedId, setSelectedId] = useState(jumpId);
  const [query, setQuery] = useState(jumpId);
  const [skill, setSkill] = useState("");
  const [scenario, setScenario] = useState("");
  const [difficulty, setDifficulty] = useState("");
  const [admin, setAdmin] = useState(false);
  const [error, setError] = useState("");
  const importFileRef = React.useRef<HTMLInputElement>(null);
  const selected = items.find((item) => item.id === selectedId);

  const refreshImports = useCallback(() => {
    apiClient.listQuestionImports().then((data) => {
      const jobs = data.jobs || [];
      setImports(jobs);
      setSelectedImportId((prev) => prev || jobs[0]?.id || "");
    }).catch((err) => setError(errorMessage(err)));
  }, []);

  useEffect(() => {
    apiClient.questionFacets().then(setFacets).catch((err) => setError(errorMessage(err)));
    refreshImports();
  }, [refreshImports]);

  const hasRunningImport = imports.some((job) => ["queued", "parsing", "generating", "validating", "committing"].includes(job.status));
  useEffect(() => {
    if (!hasRunningImport) return;
    const t = window.setInterval(refreshImports, 1200);
    return () => window.clearInterval(t);
  }, [hasRunningImport, refreshImports]);

  useEffect(() => {
    if (jumpId) {
      setQuery(jumpId);
      setSelectedId(jumpId);
    }
  }, [jumpId]);

  const load = useCallback(() => {
    const params = new URLSearchParams({ limit: "20" });
    if (query.trim()) params.set("q", query.trim());
    if (skill) params.set("skill_category", skill);
    if (scenario) params.set("scenario", scenario);
    if (difficulty) params.set("difficulty", difficulty);
    if (admin) params.set("view", "admin");
    apiClient.questionBank(params).then((data) => {
      setItems(data.items || []);
      if (!data.items.some((item) => item.id === selectedId)) {
        setSelectedId(data.items[0]?.id || "");
      }
    }).catch((err) => setError(errorMessage(err)));
  }, [admin, difficulty, query, scenario, selectedId, skill]);

  useEffect(() => {
    if (!selectedImportId) {
      setImportItems([]);
      return;
    }
    apiClient.getQuestionImport(selectedImportId)
      .then((data) => setImportItems(data.items || []))
      .catch((err) => setError(errorMessage(err)));
  }, [selectedImportId]);

  useEffect(() => {
    const t = window.setTimeout(load, 180);
    return () => window.clearTimeout(t);
  }, [load]);

  const uploadImport = async (file?: File) => {
    if (!file) return;
    setImportBusy(true);
    setError("");
    try {
      const data = await apiClient.createQuestionImport(importSource, file);
      setSelectedImportId(data.job.id);
      refreshImports();
    } catch (err) {
      setError(errorMessage(err));
    } finally {
      setImportBusy(false);
      if (importFileRef.current) importFileRef.current.value = "";
    }
  };

  const commitImport = async (id: string) => {
    if (!id) return;
    setImportBusy(true);
    setError("");
    try {
      await apiClient.commitQuestionImport(id);
      refreshImports();
      load();
    } catch (err) {
      setError(errorMessage(err));
    } finally {
      setImportBusy(false);
    }
  };

  const reviewImport = async (id: string, action: string, itemIds: string[] = []) => {
    if (!id) return;
    setImportBusy(true);
    setError("");
    try {
      const data = await apiClient.reviewQuestionImportItems(id, action, itemIds);
      setImportItems(data.items || []);
      refreshImports();
    } catch (err) {
      setError(errorMessage(err));
    } finally {
      setImportBusy(false);
    }
  };

  return (
    <section className="page questions-page">
      <PageHeader eyebrow="题库工作区" title="题库独立管理，面试页只保留面试。" copy="候选人视图隐藏答案和评分标准；管理视图用于检查 rubric 和追问提示。" />
      <section className="import-workbench">
        <div className="import-actions">
          <div>
            <p className="eyebrow">导入流水线</p>
            <h2>先暂存校验，再提交进 RAG 题库。</h2>
          </div>
          <div className="segmented">
            <button className={importSource === "question_bank" ? "active" : ""} onClick={() => setImportSource("question_bank")}>本地题库</button>
            <button className={importSource === "document" ? "active" : ""} onClick={() => setImportSource("document")}>文档生成</button>
          </div>
          <input
            ref={importFileRef}
            className="visually-hidden"
            type="file"
            accept=".json,.csv,.md,.markdown,.txt,.pdf,.docx,text/plain,text/markdown,application/json,text/csv,application/pdf,application/vnd.openxmlformats-officedocument.wordprocessingml.document"
            onChange={(evt) => uploadImport(evt.target.files?.[0])}
          />
          <button className="secondary" disabled={importBusy} onClick={() => importFileRef.current?.click()}>{importBusy ? "处理中" : "上传导入"}</button>
        </div>
        <div className="import-grid">
          <div className="import-list">
            {imports.length ? imports.map((job) => (
              <button key={job.id} className={`import-row ${job.id === selectedImportId ? "active" : ""}`} onClick={() => setSelectedImportId(job.id)}>
                <strong>{job.filename || job.id}</strong>
                <span>{importSourceLabel(job.source_type)} · {job.status}</span>
                <em>{job.valid_items}/{job.total_items} 有效</em>
              </button>
            )) : <div className="empty-state">暂无导入任务</div>}
          </div>
          <ImportDetail jobs={imports} selectedId={selectedImportId} items={importItems} busy={importBusy} commitImport={commitImport} reviewImport={reviewImport} />
        </div>
      </section>
      <div className="question-toolbar">
        <input value={query} onChange={(evt) => setQuery(evt.target.value)} placeholder="搜索题干、标签或编号" />
        <Select value={skill} onChange={setSkill} label="全部技能" values={facets.skill_categories} />
        <Select value={scenario} onChange={setScenario} label="全部场景" values={facets.scenarios} />
        <Select value={difficulty} onChange={setDifficulty} label="全部难度" values={facets.difficulties} format={(v) => `难度 ${v}`} />
        <label className="admin-toggle"><input type="checkbox" checked={admin} onChange={(evt) => setAdmin(evt.target.checked)} /> 管理视图</label>
      </div>
      {error && <div className="system-line error">{error}</div>}
      <div className="question-layout">
        <div className="question-list">
          <div className="question-count">显示 {items.length} 道题</div>
          {items.map((item) => <QuestionCard key={item.id} item={item} active={item.id === selectedId} onClick={() => setSelectedId(item.id)} />)}
        </div>
        <QuestionDetail item={selected} admin={admin} />
      </div>
    </section>
  );
}

function ImportDetail({ jobs, selectedId, items, busy, commitImport, reviewImport }: {
  jobs: QuestionBankImportJob[];
  selectedId: string;
  items: QuestionBankImportItem[];
  busy: boolean;
  commitImport: (id: string) => void;
  reviewImport: (id: string, action: string, itemIds?: string[]) => void;
}) {
  const job = jobs.find((item) => item.id === selectedId);
  if (!job) return <aside className="import-detail"><div className="empty-state">选择导入任务</div></aside>;
  const accepted = items.filter((item) => item.status === "valid" && reviewStatus(item) === "accepted").length;
  const rejected = items.filter((item) => item.status === "valid" && reviewStatus(item) === "rejected").length;
  return (
    <aside className="import-detail">
      <div className="import-detail-head">
        <div>
          <strong>{job.status}</strong>
          <span>{job.id}</span>
        </div>
        <button className="primary" disabled={busy || job.status !== "ready" || accepted === 0} onClick={() => commitImport(job.id)}>提交入库</button>
      </div>
      <div className="import-stats">
        <span>切片 {job.total_chunks}</span>
        <span>有效 {job.valid_items}</span>
        <span>接受 {accepted}</span>
        <span>拒绝 {rejected}</span>
        <span>无效 {job.invalid_items}</span>
        <span>已入库 {job.imported_items}</span>
      </div>
      <div className="import-review-toolbar">
        <button className="secondary" disabled={busy || job.status !== "ready"} onClick={() => reviewImport(job.id, "accept_all_valid")}>接受全部有效</button>
        <button className="secondary" disabled={busy || job.status !== "ready"} onClick={() => reviewImport(job.id, "accept_complete_valid")}>接受字段完整</button>
        <button className="secondary" disabled={busy || job.status !== "ready"} onClick={() => reviewImport(job.id, "reject_all_valid")}>拒绝全部有效</button>
        <span>质量分规则预留</span>
      </div>
      {job.error && <p className="system-line error">{job.error}</p>}
      <div className="import-items">
        {items.map((item) => (
          <article key={item.id} className={`import-item ${item.status} review-${reviewStatus(item)}`}>
            <div>
              <strong>{item.question_id}</strong>
              <span>{item.status} · {reviewStatusLabel(reviewStatus(item))}</span>
            </div>
            <p>{item.item.content || "空题干"}</p>
            {item.status === "valid" && (
              <div className="import-item-actions">
                <button className={reviewStatus(item) === "accepted" ? "active" : ""} disabled={busy || job.status !== "ready"} onClick={() => reviewImport(job.id, "accept", [item.id])}>接受</button>
                <button className={reviewStatus(item) === "rejected" ? "active danger" : "danger"} disabled={busy || job.status !== "ready"} onClick={() => reviewImport(job.id, "reject", [item.id])}>拒绝</button>
              </div>
            )}
            <ImportDiff item={item} />
            {!!item.errors?.length && <em>{item.errors.join("；")}</em>}
          </article>
        ))}
      </div>
    </aside>
  );
}

function reviewStatus(item: QuestionBankImportItem) {
  return item.review_status || "accepted";
}

function reviewStatusLabel(status: string) {
  return status === "rejected" ? "已拒绝" : "已接受";
}

function ImportDiff({ item }: { item: QuestionBankImportItem }) {
  const rows = importDiffRows(item);
  if (!rows.length) return null;
  return (
    <div className="import-diff">
      {rows.map((row) => (
        <div key={row.key} className="import-diff-row">
          <span>{row.label}</span>
          <code>{row.before || "空"}</code>
          <code>{row.after || "空"}</code>
          <em>{row.source}</em>
        </div>
      ))}
    </div>
  );
}

function importDiffRows(importItem: QuestionBankImportItem) {
  const original = importItem.original_item;
  if (!original) return [];
  const fields: Array<[keyof QuestionBankItem, string]> = [
    ["skill_category", "技能"],
    ["difficulty", "难度"],
    ["tags", "标签"],
    ["expected_points", "要点"],
    ["rubric", "Rubric"],
    ["sample_answer", "参考答案"],
    ["follow_up_hints", "追问"],
  ];
  return fields.map(([key, label]) => {
    const before = formatImportField(original[key]);
    const after = formatImportField(importItem.item[key]);
    return {
      key,
      label,
      before,
      after,
      source: before === after ? "上传" : before ? "合并" : "LLM",
    };
  }).filter((row) => row.before || row.after);
}

function PageHeader({ eyebrow, title, copy }: { eyebrow: string; title: string; copy: string }) {
  return <header className="page-header"><p className="eyebrow">{eyebrow}</p><h1>{title}</h1><p>{copy}</p></header>;
}

function ProgressBar({ session, route }: { session: Session | null; route: Route }) {
  const steps = session?.progress || [
    { key: "resume", label: "简历", status: route === routes.resume ? "active" : "pending" },
    { key: "jd", label: "JD 分析", status: route === routes.jd ? "active" : "pending" },
    { key: "interview", label: "面试", status: route === routes.interview ? "active" : "pending" },
    { key: "report", label: "报告", status: route === routes.report ? "active" : "pending" },
  ];
  return <header className="progress-bar">{steps.map((step) => <div key={step.key} className={`progress-step ${step.status}`}><span>{step.label}</span></div>)}</header>;
}

function ProfileAnalysisPanel({ analysis, compact = false }: { analysis?: ProfileAnalysis; compact?: boolean }) {
  if (!analysis) return null;
  return (
    <section className={`profile-analysis ${compact ? "compact" : ""}`}>
      <div className="profile-score"><span>匹配分</span><strong>{analysis.match_score}</strong></div>
      <div className="profile-main">
        <div className="profile-summary">
          <h2>JD / 简历分析</h2>
          <p>{analysis.summary}</p>
          <div className="profile-meta">
            <span>年限差 {formatYearsGap(analysis.years_gap)}</span>
            <span>重点 {(analysis.question_focus || []).join(" / ") || "-"}</span>
          </div>
        </div>
        <div className="profile-columns">
          <AnalysisBlock title="命中要求" items={analysis.matched_requirements} tone="good" />
          <AnalysisBlock title="缺失要求" items={analysis.missing_requirements} tone="bad" />
          <AnalysisBlock title="风险点" items={analysis.risk_points} tone="warn" />
          <AnalysisBlock title="简历优化" items={analysis.resume_suggestions} tone="info" />
        </div>
        {!!analysis.project_probe_plan?.length && (
          <div className="probe-plan">
            <h3>项目追问计划</h3>
            <div>{analysis.project_probe_plan.map((item) => <article key={`${item.project_name}-${item.focus}`}><strong>{item.project_name || "未命名项目"}</strong><span>{item.focus}</span><p>{item.suggested_question}</p></article>)}</div>
          </div>
        )}
      </div>
    </section>
  );
}

function AnalysisBlock({ title, items, tone }: { title: string; items?: string[]; tone: string }) {
  return <section className="analysis-block"><h3>{title}</h3><div>{items?.length ? items.map((item) => <span key={item} className={tone}>{item}</span>) : <em>暂无</em>}</div></section>;
}

function EventTimeline({ events }: { events: StreamEvent[] }) {
  return <div className="event-timeline">{events.length ? events.map((event, i) => <span key={`${event.type}-${i}`} className="event-chip" title={event.type}><strong>{event.label}</strong><em>{event.detail || event.at}</em></span>) : <span className="event-chip muted">等待实时事件</span>}</div>;
}

function RoundView({ round }: { round: InterviewRound }) {
  return <>{<QuestionBubble number={round.number} question={round.question} />}{round.answer && <AnswerBubble answer={round.answer} />}{round.feedback && <FeedbackCard feedback={round.feedback} />}{round.follow_ups?.map((follow, index) => <React.Fragment key={index}><article className="bubble question follow"><div className="bubble-meta">追问</div><p>{follow.question}</p></article>{follow.answer && <AnswerBubble answer={follow.answer} />}{follow.feedback && <FeedbackCard feedback={follow.feedback} />}</React.Fragment>)}</>;
}

function QuestionBubble({ number, question }: { number: number; question?: InterviewQuestion }) {
  return <article className="bubble question"><div className="bubble-meta">第 {number || 1} 题</div><p>{question?.content || "等待题目"}</p><div className="tags">{question?.tags?.map((tag) => <span key={tag}>{tag}</span>)}</div></article>;
}

function AnswerBubble({ answer }: { answer: string }) {
  return <article className="bubble answer"><p>{answer}</p></article>;
}

function FeedbackCard({ feedback }: { feedback: InterviewFeedback }) {
  return <article className="feedback-card"><div className="feedback-head"><span>评分</span><strong>{feedback.score}</strong></div><Pills title="命中要点" items={feedback.hit_points} tone="hit" /><Pills title="遗漏要点" items={feedback.missed_points} tone="miss" /><Pills title="参考要点" items={feedback.expected_points} tone="neutral" />{feedback.suggestion && <p className="suggestion">{feedback.suggestion}</p>}</article>;
}

function Pills({ title, items, tone }: { title: string; items?: string[]; tone: string }) {
  if (!items?.length) return null;
  return <div className="pill-block"><strong>{title}</strong><div>{items.map((item) => <span className={tone} key={item}>{item}</span>)}</div></div>;
}

function ScoreCard({ skill, score }: { skill: string; score: number }) {
  return <div className="score-card"><strong>{skill}</strong><span>{score}</span><div className="meter"><i style={{ width: `${clamp(score, 0, 100)}%` }} /></div></div>;
}

function TranscriptPanel({ analysis }: { analysis?: TranscriptAnalysis }) {
  if (!analysis) return null;
  return <section className="transcript-analysis"><div className="analysis-head"><div><p className="eyebrow">回答诊断</p><h2>{analysis.rounds_analyzed} 轮有效答题 · 平均 {analysis.average_answer_chars} 字</h2></div></div><div className="dimension-grid">{analysis.dimensions.map((d) => <article className="dimension-card" key={d.name}><div className="dimension-head"><strong>{d.name}</strong><span>{d.score}</span></div><div className="meter"><i style={{ width: `${clamp(d.score, 0, 100)}%` }} /></div>{d.evidence?.map((item) => <p key={item}>{item}</p>)}{d.advice && <em>{d.advice}</em>}</article>)}</div>{!!analysis.patterns?.length && <div className="pattern-list">{analysis.patterns.map((p) => <span key={p}>{p}</span>)}</div>}</section>;
}

function DrillPlanPanel({ plan, startDrill, jumpQuestion }: { plan: DrillPlanItem[]; startDrill: (plan: DrillPlanItem[]) => void; jumpQuestion: (id: string) => void }) {
  if (!plan.length) return null;
  return <section className="drill-plan"><div className="analysis-head"><div><p className="eyebrow">训练计划</p><h2>下一轮按弱项顺序练，不再泛刷题。</h2></div><button className="secondary drill-start" onClick={() => startDrill(plan)}>按此计划训练</button></div><div className="drill-list">{plan.map((item) => <article className="drill-card" key={`${item.practice_order}-${item.skill}`}><div className="drill-order">{item.practice_order}</div><div><div className="drill-head"><strong>{item.skill || "综合表达"}</strong><span>目标 {item.target_score || 75} 分</span></div><p>{item.reason}</p><div className="recommended-question-ids">{item.recommended_question_ids?.map((id) => <button key={id} onClick={() => jumpQuestion(id)}>题库题 {id}</button>)}</div><ul>{item.recommended_questions?.map((q) => <li key={q}>{q}</li>)}</ul></div></article>)}</div></section>;
}

function ListSection({ title, items }: { title: string; items?: string[] }) {
  return <section><h2>{title}</h2><ul>{items?.length ? items.map((item) => <li key={item}>{item}</li>) : <li>暂无</li>}</ul></section>;
}

function RoundReview({ round }: { round: InterviewRound }) {
  return <article className="review-card"><div className="review-head"><strong>第 {round.number} 题</strong>{round.feedback && <span>{round.feedback.score} 分</span>}</div><p className="review-question">{round.question?.content}</p>{round.answer && <p className="review-answer">{round.answer}</p>}{round.feedback && <FeedbackCard feedback={round.feedback} />}</article>;
}

function QuestionCard({ item, active, onClick }: { item: QuestionBankItem; active: boolean; onClick: () => void }) {
  return <button className={`question-card ${active ? "active" : ""}`} onClick={onClick}><span className="question-card-meta">{item.id} · {item.skill_category || "未分类"} · 难度 {item.difficulty || "-"}</span><strong>{item.content}</strong><span className="question-card-foot">{item.scenario || "general"}</span><div className="question-tags">{item.tags?.slice(0, 4).map((tag) => <span key={tag}>{tag}</span>)}</div></button>;
}

function QuestionDetail({ item, admin }: { item?: QuestionBankItem; admin: boolean }) {
  if (!item) return <aside className="question-detail"><div className="empty-state">选择一道题查看详情</div></aside>;
  return <aside className="question-detail"><div className="question-detail-head"><span>{item.id}</span><strong>{item.skill_category || "未分类"}</strong></div><p className="question-detail-content">{item.content}</p><div className="question-detail-meta"><span>难度 {item.difficulty || "-"}</span><span>{item.scenario || "general"}</span><span>{item.source || "manual"}</span></div><div className="question-tags detail">{item.tags?.map((tag) => <span key={tag}>{tag}</span>)}</div>{admin ? <><section><h3>向量状态</h3><p>{item.embedding_status || "pending"}{item.embedding_model ? ` · ${item.embedding_model}` : ""}</p>{item.embedding_error && <p>{item.embedding_error}</p>}</section><DetailList title="评分要点" items={item.expected_points} /><Rubric rubric={item.rubric} />{item.sample_answer && <section><h3>参考回答</h3><p>{item.sample_answer}</p></section>}<DetailList title="追问提示" items={item.follow_up_hints} /></> : <p className="question-locked">候选人视图已隐藏答案和评分标准。</p>}</aside>;
}

function DetailList({ title, items }: { title: string; items?: string[] }) {
  if (!items?.length) return null;
  return <section><h3>{title}</h3><ul>{items.map((item) => <li key={item}>{item}</li>)}</ul></section>;
}

function Rubric({ rubric }: { rubric?: Record<string, string> }) {
  const entries = Object.entries(rubric || {});
  if (!entries.length) return null;
  return <section><h3>评分规则</h3><dl className="rubric-list">{entries.map(([key, value]) => <div key={key}><dt>{key}</dt><dd>{value}</dd></div>)}</dl></section>;
}

function Select({ value, onChange, label, values, format = (v: string) => v }: { value: string; onChange: (value: string) => void; label: string; values: Record<string, number>; format?: (value: string) => string }) {
  const keys = useMemo(() => Object.keys(values).sort((a, b) => String(a).localeCompare(String(b), "zh-CN", { numeric: true })), [values]);
  return <select value={value} onChange={(evt) => onChange(evt.target.value)}><option value="">{label}</option>{keys.map((key) => <option key={key} value={key}>{format(key)} ({values[key]})</option>)}</select>;
}

function EmptyPage({ title, action, onAction }: { title: string; action: string; onAction: () => void }) {
  return <section className="page empty-page"><h1>{title}</h1><button className="primary" onClick={onAction}>{action}</button></section>;
}

function shouldShowCurrent(question: InterviewQuestion | undefined, rounds: InterviewRound[]) {
  const last = rounds[rounds.length - 1];
  return question && (!last || last.question?.content !== question.content);
}

function errorMessage(err: unknown): string {
  return err instanceof Error ? err.message : String(err);
}

function modeLabel(mode: Mode) {
  return mode === "practice" ? "模拟" : "考试";
}

function importSourceLabel(source: string) {
  return source === "document" ? "文档生成" : "本地题库";
}

function formatImportField(value: unknown): string {
  if (value == null || value === "") return "";
  if (Array.isArray(value)) return value.join(" / ");
  if (typeof value === "object") {
    return Object.entries(value as Record<string, string>)
      .map(([key, val]) => `${key}: ${val}`)
      .join(" / ");
  }
  return String(value);
}

function formatTime(value: string) {
  if (!value) return "-";
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) return "-";
  return date.toLocaleString();
}

function formatYearsGap(value: number) {
  if (value > 0) return `+${value} 年`;
  if (value < 0) return `${value} 年`;
  return "0 年";
}

function clamp(n: number, min: number, max: number) {
  return Math.max(min, Math.min(max, Number(n) || 0));
}

createRoot(document.getElementById("root")!).render(<App />);
