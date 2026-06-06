import { useCallback, useEffect, useRef, useState } from "react";
import { createRoot } from "react-dom/client";
import { ApiError, apiClient } from "./apiClient";
import { buildDraft, clearDraft, DRAFT_KEY, drillJDText, loadDraft, normalizeQuestionBankFilter, saveDraft } from "./draftStore";
import { formatTime, modeLabel } from "./interviewView";
import { AgentPage, InterviewPage, JDPage, ProgressBar, ReportPage, ResumePage, UserMemoryPage } from "./candidatePages";
import { QuestionBankPage } from "./questionBankPage";
import { defaultRouteForWorkspace, navItemsForWorkspace, resolveNavigationState, routes, workspaceForRoute, type Route, type Workspace } from "./routes";
import { useInterviewStream, type StreamEvent } from "./useInterviewStream";
import type {
  Draft,
  DrillPlanItem,
  Mode,
  Session,
  SessionSummary,
  UserMemory,
} from "./types";
import "./styles.css";

const defaultResume = "两年 Go 后端经验，参与过秒杀活动、Redis 缓存优化、Kafka 消费链路和 PostgreSQL 查询优化。";
const defaultJD = "需要 Go 后端工程师，熟悉高并发服务、Redis、PostgreSQL、消息队列和线上问题排查。";

function App() {
  const initialNavigation = resolveNavigationState(window.location.pathname, window.location.search);
  const [route, setRoute] = useState<Route>(() => initialNavigation.route);
  const [workspace, setWorkspace] = useState<Workspace>(() => initialNavigation.workspace);
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
  const [userMemory, setUserMemory] = useState<UserMemory | null>(null);
  const [events, setEvents] = useState<StreamEvent[]>([]);
  const [pendingAnswer, setPendingAnswer] = useState("");
  const [questionJump, setQuestionJump] = useState(() => initialNavigation.questionJump);
  const [deletingSession, setDeletingSession] = useState("");
  const [sidebarCollapsed, setSidebarCollapsed] = useState(false);
  const previousRoute = useRef<Route | null>(null);

  const navigate = useCallback((next: Route, search = "") => {
    window.history.pushState({}, "", `${next}${search}`);
    setRoute(next);
    setWorkspace(workspaceForRoute(next));
    setQuestionJump(new URLSearchParams(search).get("q") || "");
  }, []);

  const switchWorkspace = useCallback((next: Workspace) => {
    setWorkspace(next);
    navigate(defaultRouteForWorkspace(next));
  }, [navigate]);

  useEffect(() => {
    const onPop = () => {
      const next = resolveNavigationState(window.location.pathname, window.location.search);
      setRoute(next.route);
      setWorkspace(next.workspace);
      setQuestionJump(next.questionJump);
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

  const loadUserMemory = useCallback((targetUserId: string) => {
    const uid = targetUserId.trim();
    if (!uid) {
      setUserMemory(null);
      return;
    }
    setBusy(true);
    apiClient.getUserMemory(uid)
      .then((data) => {
        setUserMemory(data);
        setNotice("");
      })
      .catch((err) => {
        setUserMemory(null);
        if (!(err instanceof ApiError && err.status === 404)) {
          setNotice(errorMessage(err));
        }
      })
      .finally(() => setBusy(false));
  }, []);

  const refreshUserMemory = useCallback(() => {
    loadUserMemory(userId);
  }, [loadUserMemory, userId]);

  useEffect(() => {
    const enteredMemory = previousRoute.current !== routes.memory && route === routes.memory;
    previousRoute.current = route;
    if (enteredMemory) loadUserMemory(userId);
  }, [loadUserMemory, route, userId]);

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

  const deleteSession = async (sessionId: string) => {
    const uid = userId.trim();
    if (!uid || !sessionId) return;
    if (!window.confirm("删除这条历史会话？")) return;
    setDeletingSession(sessionId);
    try {
      await apiClient.deleteSession(sessionId, uid);
      if (session?.session_id === sessionId) {
        setSession(null);
        setEvents([]);
        setPendingAnswer("");
      }
      refreshSessions();
    } catch (err) {
      setNotice(errorMessage(err));
    } finally {
      setDeletingSession("");
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

  const startAgentDrill = (topic: string) => {
    const cleanTopic = topic.trim() || "专项训练";
    const nextJD = `${draft.jd_text}\n\n专项训练重点：${cleanTopic}`;
    updateDraft({ jd_text: nextJD, analysis: undefined });
    setNotice("已按 Agent 动作预填专项训练重点。");
    navigate(routes.jd);
  };

  const navItems = navItemsForWorkspace(workspace);

  return (
    <main className={`app-shell ${sidebarCollapsed ? "sidebar-collapsed" : ""}`}>
      <aside className="sidebar">
        <div className="brand">
          <div>
            <strong>{sidebarCollapsed ? "IA" : "InterviewAgent"}</strong>
            <span>面试准备、评估和训练工作台</span>
          </div>
          <button className="sidebar-toggle" onClick={() => setSidebarCollapsed((value) => !value)} aria-label={sidebarCollapsed ? "展开侧栏" : "收起侧栏"}>
            {sidebarCollapsed ? "›" : "‹"}
          </button>
        </div>
        <div className="mode-switch" aria-label="工作区">
          <button className={workspace === "candidate" ? "active" : ""} onClick={() => switchWorkspace("candidate")}>候选人面试</button>
          <button className={workspace === "admin" ? "active" : ""} onClick={() => switchWorkspace("admin")}>管理后台</button>
        </div>
        <nav className="nav-list" aria-label="主导航">
          {navItems.map((item) => (
            <button key={item.route} data-short={item.label.slice(0, 1)} className={route === item.route ? "active" : ""} onClick={() => navigate(item.route)}>
              {item.label}
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
              <div key={item.session_id} className="session-item">
                <button className="session-open" onClick={() => loadSession(item.session_id)}>
                  <strong>{modeLabel(item.mode)} · {item.status}</strong>
                  <span>{formatTime(item.updated_at)}</span>
                </button>
                <button className="session-delete" disabled={deletingSession === item.session_id} onClick={() => deleteSession(item.session_id)}>删除</button>
              </div>
            )) : <div className="empty-state">暂无历史会话</div>}
          </div>
        </section>
        <div className="connection">
          <span className={`dot ${health.startsWith("已") ? "ok" : "bad"}`} />
          <span>{health}</span>
        </div>
      </aside>

      <section className="main">
        {workspace === "candidate" && <ProgressBar session={session} route={route} />}
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
        {route === routes.agent && (
          <AgentPage userId={userId} busy={busy} setBusy={setBusy} setNotice={setNotice} goJD={() => navigate(routes.jd)} startAgentDrill={startAgentDrill} />
        )}
        {route === routes.memory && (
          <UserMemoryPage memory={userMemory} busy={busy} refresh={refreshUserMemory} />
        )}
        {route === routes.questions && <QuestionBankPage jumpId={questionJump} adminDefault={workspace === "admin"} />}
      </section>
    </main>
  );
}

function errorMessage(err: unknown): string {
  return err instanceof Error ? err.message : String(err);
}

createRoot(document.getElementById("root")!).render(<App />);
