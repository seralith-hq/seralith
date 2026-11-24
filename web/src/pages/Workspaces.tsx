import { useState } from "react";
import { post } from "../lib/api";
import { useResource } from "../lib/hooks";
import { href } from "../lib/router";
import { plural, relativeTime, shortSha } from "../lib/format";
import type { Workspace } from "../lib/types";
import { Badge, Empty, ErrorBox, Modal } from "../components/ui";

export function WorkspacesPage({ version }: { version: number }) {
  const { data, error, reload } = useResource<Workspace[]>("/workspaces", [version]);
  const [creating, setCreating] = useState(false);

  return (
    <div className="page">
      <div className="page-header">
        <div>
          <h1 className="page-title">Workspaces</h1>
          <p className="page-subtitle">
            Isolated worktrees. Run humans and agents in parallel without collisions.
          </p>
        </div>
        <button className="btn btn-primary" onClick={() => setCreating(true)}>
          + New workspace
        </button>
      </div>
      <ErrorBox error={error} />
      {data && data.length === 0 && (
        <div className="card">
          <Empty title="No workspaces yet">
            <p>Create one per task. Each gets its own branch and directory.</p>
            <button className="btn btn-primary" onClick={() => setCreating(true)}>
              Create your first workspace
            </button>
          </Empty>
        </div>
      )}
      <div className="grid grid-cards">
        {data?.map((ws) => (
          <a
            key={ws.name}
            href={href("workspaces", ws.name)}
            className="card card-pad grid"
            style={{ gap: 10 }}
          >
            <div className="row spread">
              <strong style={{ fontSize: 15 }}>{ws.name}</strong>
              {ws.running ? (
                <Badge value="running" pulse />
              ) : ws.segments.length > 0 ? (
                <Badge value="warn" />
              ) : (
                <Badge value="neutral" />
              )}
            </div>
            <div className="mono muted">{ws.branch}</div>
            <div className="row spread small">
              <span>{ws.segments.length > 0 ? plural(ws.segments.length, "pending segment") : "clean"}</span>
              <span className="muted">
                {shortSha(ws.head)} · {relativeTime(ws.updatedAt)}
              </span>
            </div>
          </a>
        ))}
      </div>
      {creating && (
        <NewWorkspaceDialog
          onClose={() => setCreating(false)}
          onCreated={(name) => {
            setCreating(false);
            reload();
            location.hash = href("workspaces", name);
          }}
        />
      )}
    </div>
  );
}

function NewWorkspaceDialog({
  onClose,
  onCreated,
}: {
  onClose: () => void;
  onCreated: (name: string) => void;
}) {
  const [name, setName] = useState("");
  const [base, setBase] = useState("HEAD");
  const [error, setError] = useState<string>();
  const [busy, setBusy] = useState(false);

  const submit = async () => {
    setBusy(true);
    setError(undefined);
    try {
      await post<Workspace>("/workspaces", { name, base });
      onCreated(name);
    } catch (e) {
      setError(e instanceof Error ? e.message : String(e));
    } finally {
      setBusy(false);
    }
  };

  return (
    <Modal title="New workspace" onClose={onClose}>
      <ErrorBox error={error} />
      <div className="field">
        <label htmlFor="ws-name">Name</label>
        <input
          id="ws-name"
          autoFocus
          placeholder="auth-refactor"
          value={name}
          onChange={(e) => setName(e.target.value.toLowerCase().replace(/[^a-z0-9._-]/g, "-"))}
          onKeyDown={(e) => e.key === "Enter" && name && void submit()}
        />
      </div>
      <div className="field">
        <label htmlFor="ws-base">Start from</label>
        <input id="ws-base" value={base} onChange={(e) => setBase(e.target.value)} />
      </div>
      <div className="row" style={{ justifyContent: "flex-end" }}>
        <button className="btn" onClick={onClose}>
          Cancel
        </button>
        <button className="btn btn-primary" disabled={!name || busy} onClick={() => void submit()}>
          Create
        </button>
      </div>
    </Modal>
  );
}
