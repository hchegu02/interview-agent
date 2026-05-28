import type {
  ProfileAnalyzeResponse,
  QuestionBankFilter,
  QuestionBankImportItem,
  QuestionBankImportJob,
  QuestionBankItem,
  QuestionFacets,
  Session,
  SessionSummary,
} from "./types";

type RequestOptions = RequestInit & { form?: FormData };

async function api<T>(path: string, options: RequestOptions = {}): Promise<T> {
  const headers = options.form ? options.headers : {
    "Content-Type": "application/json",
    ...(options.headers || {}),
  };
  const res = await fetch(path, {
    ...options,
    headers,
    body: options.form || options.body,
  });
  const text = await res.text();
  const data = text ? JSON.parse(text) : {};
  if (!res.ok) {
    throw new Error(data.error || `${res.status} ${res.statusText}`);
  }
  return data as T;
}

export const apiClient = {
  ping: () => api<{ pong: boolean; llm_mode: string }>("/api/ping"),

  parseResume: (file: File) => {
    const form = new FormData();
    form.append("file", file);
    return api<{ filename: string; text: string }>("/api/documents/parse-resume", {
      method: "POST",
      form,
    });
  },

  analyzeProfile: (jdText: string, resumeText: string) => api<ProfileAnalyzeResponse>("/api/profile/analyze", {
    method: "POST",
    body: JSON.stringify({ jd_text: jdText, resume_text: resumeText }),
  }),

  startInterview: (payload: { user_id: string; mode: string; jd_text: string; resume_text: string; question_bank_filter?: QuestionBankFilter }) =>
    api<Session>("/api/interview/start", {
      method: "POST",
      body: JSON.stringify(payload),
    }),

  answerInterview: (payload: { session_id: string; user_id: string; answer: string }) =>
    api<Session>("/api/interview/answer", {
      method: "POST",
      body: JSON.stringify(payload),
    }),

  loadSession: (sessionId: string, userId: string) => {
    const params = userId ? `?user_id=${encodeURIComponent(userId)}` : "";
    return api<Session>(`/api/interview/sessions/${encodeURIComponent(sessionId)}${params}`);
  },

  listSessions: (userId: string) =>
    api<{ sessions: SessionSummary[] }>(`/api/interview/sessions?user_id=${encodeURIComponent(userId)}&limit=20`),

  questionFacets: () => api<QuestionFacets>("/api/question-bank/facets"),

  questionBank: (params: URLSearchParams) =>
    api<{ items: QuestionBankItem[]; next_cursor?: string; limit: number }>(`/api/question-bank?${params.toString()}`),

  listQuestionImports: () =>
    api<{ jobs: QuestionBankImportJob[] }>("/api/question-bank/imports"),

  getQuestionImport: (id: string) =>
    api<{ job: QuestionBankImportJob; items?: QuestionBankImportItem[] }>(`/api/question-bank/imports/${encodeURIComponent(id)}`),

  createQuestionImport: (sourceType: "question_bank" | "document", file: File, async = true) => {
    const form = new FormData();
    form.append("source_type", sourceType);
    form.append("file", file);
    const suffix = async ? "?async=true" : "";
    return api<{ job: QuestionBankImportJob }>(`/api/question-bank/imports${suffix}`, {
      method: "POST",
      form,
    });
  },

  commitQuestionImport: (id: string, async = true) =>
    api<{ job: QuestionBankImportJob }>(`/api/question-bank/imports/${encodeURIComponent(id)}/commit${async ? "?async=true" : ""}`, {
      method: "POST",
    }),
};
