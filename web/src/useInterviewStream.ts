import { useEffect } from "react";
import type { Session } from "./types";

export type StreamEvent = {
  type: string;
  label: string;
  detail: string;
  at: string;
};

const phaseText: Record<string, string> = {
  preparing: "准备题目",
  answering: "等待作答",
  evaluating: "正在评估",
  reporting: "生成报告",
  completed: "已完成",
  failed: "异常中断",
};

export function useInterviewStream(
  sessionId: string,
  userId: string,
  enabled: boolean,
  onSession: (session: Session) => void,
  onEvent: (event: StreamEvent) => void,
) {
  useEffect(() => {
    if (!enabled || !sessionId || !window.EventSource) return;
    let lastEventId = "";
    const params = new URLSearchParams({ session_id: sessionId });
    if (userId) params.set("user_id", userId);
    if (lastEventId) params.set("last_event_id", lastEventId);
    const stream = new EventSource(`/api/interview/stream?${params.toString()}`);
    onEvent(makeEvent("stream.open", "SSE 已连接，等待实时事件"));

    const handle = (evt: MessageEvent<string>) => {
      if (evt.lastEventId) lastEventId = evt.lastEventId;
      const data = JSON.parse(evt.data) as Session & { type?: string; error?: string };
      const type = data.type || evt.type || "message";
      onEvent(makeEvent(type, phaseText[data.phase] || data.status || data.error || "状态已同步"));
      if (data.session_id) onSession(data);
    };

    stream.onmessage = handle;
    for (const name of ["snapshot", "interview.progress", "interview.completed", "interview.failed"]) {
      stream.addEventListener(name, handle as EventListener);
    }
    stream.onerror = () => onEvent(makeEvent("stream.retry", "事件流重连中"));
    return () => stream.close();
  }, [enabled, sessionId, userId, onSession, onEvent]);
}

function makeEvent(type: string, detail: string): StreamEvent {
  return {
    type,
    detail,
    label: eventLabel(type),
    at: new Date().toLocaleTimeString(),
  };
}

function eventLabel(type: string): string {
  switch (type) {
    case "snapshot":
      return "快照";
    case "interview.progress":
      return "进度";
    case "interview.completed":
      return "完成";
    case "interview.failed":
      return "失败";
    case "answer.submitted":
      return "提交";
    case "stream.open":
      return "连接";
    case "stream.retry":
      return "重连";
    default:
      return "事件";
  }
}
