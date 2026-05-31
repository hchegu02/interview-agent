import { afterEach, describe, expect, it, vi } from "vitest";
import { apiClient } from "./apiClient";

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
});
