import type { DrillPlanItem, RetrievalTrace } from "./types";

export function drillQuestionIds(plan: DrillPlanItem[]): string[] {
  const seen = new Set<string>();
  const out: string[] = [];
  for (const item of plan) {
    for (const id of item.recommended_question_ids || []) {
      const value = id.trim();
      if (!value || seen.has(value)) continue;
      seen.add(value);
      out.push(value);
    }
  }
  return out;
}

export function drillPlanSummary(plan: DrillPlanItem[]): string {
  const skills = plan.map((item) => item.skill?.trim()).filter((skill): skill is string => Boolean(skill));
  const ids = drillQuestionIds(plan);
  return `${plan.length} 个弱项 · ${ids.length} 道题 · ${skills.slice(0, 3).join(" / ") || "综合表达"}`;
}

export function retrievalTraceSummary(trace?: RetrievalTrace): string {
  if (!trace) return "暂无检索记录";
  const stageCount = trace.stages?.length || 0;
  const finalCount = trace.final?.length || 0;
  const fallbackCount = trace.fallback_reasons?.length || 0;
  return `${stageCount} 个阶段 · ${finalCount} 个最终候选 · ${fallbackCount} 个降级信号`;
}
