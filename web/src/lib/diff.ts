export type DiffLineKind = "context" | "add" | "del" | "hunk" | "meta";

export interface DiffLine {
  kind: DiffLineKind;
  text: string;
  oldNo?: number;
  newNo?: number;
}

export interface DiffFile {
  path: string;
  oldPath?: string;
  added: number;
  deleted: number;
  binary: boolean;
  lines: DiffLine[];
}

const hunkHeader = /^@@ -(\d+)(?:,(\d+))? \+(\d+)(?:,(\d+))? @@/;

export function parseUnifiedDiff(patch: string): DiffFile[] {
  const files: DiffFile[] = [];
  let current: DiffFile | undefined;
  let oldNo = 0;
  let newNo = 0;
  for (const raw of patch.split("\n")) {
    if (raw.startsWith("diff --git ")) {
      const m = /^diff --git a\/(.+) b\/(.+)$/.exec(raw);
      current = { path: m?.[2] ?? raw, oldPath: m?.[1], added: 0, deleted: 0, binary: false, lines: [] };
      files.push(current);
      continue;
    }
    if (!current) continue;
    if (raw.startsWith("@@")) {
      const m = hunkHeader.exec(raw);
      oldNo = Number(m?.[1] ?? 0);
      newNo = Number(m?.[3] ?? 0);
      current.lines.push({ kind: "hunk", text: raw });
    } else if (raw.startsWith("Binary files")) {
      current.binary = true;
    } else if (raw.startsWith("+++ ") || raw.startsWith("--- ")) {
      if (raw.startsWith("+++ b/")) current.path = raw.slice(6);
    } else if (raw.startsWith("+")) {
      current.added++;
      current.lines.push({ kind: "add", text: raw.slice(1), newNo: newNo++ });
    } else if (raw.startsWith("-")) {
      current.deleted++;
      current.lines.push({ kind: "del", text: raw.slice(1), oldNo: oldNo++ });
    } else if (raw.startsWith(" ")) {
      current.lines.push({ kind: "context", text: raw.slice(1), oldNo: oldNo++, newNo: newNo++ });
    } else if (raw.startsWith("\\")) {
      current.lines.push({ kind: "meta", text: raw });
    }
  }
  return files;
}
