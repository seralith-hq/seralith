import { describe, expect, it } from "vitest";
import { parseUnifiedDiff } from "./diff";
import { actorLabel, duration, percentages, relativeTime, shortSha } from "./format";
import { href, parseRoute } from "./router";
import { captureToken } from "./api";

describe("parseUnifiedDiff", () => {
  it("parses files, hunks, and line numbers", () => {
    const patch = [
      "diff --git a/src/a.go b/src/a.go",
      "index 1..2 100644",
      "--- a/src/a.go",
      "+++ b/src/a.go",
      "@@ -1,3 +1,4 @@",
      " package a",
      "-func Old() {}",
      "+func New() {}",
      "+func More() {}",
      " // end",
      "diff --git a/img.png b/img.png",
      "Binary files a/img.png and b/img.png differ",
    ].join("\n");
    const files = parseUnifiedDiff(patch);
    expect(files).toHaveLength(2);
    const [a, img] = files;
    expect(a?.path).toBe("src/a.go");
    expect(a?.added).toBe(2);
    expect(a?.deleted).toBe(1);
    expect(a?.lines.find((l) => l.text === "func More() {}")?.newNo).toBe(3);
    expect(a?.lines.find((l) => l.text === "// end")).toMatchObject({ oldNo: 3, newNo: 4 });
    expect(img?.binary).toBe(true);
  });

  it("returns nothing for an empty patch", () => {
    expect(parseUnifiedDiff("")).toEqual([]);
  });
});

describe("format", () => {
  it("computes attribution percentages that sum to 100", () => {
    expect(percentages({ human: 1, agent: 2 })).toEqual({ human: 33, agent: 67, total: 3 });
    expect(percentages(undefined)).toEqual({ human: 0, agent: 0, total: 0 });
  });

  it("labels actors", () => {
    expect(actorLabel({ kind: "agent", id: "codex", tool: "codex", model: "m" })).toBe("codex · m");
    expect(actorLabel({ kind: "human", id: "dev@example.com" })).toBe("dev@example.com");
  });

  it("formats times", () => {
    const now = new Date("2026-01-01T12:00:00Z");
    expect(relativeTime("2026-01-01T11:58:00Z", now)).toBe("2m ago");
    expect(relativeTime("2025-12-30T12:00:00Z", now)).toBe("2d ago");
    expect(duration("2026-01-01T00:00:00Z", "2026-01-01T00:01:05Z")).toBe("1m 5s");
    expect(shortSha("abcdef0123456789")).toBe("abcdef01");
  });
});

describe("router", () => {
  it("parses routes", () => {
    expect(parseRoute("#/workspaces/auth/changes")).toEqual({
      page: "workspace",
      name: "auth",
      tab: "changes",
    });
    expect(parseRoute("#/workspaces/auth")).toEqual({ page: "workspace", name: "auth", tab: "terminal" });
    expect(parseRoute("#/reviews/abc")).toEqual({ page: "review", sha: "abc" });
    expect(parseRoute("")).toEqual({ page: "workspaces" });
    expect(href("workspaces", "a b")).toBe("#/workspaces/a%20b");
  });
});

describe("captureToken", () => {
  it("extracts and stores the token from the URL hash", () => {
    const store = new Map<string, string>();
    const storage = {
      getItem: (k: string) => store.get(k) ?? null,
      setItem: (k: string, v: string) => void store.set(k, v),
    } as Storage;
    expect(captureToken("#token=abc123", storage)).toBe("abc123");
    expect(captureToken("#/workspaces", storage)).toBe("abc123");
  });
});
