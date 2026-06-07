import React, { useEffect, useState } from "react";
import { apiClient } from "./apiClient";
import { analysisSummary, draftScopeSummary, normalizeQuestionBankFilter, normalizeResumeSections } from "./draftStore";
import { clamp, formatYearsGap, modeLabel, shouldShowCurrent } from "./interviewView";
import { drillPlanSummary, retrievalTraceSummary } from "./reportView";
import { routes, type Route } from "./routes";
import { EmptyPage, PageHeader, Select } from "./sharedView";
import type {
  AgentResponse,
  Draft,
  DrillPlanItem,
  InterviewFeedback,
  InterviewQuestion,
  InterviewRound,
  ProfileAnalysis,
  ReportFollowUpReview,
  ReportRoundReview,
  RetrievalTrace,
  Session,
  TranscriptAnalysis,
  QuestionFacets,
  SkillAction,
  ToolTrace,
  UserMemory,
  WorkingMemory,
} from "./types";
import type { StreamEvent } from "./useInterviewStream";

export function ResumePage({ draft, busy, updateDraft, resetDraft, goNext, setNotice }: {
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
      updateDraft({ resume_text: data.text || "", resume_sections: normalizeResumeSections(undefined, data.text || "") });
      setNotice(`已读取 ${data.filename || file.name}，可继续编辑。`);
    } catch (err) {
      setNotice(errorMessage(err));
    } finally {
      setUploading(false);
      if (fileRef.current) fileRef.current.value = "";
    }
  };
  const sections = draft.resume_sections || normalizeResumeSections(undefined, draft.resume_text);
  const updateSection = (key: keyof NonNullable<Draft["resume_sections"]>, value: string) => {
    updateDraft({ resume_sections: { ...sections, [key]: value }, analysis: undefined });
  };
  return (
    <section className="page resume-page">
      <PageHeader eyebrow="Step 1 · 简历档案" title="候选人资料" />
      <div className="two-column">
        <div className="resume-sections">
          <label className="field">
            <span>概况 / 年限</span>
            <textarea value={sections.summary} onChange={(evt) => updateSection("summary", evt.target.value)} />
          </label>
          <label className="field">
            <span>技能栈</span>
            <textarea value={sections.skills} onChange={(evt) => updateSection("skills", evt.target.value)} />
          </label>
          <label className="field wide">
            <span>项目经历</span>
            <textarea value={sections.projects} onChange={(evt) => updateSection("projects", evt.target.value)} />
          </label>
          <label className="field">
            <span>亮点 / 成果</span>
            <textarea value={sections.highlights} onChange={(evt) => updateSection("highlights", evt.target.value)} />
          </label>
          <label className="field">
            <span>原文补充</span>
            <textarea value={sections.raw_notes} onChange={(evt) => updateSection("raw_notes", evt.target.value)} />
          </label>
        </div>
        <aside className="control-panel">
          <h2>简历输入</h2>
          <p>上传后会先拆到模块，仍会合成为兼容的简历文本。</p>
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

export function JDPage({ draft, busy, updateDraft, analyze, startInterview }: {
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

export function InterviewPage({ session, events, busy, pendingAnswer, submitAnswer, goJD }: {
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
      <div className="conversation">
        {rounds.map((round) => <RoundView key={round.round_id} round={round} />)}
        {shouldShowCurrent(session.question, rounds) && <QuestionBubble number={(rounds.length || 0) + 1} question={session.question} />}
        {pendingAnswer && <article className="bubble answer pending"><div className="bubble-meta">已提交，等待评分</div><p>{pendingAnswer}</p></article>}
        {busy && <div className="system-line">正在评估回答，准备下一题...</div>}
      </div>
      {session.mode === "practice" && (
        <>
          <AgentStatePanel memory={session.working_memory} />
          <EventTimeline events={events} />
        </>
      )}
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

export function ReportPage({ session, startDrill, jumpQuestion }: {
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
      <ReportRoundReviews session={session} />
      <TranscriptPanel analysis={report.transcript_analysis} />
      <DrillPlanPanel plan={report.drill_plan || []} startDrill={startDrill} jumpQuestion={jumpQuestion} />
      <div className="report-summary">
        <ListSection title="亮点" items={report.highlights} />
        <ListSection title="改进项" items={report.improvements} />
        <ListSection title="下一步" items={report.next_steps} />
      </div>
      {session.mode === "practice" && (
        <>
          <AgentStatePanel memory={session.working_memory} />
          <RetrievalTracePanel trace={session.retrieval_trace} />
        </>
      )}
    </section>
  );
}

export function AgentPage({ userId, busy, setBusy, setNotice, goJD, startAgentDrill }: {
  userId: string;
  busy: boolean;
  setBusy: (busy: boolean) => void;
  setNotice: (message: string) => void;
  goJD: () => void;
  startAgentDrill: (topic: string) => void;
}) {
  const [message, setMessage] = useState("帮我把 GitHub 项目亮点整理成面试表达：https://github.com/hchegu02/interview-agent");
  const [response, setResponse] = useState<AgentResponse | null>(null);
  const send = async (evt: React.FormEvent) => {
    evt.preventDefault();
    const value = message.trim();
    if (!value) {
      setNotice("请输入要交给 Agent 的任务。");
      return;
    }
    setBusy(true);
    setNotice("正在路由意图并执行 Skill...");
    try {
      const next = await apiClient.sendAgentMessage({ user_id: userId.trim(), message: value });
      setResponse(next);
      setNotice("");
    } catch (err) {
      setNotice(errorMessage(err));
    } finally {
      setBusy(false);
    }
  };
  const runAction = (action: SkillAction) => {
    switch (action.type) {
    case "start_interview":
      goJD();
      break;
    case "start_drill":
      startAgentDrill(action.value || message);
      break;
    default:
      setNotice(`当前前端只展示动作：${action.label}`);
    }
  };
  return (
    <section className="page agent-page">
      <PageHeader eyebrow="Agent Skill" title="把零散需求路由到专项训练能力" copy="前端只提交消息并展示后端结果；意图判断、Skill 执行和工具边界都在后端。" />
      <div className="two-column">
        <form className="agent-composer" onSubmit={send}>
          <label className="field tall">
            <span>任务消息</span>
            <textarea value={message} onChange={(evt) => setMessage(evt.target.value)} />
          </label>
          <button className="primary" disabled={busy || !message.trim()}>{busy ? "处理中" : "发送给 Agent"}</button>
        </form>
        <aside className="control-panel agent-examples">
          <h2>可处理的任务</h2>
          <button className="secondary" onClick={() => setMessage("考我 Redis 缓存击穿和热点 key")}>专项测验</button>
          <button className="secondary" onClick={() => setMessage("解释一下 Go GMP 调度和 work stealing")}>知识讲解</button>
          <button className="secondary" onClick={() => setMessage("帮我润色项目亮点：https://github.com/hchegu02/interview-agent")}>项目润色</button>
          <button className="secondary" onClick={() => setMessage("开始模拟面试")}>面试入口</button>
        </aside>
      </div>
      {response && (
        <section className="agent-result">
          <div className="agent-route">
            <span>{response.intent}</span>
            {response.skill && <span>{response.skill}</span>}
            <span>{Math.round(response.confidence * 100)}%</span>
          </div>
          <article className="feedback-card">
            <div className="review-head">
              <strong>{response.result.title || "Agent 结果"}</strong>
              <span>{response.reason}</span>
            </div>
            <p>{response.result.content}</p>
            {!!response.result.actions?.length && (
              <div className="agent-actions">
                {response.result.actions.map((action) => (
                  <button key={`${action.type}-${action.label}-${action.value || ""}`} className="secondary" onClick={() => runAction(action)}>
                    {action.label}
                  </button>
                ))}
              </div>
            )}
          </article>
          <AgentToolTracePanel traces={response.tool_trace} />
        </section>
      )}
    </section>
  );
}

export function AgentToolTracePanel({ traces }: { traces?: ToolTrace[] }) {
  if (!traces?.length) return null;
  return (
    <section className="agent-tool-trace">
      <div className="review-head">
        <strong>工具调用 Trace</strong>
        <span>{traces.length} 次调用</span>
      </div>
      <div className="tool-trace-list">
        {traces.map((trace, index) => (
          <article key={`${trace.name}-${index}`} className={`tool-trace-item ${trace.status || "unknown"}`}>
            <div>
              <strong>{trace.name || "unknown_tool"}</strong>
              <span>{trace.permission || "permission_unknown"}</span>
            </div>
            <div>
              <span className="tool-trace-status">{trace.status || "unknown"}</span>
              {trace.error_class && <span>{trace.error_class}</span>}
              {typeof trace.elapsed_ms === "number" && <span>{trace.elapsed_ms}ms</span>}
            </div>
            {trace.summary && <p>{trace.summary}</p>}
          </article>
        ))}
      </div>
    </section>
  );
}

export function UserMemoryPage({ memory, busy, refresh }: {
  memory: UserMemory | null;
  busy: boolean;
  refresh: () => void;
}) {
  return (
    <section className="page memory-page">
      <PageHeader eyebrow="Memory" title="长期用户画像" copy="这里只读展示跨 Session 沉淀出的稳定强项、弱项、技能分数和复习建议。" />
      <div className="memory-toolbar">
        <button className="secondary" disabled={busy} onClick={refresh}>{busy ? "刷新中" : "刷新画像"}</button>
      </div>
      <UserMemoryPanel memory={memory} />
    </section>
  );
}

export function ProgressBar({ session, route }: { session: Session | null; route: Route }) {
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

export function UserMemoryPanel({ memory }: { memory?: UserMemory | null }) {
  const strengths = memory?.strengths?.filter(Boolean) || [];
  const weaknesses = memory?.weaknesses?.filter((item) => item.topic || item.evidence) || [];
  const skillScores = Object.entries(memory?.skill_scores || {}).sort((a, b) => b[1] - a[1]);
  const advice = memory?.last_advice?.filter(Boolean) || [];
  const hasMemory = strengths.length || weaknesses.length || skillScores.length || advice.length || memory?.updated_at;
  return (
    <section className="user-memory">
      <div className="analysis-head">
        <div>
          <p className="eyebrow">长期用户画像</p>
          <h2>{hasMemory ? "跨 Session 沉淀的稳定信号。" : "暂无长期画像数据"}</h2>
        </div>
        {memory?.updated_at && <span className="memory-updated">{memory.updated_at}</span>}
      </div>
      {hasMemory ? (
        <div className="user-memory-grid">
          <StatePills title="优势" items={strengths} empty="暂无优势记录" tone="good" />
          <section className="state-block">
            <h3>弱项</h3>
            <div className="memory-weaknesses">
              {weaknesses.length ? weaknesses.map((item) => (
                <article key={`${item.topic}-${item.evidence || ""}`}>
                  <strong>{item.topic || "未命名弱项"}</strong>
                  <span>严重度 {item.severity ?? "-"}</span>
                  {item.evidence && <p>{item.evidence}</p>}
                  {item.updated_at && <em>{item.updated_at}</em>}
                </article>
              )) : <em>暂无弱项记录</em>}
            </div>
          </section>
          <StateCoverage items={skillScores} />
          <section className="state-block">
            <h3>最近建议</h3>
            <div className="memory-advice">
              {advice.length ? advice.map((item) => <p key={item}>{item}</p>) : <em>暂无最近建议</em>}
            </div>
          </section>
        </div>
      ) : (
        <p className="memory-empty">完成更多面试后再展示稳定弱项和建议。</p>
      )}
    </section>
  );
}

function EventTimeline({ events }: { events: StreamEvent[] }) {
  return <div className="event-timeline">{events.length ? events.map((event, i) => <span key={`${event.type}-${i}`} className="event-chip" title={event.type}><strong>{event.label}</strong><em>{event.detail || event.at}</em></span>) : <span className="event-chip muted">等待实时事件</span>}</div>;
}

function AgentStatePanel({ memory }: { memory?: WorkingMemory }) {
  if (!memory) return null;
  const difficulty = difficultyLabel(memory.difficulty?.current);
  const remainingRounds = Math.max((memory.max_rounds || 0) - (memory.rounds_asked || 0), 0);
  const weakSkills = memory.weak_skills || [];
  const coverage = Object.entries(memory.skill_coverage || {}).sort((a, b) => b[1] - a[1]);
  const degradedReasons = Object.entries(memory.degraded_reasons || {});
  return (
    <section className="agent-state">
      <div className="analysis-head">
        <div>
          <p className="eyebrow">Agent 状态</p>
          <h2>当前训练策略由后端 WorkingMemory 驱动。</h2>
        </div>
        <span className={`difficulty-pill ${difficulty.className}`}>{difficulty.label}</span>
      </div>
      <div className="agent-state-grid">
        <StateMetric label="已问 / 剩余" value={`${memory.rounds_asked || 0} / ${remainingRounds}`} />
        <StateMetric label="均分 / 有效轮" value={`${formatMemoryScore(memory.avg_score)} / ${memory.scored_rounds || 0}`} />
        <StateMetric label="追问预算" value={`${memory.probes_used || 0} / ${memory.max_probes || 0}`} />
        <StateMetric label="反思预算" value={`${memory.reflections_used || 0} / ${memory.max_reflections || 0}`} />
        <StateMetric label="高分 streak" value={String(memory.difficulty?.correct_streak || 0)} />
        <StateMetric label="低分 streak" value={String(memory.difficulty?.wrong_streak || 0)} />
      </div>
      <div className="agent-state-details">
        <StatePills title="当前弱项" items={weakSkills} empty="暂无低分弱项" tone="warn" />
        <StatePills title="已确认技能" items={memory.confirmed_skills} empty="暂无确认技能" tone="good" />
        <StateCoverage items={coverage} />
        <StateReasons reasons={degradedReasons} />
      </div>
    </section>
  );
}

function StateMetric({ label, value }: { label: string; value: string }) {
  return <div className="state-metric"><span>{label}</span><strong>{value}</strong></div>;
}

function StatePills({ title, items, empty, tone }: { title: string; items?: string[]; empty: string; tone: string }) {
  const values = items?.filter(Boolean) || [];
  return <section className="state-block"><h3>{title}</h3><div className="state-pills">{values.length ? values.map((item) => <span className={tone} key={item}>{item}</span>) : <em>{empty}</em>}</div></section>;
}

function StateCoverage({ items }: { items: [string, number][] }) {
  return (
    <section className="state-block">
      <h3>技能覆盖度</h3>
      <div className="state-coverage">
        {items.length ? items.slice(0, 6).map(([skill, score]) => (
          <span key={skill}>{skill}<strong>{formatMemoryScore(score)}</strong></span>
        )) : <em>暂无覆盖度数据</em>}
      </div>
    </section>
  );
}

function StateReasons({ reasons }: { reasons: [string, string][] }) {
  return (
    <section className="state-block">
      <h3>降级原因</h3>
      <div className="state-reasons">
        {reasons.length ? reasons.map(([component, reason]) => <span key={component}><strong>{component}</strong>{reason}</span>) : <em>暂无降级记录</em>}
      </div>
    </section>
  );
}

function difficultyLabel(value?: number): { label: string; className: string } {
  switch (value) {
  case 1:
    return { label: "基础难度", className: "easy" };
  case 3:
    return { label: "深入难度", className: "hard" };
  default:
    return { label: "进阶难度", className: "medium" };
  }
}

function formatMemoryScore(value?: number): string {
  if (typeof value !== "number" || !Number.isFinite(value)) return "-";
  return Number(value).toFixed(1).replace(/\.0$/, "");
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

function RetrievalTracePanel({ trace }: { trace?: RetrievalTrace }) {
  if (!trace) return null;
  return (
    <section className="retrieval-trace">
      <div className="analysis-head">
        <div>
          <p className="eyebrow">检索链路</p>
          <h2>{retrievalTraceSummary(trace)}</h2>
          <p>{trace.query || "未记录查询词"}</p>
        </div>
      </div>
      {!!trace.fallback_reasons?.length && (
        <div className="trace-fallbacks">
          {trace.fallback_reasons.map((reason) => <span key={reason}>{reason}</span>)}
        </div>
      )}
      {!!trace.stages?.length && (
        <div className="trace-stage-grid">
          {trace.stages.map((stage) => (
            <article className="trace-stage" key={stage.stage}>
              <strong>{stage.stage}</strong>
              <span>{stage.count} 条 · {formatTraceDuration(stage.duration_ms)}</span>
              {stage.error && <em>{stage.error}</em>}
            </article>
          ))}
        </div>
      )}
      {!!trace.final?.length && (
        <div className="trace-final">
          {trace.final.slice(0, 5).map((item) => (
            <article key={`${item.rank}-${item.id}`}>
              <div>
                <strong>#{item.rank} {item.id}</strong>
                <span>{item.stage || "final"} · {formatTraceScore(item.score)}</span>
              </div>
              {item.reason && <p>{item.reason}</p>}
              {!!item.sources && <p>{Object.entries(item.sources).map(([name, rank]) => `${name}:${rank}`).join(" / ")}</p>}
            </article>
          ))}
        </div>
      )}
    </section>
  );
}

function DrillPlanPanel({ plan, startDrill, jumpQuestion }: { plan: DrillPlanItem[]; startDrill: (plan: DrillPlanItem[]) => void; jumpQuestion: (id: string) => void }) {
  if (!plan.length) return null;
  return <section className="drill-plan"><div className="analysis-head"><div><p className="eyebrow">训练计划</p><h2>下一轮按弱项顺序练，不再泛刷题。</h2><p>{drillPlanSummary(plan)}</p></div><button className="secondary drill-start" onClick={() => startDrill(plan)}>按此计划训练</button></div><div className="drill-list">{plan.map((item) => <article className="drill-card" key={`${item.practice_order}-${item.skill}`}><div className="drill-order">{item.practice_order}</div><div><div className="drill-head"><strong>{item.skill || "综合表达"}</strong><span>目标 {item.target_score || 75} 分</span></div><p>{item.reason}</p><div className="recommended-question-ids">{item.recommended_question_ids?.map((id) => <button key={id} onClick={() => jumpQuestion(id)}>题库题 {id}</button>)}</div><ul>{item.recommended_questions?.map((q) => <li key={q}>{q}</li>)}</ul></div></article>)}</div></section>;
}

function formatTraceDuration(ms: number): string {
  if (!Number.isFinite(ms) || ms < 0) return "耗时未知";
  return `${Math.round(ms)}ms`;
}

function formatTraceScore(score: number): string {
  if (!Number.isFinite(score)) return "score -";
  return `score ${score.toFixed(3)}`;
}

function ListSection({ title, items }: { title: string; items?: string[] }) {
  return <section><h2>{title}</h2><ul>{items?.length ? items.map((item) => <li key={item}>{item}</li>) : <li>暂无</li>}</ul></section>;
}

function ReportRoundReviews({ session }: { session: Session }) {
  const reviews = session.report?.round_reviews || [];
  return (
    <section className="round-review">
      <h2>逐题评分</h2>
      {reviews.length
        ? reviews.map((review, index) => <ReportRoundReviewCard key={review.round_id || `${review.question_id || "review"}-${index}`} review={review} fallbackNumber={index + 1} />)
        : (session.rounds || []).map((round) => <RoundReview key={round.round_id} round={round} />)}
    </section>
  );
}

function ReportRoundReviewCard({ review, fallbackNumber }: { review: ReportRoundReview; fallbackNumber: number }) {
  return (
    <article className="review-card">
      <div className="review-head">
        <strong>第 {review.number || fallbackNumber} 题</strong>
        {typeof review.score === "number" && <span>{review.score} 分</span>}
      </div>
      {review.question && <p className="review-question">{review.question}</p>}
      {review.answer && <p className="review-answer">{review.answer}</p>}
      <ReviewEvidence review={review} />
      {!!review.follow_ups?.length && (
        <div className="follow-review-list">
          {review.follow_ups.map((follow, index) => <ReportFollowUpReviewCard key={`${follow.question || "follow"}-${index}`} follow={follow} index={index} />)}
        </div>
      )}
    </article>
  );
}

function ReportFollowUpReviewCard({ follow, index }: { follow: ReportFollowUpReview; index: number }) {
  return (
    <section className="follow-review">
      <div className="review-head">
        <strong>追问 {index + 1}</strong>
        {typeof follow.score === "number" && <span>{follow.score} 分</span>}
      </div>
      {follow.question && <p className="review-question">{follow.question}</p>}
      {follow.answer && <p className="review-answer">{follow.answer}</p>}
      <ReviewEvidence review={follow} />
    </section>
  );
}

function ReviewEvidence({ review }: { review: ReportRoundReview | ReportFollowUpReview }) {
  return (
    <div className="report-review-evidence">
      <Pills title="命中要点" items={review.hit_points} tone="hit" />
      <Pills title="遗漏要点" items={review.missed_points} tone="miss" />
      {"expected_points" in review && <Pills title="参考要点" items={review.expected_points} tone="neutral" />}
      {review.suggestion && <p className="suggestion">{review.suggestion}</p>}
    </div>
  );
}

function RoundReview({ round }: { round: InterviewRound }) {
  return (
    <article className="review-card">
      <div className="review-head">
        <strong>第 {round.number} 题</strong>
        {round.feedback && <span>{round.feedback.score} 分</span>}
      </div>
      <p className="review-question">{round.question?.content}</p>
      {round.answer && <p className="review-answer">{round.answer}</p>}
      {round.feedback && <FeedbackCard feedback={round.feedback} />}
      {!!round.follow_ups?.length && (
        <div className="follow-review-list">
          {round.follow_ups.map((follow, index) => (
            <section className="follow-review" key={`${follow.question}-${index}`}>
              <div className="review-head">
                <strong>追问 {index + 1}</strong>
                {follow.feedback && <span>{follow.feedback.score} 分</span>}
              </div>
              <p className="review-question">{follow.question}</p>
              {follow.answer && <p className="review-answer">{follow.answer}</p>}
              {follow.feedback && <FeedbackCard feedback={follow.feedback} />}
            </section>
          ))}
        </div>
      )}
    </article>
  );
}

function errorMessage(err: unknown): string {
  return err instanceof Error ? err.message : String(err);
}
