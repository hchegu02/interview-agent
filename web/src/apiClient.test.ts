import { afterEach, describe, expect, it, vi } from "vitest";
import { ApiError, apiClient } from "./apiClient";

describe("apiClient", () => {
  afterEach(() => {
    vi.restoreAllMocks();
  });

  it("deletes an interview session for the current user", async () => {
    const fetchMock = vi.fn().mockResolvedValue({
      ok: true,
      text: () => Promise.resolve(`{"deleted":true}`),
    });
    vi.stubGlobal("fetch", fetchMock);

    await expect(apiClient.deleteSession("s1", "u1")).resolves.toEqual({ deleted: true });

    expect(fetchMock).toHaveBeenCalledWith("/api/interview/sessions/s1?user_id=u1", expect.objectContaining({
      method: "DELETE",
    }));
  });

  it("sends an agent message to the backend router", async () => {
    const fetchMock = vi.fn().mockResolvedValue({
      ok: true,
      text: () => Promise.resolve(`{"intent":"skill.quiz","skill":"quiz","confidence":0.9,"reason":"matched","result":{"title":"专项测验","content":"请回答"},"tool_trace":[{"name":"github.project_analyze","permission":"read_only","status":"success","elapsed_ms":4,"summary":"loaded"}]}`),
    });
    vi.stubGlobal("fetch", fetchMock);

    await expect(apiClient.sendAgentMessage({ user_id: "u1", message: "考我 Redis" })).resolves.toEqual({
      intent: "skill.quiz",
      skill: "quiz",
      confidence: 0.9,
      reason: "matched",
      result: { title: "专项测验", content: "请回答" },
      tool_trace: [{ name: "github.project_analyze", permission: "read_only", status: "success", elapsed_ms: 4, summary: "loaded" }],
    });

    expect(fetchMock).toHaveBeenCalledWith("/api/agent/message", expect.objectContaining({
      method: "POST",
      body: JSON.stringify({ user_id: "u1", message: "考我 Redis" }),
    }));
  });

  it("loads a read-only user memory profile", async () => {
    const fetchMock = vi.fn().mockResolvedValue({
      ok: true,
      text: () => Promise.resolve(`{"user_id":"u1","strengths":["Go"],"skill_scores":{"Go":82}}`),
    });
    vi.stubGlobal("fetch", fetchMock);

    await expect(apiClient.getUserMemory("u1")).resolves.toEqual({
      user_id: "u1",
      strengths: ["Go"],
      skill_scores: { Go: 82 },
    });

    expect(fetchMock).toHaveBeenCalledWith("/api/users/u1/memory", expect.objectContaining({
      headers: expect.objectContaining({ "X-User-ID": "u1" }),
    }));
  });

  it("preserves HTTP status on API errors", async () => {
    vi.stubGlobal("fetch", vi.fn().mockResolvedValue({
      ok: false,
      status: 404,
      statusText: "Not Found",
      text: () => Promise.resolve(`{"error":"user memory not found"}`),
    }));

    await expect(apiClient.getUserMemory("missing")).rejects.toMatchObject({
      status: 404,
      message: "user memory not found",
    });

    await expect(apiClient.getUserMemory("missing")).rejects.toBeInstanceOf(ApiError);
  });
});
