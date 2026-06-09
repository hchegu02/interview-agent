import React, { useCallback, useEffect, useState } from "react";
import { apiClient } from "./apiClient";
import { QuestionBankReviewWorkbench } from "./questionBankReviewWorkbench";
import { DetailList, PageHeader, Rubric, Select } from "./sharedView";
import type { QuestionBankImportJob, QuestionBankImportItem, QuestionBankItem, QuestionFacets } from "./types";

export function QuestionBankPage({ jumpId, adminDefault }: { jumpId: string; adminDefault: boolean }) {
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
  const [admin, setAdmin] = useState(adminDefault);
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

  useEffect(() => {
    setAdmin(adminDefault);
  }, [adminDefault]);

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
      <PageHeader eyebrow="题库工作区" title="题库管理" />
      <input
        ref={importFileRef}
        className="visually-hidden"
        type="file"
        accept=".json,.csv,.md,.markdown,.txt,.pdf,.docx,text/plain,text/markdown,application/json,text/csv,application/pdf,application/vnd.openxmlformats-officedocument.wordprocessingml.document"
        onChange={(evt) => uploadImport(evt.target.files?.[0])}
      />
      <QuestionBankReviewWorkbench
        jobs={imports}
        selectedId={selectedImportId}
        items={importItems}
        busy={importBusy}
        source={importSource}
        onSourceChange={setImportSource}
        onSelectJob={setSelectedImportId}
        onUploadClick={() => importFileRef.current?.click()}
        onCommit={commitImport}
        onReview={reviewImport}
      />
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

function QuestionCard({ item, active, onClick }: { item: QuestionBankItem; active: boolean; onClick: () => void }) {
  return <button className={`question-card ${active ? "active" : ""}`} onClick={onClick}><span className="question-card-meta">{item.id} · {item.skill_category || "未分类"} · 难度 {item.difficulty || "-"}</span><strong>{item.content}</strong><span className="question-card-foot">{item.scenario || "general"}</span><div className="question-tags">{item.tags?.slice(0, 4).map((tag) => <span key={tag}>{tag}</span>)}</div></button>;
}

function QuestionDetail({ item, admin }: { item?: QuestionBankItem; admin: boolean }) {
  if (!item) return <aside className="question-detail"><div className="empty-state">选择一道题查看详情</div></aside>;
  return <aside className="question-detail"><div className="question-detail-head"><span>{item.id}</span><strong>{item.skill_category || "未分类"}</strong></div><p className="question-detail-content">{item.content}</p><div className="question-detail-meta"><span>难度 {item.difficulty || "-"}</span><span>{item.scenario || "general"}</span><span>{item.source || "manual"}</span></div><div className="question-tags detail">{item.tags?.map((tag) => <span key={tag}>{tag}</span>)}</div>{admin && <><section><h3>向量状态</h3><p>{item.embedding_status || "pending"}{item.embedding_model ? ` · ${item.embedding_model}` : ""}</p>{item.embedding_error && <p>{item.embedding_error}</p>}</section><DetailList title="评分要点" items={item.expected_points} /><Rubric rubric={item.rubric} />{item.sample_answer && <section><h3>参考回答</h3><p>{item.sample_answer}</p></section>}<DetailList title="追问提示" items={item.follow_up_hints} /></>}</aside>;
}

function errorMessage(err: unknown): string {
  return err instanceof Error ? err.message : String(err);
}
