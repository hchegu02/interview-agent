import { useMemo } from "react";

export function PageHeader({ eyebrow, title, copy }: { eyebrow: string; title: string; copy?: string }) {
  return <header className="page-header"><p className="eyebrow">{eyebrow}</p><h1>{title}</h1>{copy && <p>{copy}</p>}</header>;
}

export function DetailList({ title, items }: { title: string; items?: string[] }) {
  if (!items?.length) return null;
  return <section><h3>{title}</h3><ul>{items.map((item) => <li key={item}>{item}</li>)}</ul></section>;
}

export function Rubric({ rubric }: { rubric?: Record<string, string> }) {
  const entries = Object.entries(rubric || {});
  if (!entries.length) return null;
  return <section><h3>评分规则</h3><dl className="rubric-list">{entries.map(([key, value]) => <div key={key}><dt>{key}</dt><dd>{value}</dd></div>)}</dl></section>;
}

export function Select({ value, onChange, label, values, format = (v: string) => v }: { value: string; onChange: (value: string) => void; label: string; values: Record<string, number>; format?: (value: string) => string }) {
  const keys = useMemo(() => Object.keys(values).sort((a, b) => String(a).localeCompare(String(b), "zh-CN", { numeric: true })), [values]);
  return <select value={value} onChange={(evt) => onChange(evt.target.value)}><option value="">{label}</option>{keys.map((key) => <option key={key} value={key}>{format(key)} ({values[key]})</option>)}</select>;
}

export function EmptyPage({ title, action, onAction }: { title: string; action: string; onAction: () => void }) {
  return <section className="page empty-page"><h1>{title}</h1><button className="primary" onClick={onAction}>{action}</button></section>;
}
