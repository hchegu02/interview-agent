import { useEffect, useMemo, useState } from "react";
import {
  buildImportReviewMetrics,
  commitSummary,
  filterImportItems,
  hasImportAnswerCompleteness,
  importDiffRows,
  importItemReviewFlags,
  importSourceLabel,
  reviewStatus,
  reviewStatusLabel,
  type ImportReviewFilter,
} from "./questionBankImportView";
import { DetailList, Rubric } from "./sharedView";
import type { QuestionBankImportItem, QuestionBankImportJob } from "./types";

type Props = {
  jobs: QuestionBankImportJob[];
  selectedId: string;
  items: QuestionBankImportItem[];
  busy: boolean;
  source: "question_bank" | "document";
  onSourceChange: (source: "question_bank" | "document") => void;
  onSelectJob: (id: string) => void;
  onUploadClick: () => void;
  onCommit: (id: string) => void;
  onReview: (id: string, action: string, itemIds?: string[]) => void;
};

const filters: { key: ImportReviewFilter; label: string }[] = [
  { key: "all", label: "全部" },
  { key: "pending", label: "待补齐" },
  { key: "complete", label: "字段完整" },
  { key: "missing_rubric", label: "缺 Rubric" },
  { key: "missing_expected_points", label: "缺要点" },
  { key: "agent_rejected", label: "Agent 拒绝" },
  { key: "invalid", label: "无效" },
  { key: "accepted", label: "已接受" },
  { key: "rejected", label: "已拒绝" },
];

export function QuestionBankReviewWorkbench({
  jobs,
  selectedId,
  items,
  busy,
  source,
  onSourceChange,
  onSelectJob,
  onUploadClick,
  onCommit,
  onReview,
}: Props) {
  const [query, setQuery] = useState("");
  const [filter, setFilter] = useState<ImportReviewFilter>("all");
  const [selectedItemId, setSelectedItemId] = useState("");
  const [selectedItemIds, setSelectedItemIds] = useState<Set<string>>(new Set());
  const [allowAcceptAll, setAllowAcceptAll] = useState(false);
  const job = jobs.find((item) => item.id === selectedId);
  const filteredItems = useMemo(() => filterImportItems(items, filter, query), [filter, items, query]);
  const selectedItem = items.find((item) => item.id === selectedItemId) || filteredItems[0];
  const metrics = useMemo(() => buildImportReviewMetrics(job, items, selectedItemIds), [items, job, selectedItemIds]);
  const canReview = Boolean(job && job.status === "ready" && !busy);
  const canCommit = Boolean(job && job.status === "ready" && metrics.commitReady > 0 && !busy);
  const summary = commitSummary(job);

  useEffect(() => {
    setSelectedItemIds(new Set());
    setAllowAcceptAll(false);
    setSelectedItemId("");
  }, [selectedId]);

  useEffect(() => {
    if (selectedItemId && items.some((item) => item.id === selectedItemId)) return;
    setSelectedItemId(filteredItems[0]?.id || "");
  }, [filteredItems, items, selectedItemId]);

  const selectedValidIds = Array.from(selectedItemIds).filter((id) => items.some((item) => item.id === id && item.status === "valid"));
  const allVisibleSelected = filteredItems.length > 0 && filteredItems.every((item) => selectedItemIds.has(item.id));

  const toggleItem = (id: string) => {
    setSelectedItemIds((prev) => {
      const next = new Set(prev);
      if (next.has(id)) next.delete(id);
      else next.add(id);
      return next;
    });
  };

  const toggleVisible = () => {
    setSelectedItemIds((prev) => {
      const next = new Set(prev);
      if (allVisibleSelected) filteredItems.forEach((item) => next.delete(item.id));
      else filteredItems.forEach((item) => next.add(item.id));
      return next;
    });
  };

  const reviewSelected = (action: "accept" | "reject") => {
    if (!job || selectedValidIds.length === 0) return;
    onReview(job.id, action, selectedValidIds);
  };

  return (
    <section className="review-workbench">
      <div className="review-command-bar">
        <div>
          <p className="eyebrow">题库导入审核</p>
          <h2>暂存校验、人工抽查、发布入库</h2>
        </div>
        <div className="review-metrics" aria-label="导入审核统计">
          <Metric label="有效" value={metrics.valid} />
          <Metric label="完整" value={metrics.complete} />
          <Metric label="待补齐" value={metrics.incomplete} />
          <Metric label="可发布" value={metrics.commitReady} />
          <Metric label="无效" value={metrics.invalid} tone="warn" />
        </div>
        <div className="review-source-upload">
          <div className="segmented">
            <button className={source === "question_bank" ? "active" : ""} onClick={() => onSourceChange("question_bank")}>本地题库</button>
            <button className={source === "document" ? "active" : ""} onClick={() => onSourceChange("document")}>文档生成</button>
          </div>
          <button className="secondary" disabled={busy} onClick={onUploadClick}>{busy ? "处理中" : "上传导入"}</button>
        </div>
      </div>

      <div className="review-shell">
        <aside className="review-job-rail">
          <div className="review-panel-head">
            <strong>Import Jobs</strong>
            <span>{jobs.length} 个批次</span>
          </div>
          <div className="review-job-list">
            {jobs.length ? jobs.map((item) => (
              <button key={item.id} className={`review-job ${item.id === selectedId ? "active" : ""}`} onClick={() => onSelectJob(item.id)}>
                <span className={`status-pill status-${item.status}`}>{item.status}</span>
                <strong>{item.filename || item.id}</strong>
                <span>{importSourceLabel(item.source_type)} · {item.valid_items}/{item.total_items} 有效 · 已入库 {item.imported_items}</span>
              </button>
            )) : <div className="empty-state">暂无导入任务</div>}
          </div>
        </aside>

        <main className="review-queue">
          <div className="review-panel-head">
            <div>
              <strong>Review Queue</strong>
              <span>{filteredItems.length}/{items.length} 道题</span>
            </div>
            <label className="review-select-all">
              <input type="checkbox" checked={allVisibleSelected} onChange={toggleVisible} />
              选择当前结果
            </label>
          </div>
          <div className="review-filter-bar">
            <input value={query} onChange={(evt) => setQuery(evt.target.value)} placeholder="搜索题干、ID、技能、标签" />
            <div className="review-filter-tabs">
              {filters.map((item) => (
                <button key={item.key} className={filter === item.key ? "active" : ""} onClick={() => setFilter(item.key)}>{item.label}</button>
              ))}
            </div>
          </div>
          <div className="review-batch-bar">
            <span>已选 {metrics.selected}，可操作 {selectedValidIds.length}</span>
            <button className="secondary" disabled={!canReview || selectedValidIds.length === 0} onClick={() => reviewSelected("accept")}>接受选中</button>
            <button className="secondary danger" disabled={!canReview || selectedValidIds.length === 0} onClick={() => reviewSelected("reject")}>拒绝选中</button>
            <button className="secondary" disabled={!canReview} onClick={() => job && onReview(job.id, "accept_complete_valid")}>接受字段完整</button>
          </div>
          <div className="review-item-list">
            {filteredItems.length ? filteredItems.map((item) => (
              <ReviewQueueItem
                key={item.id}
                item={item}
                active={item.id === selectedItem?.id}
                selected={selectedItemIds.has(item.id)}
                onSelect={() => setSelectedItemId(item.id)}
                onToggle={() => toggleItem(item.id)}
              />
            )) : <div className="empty-state">没有符合筛选条件的题目</div>}
          </div>
        </main>

        <aside className="review-inspector">
          <div className="review-panel-head">
            <strong>发布门禁</strong>
            <span>{job?.id || "未选择批次"}</span>
          </div>
          <div className="review-publish-panel">
            <GateLine label="批次状态" value={job?.status || "-"} ok={job?.status === "ready"} />
            <GateLine label="可发布题数" value={String(metrics.commitReady)} ok={metrics.commitReady > 0} />
            <GateLine label="未补齐字段" value={String(metrics.incomplete)} ok={metrics.incomplete === 0} />
            <GateLine label="Agent 拒绝" value={String(metrics.agentRejected)} ok={metrics.agentRejected === 0} />
            <button className="primary" disabled={!canCommit} onClick={() => job && onCommit(job.id)}>提交入库并生成向量</button>
            {summary && (
              <div className="commit-summary">
                <strong>最近提交结果</strong>
                <span>导入 {summary.imported ?? 0} · 跳过 {summary.skipped ?? 0} · 拒绝 {summary.rejected ?? 0}</span>
                <span>向量成功 {summary.embedded ?? 0} · 失败 {summary.embedding_failed ?? 0}</span>
              </div>
            )}
            <div className="review-danger-zone">
              <label><input type="checkbox" checked={allowAcceptAll} onChange={(evt) => setAllowAcceptAll(evt.target.checked)} /> 我确认接受所有 valid 题</label>
              <button className="secondary danger" disabled={!canReview || !allowAcceptAll} onClick={() => job && onReview(job.id, "accept_all_valid")}>接受全部有效</button>
              <button className="secondary danger" disabled={!canReview} onClick={() => job && onReview(job.id, "reject_all_valid")}>拒绝全部有效</button>
            </div>
          </div>
          <ReviewItemDetail item={selectedItem} busy={busy} canReview={canReview} jobId={job?.id} onReview={onReview} />
        </aside>
      </div>
      {job?.error && <p className="system-line error">{job.error}</p>}
    </section>
  );
}

function Metric({ label, value, tone }: { label: string; value: number; tone?: "warn" }) {
  return <span className={tone === "warn" ? "metric warn" : "metric"}><strong>{value}</strong><em>{label}</em></span>;
}

function GateLine({ label, value, ok }: { label: string; value: string; ok: boolean }) {
  return <div className={ok ? "gate-line ok" : "gate-line warn"}><span>{label}</span><strong>{value}</strong></div>;
}

function ReviewQueueItem({ item, active, selected, onSelect, onToggle }: {
  item: QuestionBankImportItem;
  active: boolean;
  selected: boolean;
  onSelect: () => void;
  onToggle: () => void;
}) {
  const flags = importItemReviewFlags(item);
  return (
    <article className={`review-item-row ${active ? "active" : ""} ${flags.invalid ? "invalid" : ""}`}>
      <input type="checkbox" checked={selected} onChange={onToggle} aria-label={`选择 ${item.question_id}`} />
      <button onClick={onSelect}>
        <span>{item.question_id} · {item.item.skill_category || "未分类"} · 难度 {item.item.difficulty || "-"}</span>
        <strong>{item.item.content || "空题干"}</strong>
        <em>{reviewStatusLabel(reviewStatus(item))} · {flags.complete ? "字段完整" : "待补齐"}{item.agent_review_status === "rejected" ? " · Agent 拒绝" : ""}</em>
      </button>
    </article>
  );
}

function ReviewItemDetail({ item, busy, canReview, jobId, onReview }: {
  item?: QuestionBankImportItem;
  busy: boolean;
  canReview: boolean;
  jobId?: string;
  onReview: (id: string, action: string, itemIds?: string[]) => void;
}) {
  if (!item) return <div className="review-item-detail empty-state">选择题目查看详情</div>;
  const flags = importItemReviewFlags(item);
  const rows = importDiffRows(item);
  return (
    <div className="review-item-detail">
      <div className="review-detail-title">
        <span>{item.question_id}</span>
        <strong>{flags.complete ? "字段完整" : "需要人工确认"}</strong>
      </div>
      <p>{item.item.content || "空题干"}</p>
      <div className="review-detail-meta">
        <span>{item.status}</span>
        <span>{reviewStatusLabel(reviewStatus(item))}</span>
        <span>{hasImportAnswerCompleteness(item) ? "完整" : "不完整"}</span>
      </div>
      {item.status === "valid" && (
        <div className="review-detail-actions">
          <button className={reviewStatus(item) === "accepted" ? "active" : ""} disabled={!canReview || busy || !jobId} onClick={() => jobId && onReview(jobId, "accept", [item.id])}>接受</button>
          <button className={reviewStatus(item) === "rejected" ? "active danger" : "danger"} disabled={!canReview || busy || !jobId} onClick={() => jobId && onReview(jobId, "reject", [item.id])}>拒绝</button>
        </div>
      )}
      {item.agent_review_status && (
        <section className="agent-review-note">
          <h3>Agent 审核</h3>
          <p>{item.agent_review_status}{item.agent_review_reason ? `：${item.agent_review_reason}` : ""}</p>
        </section>
      )}
      {!!item.errors?.length && <section className="agent-review-note error"><h3>校验错误</h3><p>{item.errors.join("；")}</p></section>}
      <DetailList title="评分要点" items={item.item.expected_points} />
      <Rubric rubric={item.item.rubric} />
      {item.item.sample_answer && <section><h3>参考回答</h3><p>{item.item.sample_answer}</p></section>}
      <DetailList title="追问提示" items={item.item.follow_up_hints} />
      {!!rows.length && (
        <div className="import-diff review-diff">
          {rows.map((row) => (
            <div key={row.key} className="import-diff-row">
              <span>{row.label}</span>
              <code>{row.before || "空"}</code>
              <code>{row.after || "空"}</code>
              <em>{row.source}</em>
            </div>
          ))}
        </div>
      )}
    </div>
  );
}
