import { useMemo } from "react";
import { parseUnifiedDiff } from "../lib/diff";
import { Empty } from "./ui";

export function DiffView({ patch }: { patch: string }) {
  const files = useMemo(() => parseUnifiedDiff(patch), [patch]);
  if (files.length === 0) return <Empty title="No changes">The workspace matches its last commit.</Empty>;
  return (
    <div className="diff">
      {files.map((f) => (
        <div className="diff-file" key={f.path}>
          <div className="diff-file-header">
            <span>
              {f.oldPath && f.oldPath !== f.path ? `${f.oldPath} → ` : ""}
              {f.path}
            </span>
            <span>
              <span className="adds">+{f.added}</span> <span className="dels">−{f.deleted}</span>
            </span>
          </div>
          {f.binary ? (
            <div className="diff-line hunk">
              <span />
              <span />
              <span>binary file</span>
            </div>
          ) : (
            f.lines.map((l, i) => (
              <div className={`diff-line ${l.kind}`} key={i}>
                <span className="no">{l.oldNo ?? ""}</span>
                <span className="no">{l.newNo ?? ""}</span>
                <span>
                  {l.kind === "add" ? "+" : l.kind === "del" ? "−" : l.kind === "context" ? " " : ""}
                  {l.text}
                </span>
              </div>
            ))
          )}
        </div>
      ))}
    </div>
  );
}
