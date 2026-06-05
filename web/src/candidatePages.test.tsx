import React from "react";
import { renderToStaticMarkup } from "react-dom/server";
import { describe, expect, it } from "vitest";
import { ReportPage } from "./candidatePages";
import type { Session } from "./types";

describe("candidate report page", () => {
  it("renders retrieval trace evidence on the report page", () => {
    const session: Session = {
      session_id: "s-1",
      mode: "practice",
      status: "completed",
      phase: "completed",
      report: { session_id: "s-1", overall_score: 82, skill_breakdown: {}, highlights: [], improvements: [], next_steps: [] },
      retrieval_trace: {
        query: "go redis interview",
        stages: [{ stage: "bm25", count: 2, duration_ms: 11 }],
        final: [{ id: "q-redis", rank: 1, score: 0.87, stage: "rrf", sources: { bm25: 1, vector: 2 } }],
        fallback_reasons: ["rerank_timeout"],
      },
      created_at: "2026-01-01T00:00:00Z",
      updated_at: "2026-01-01T00:00:00Z",
    };

    const html = renderToStaticMarkup(<ReportPage session={session} startDrill={() => undefined} jumpQuestion={() => undefined} />);

    expect(html).toContain("检索链路");
    expect(html).toContain("go redis interview");
    expect(html).toContain("q-redis");
    expect(html).toContain("rerank_timeout");
  });
});
