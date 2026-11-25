import { useState } from "react";
import { del, post } from "../lib/api";
import { useResource } from "../lib/hooks";
import { href } from "../lib/router";
import { duration, plural, relativeTime, shortId, shortSha } from "../lib/format";
import type { AgentDetection, FileChange, Preview, SessionInfo } from "../lib/types";
import { ActorChip, AttributionBar, Badge, Decisions, Empty, ErrorBox } from "../components/ui";
import { SessionTerminal } from "../components/Terminal";
import { DiffView } from "../components/DiffView";

const tabs = ["terminal", "changes", "segments"] as const;

export function WorkspacePage({ name, tab, version }: { name: string; tab: string; version: number }) {
  const status = useResource<Preview>(`/workspaces/${encodeURIComponent(name)}/status`, [version]);
  const sessions = useResource<SessionInfo[]>("/sessions", [version]);
  const ws = status.data?.Workspace;
  const live = sessions.data?.find((s) => s.workspace === name && s.state === "running");
  const recent = sessions.data?.filter((s) => s.workspace === name) ?? [];

  return (
    <div className="page">
      <div className="page-header">
        <div>
          <div className="small muted">
            <a href={href("workspaces")}>Workspaces</a> /
          </div>
          <h1 className="page-title">{name}</h1>
          {ws && (
            <p className="page-subtitle mono">
              {ws.branch} · {shortSha(ws.head)} · {ws.path}
            </p>
          )}
        </div>
        <RemoveButton name={name} disabled={!!live} />
      </div>
      <ErrorBox error={status.error} />
      <div className="tabs">
        {tabs.map((t) => (
          <a key={t} href={href("workspaces", name, t)} className={`tab${tab === t ? " active" : ""}`}>
            {t[0]?.toUpperCase() + t.slice(1)}
          </a>
        ))}
      </div>
      <div className="split">
        <div style={{ minWidth: 0 }}>
          {tab === "terminal" && (
            <TerminalTab name={name} live={live} recent={recent} onStarted={sessions.reload} />
          )}
          {tab === "changes" && <ChangesTab name={name} version={version} />}
          {tab === "segments" && <SegmentsTab preview={status.data} />}
        </div>
        <CommitPanel name={name} preview={status.data} running={!!live} onCommitted={status.reload} />
      </div>
    </div>
  );
}

function RemoveButton({ name, disabled }: { name: string; disabled: boolean }) {
  const [error, setError] = useState<string>();
  const remove = async () => {
    if (!confirm(`Remove workspace ${name} and its branch?`)) return;
    try {
      await del(`/workspaces/${encodeURIComponent(name)}`);
      location.hash = href("workspaces");
    } catch (e) {
      const msg = e instanceof Error ? e.message : String(e);
      if (msg.includes("unattested") && confirm(`${msg}\n\nDiscard pending segments and remove anyway?`)) {
        await del(`/workspaces/${encodeURIComponent(name)}?force=true`);
        location.hash = href("workspaces");
        return;
      }
      setError(msg);
    }
  };
  return (
    <div className="grid" style={{ justifyItems: "end", gap: 6 }}>
      <button className="btn btn-danger" disabled={disabled} onClick={() => void remove()}>
        Remove
      </button>
      {error && <span className="small dels">{error}</span>}
    </div>
  );
}

function TerminalTab({
  name,
  live,
  recent,
  onStarted,
}: {
  name: string;
  live?: SessionInfo;
  recent: SessionInfo[];
  onStarted: () => void;
}) {
  const agents = useResource<AgentDetection[]>("/agents");
  const [agent, setAgent] = useState("shell");
  const [model, setModel] = useState("");
  const [command, setCommand] = useState("");
  const [error, setError] = useState<string>();
  const [viewing, setViewing] = useState<SessionInfo>();

  const start = async () => {
    setError(undefined);
    try {
      const info = await post<SessionInfo>("/sessions", {
        workspace: name,
        agent,
        model: model || undefined,
        command: command.trim() ? ["sh", "-c", command.trim()] : undefined,
        cols: 120,
        rows: 32,
      });
      setViewing(info);
      onStarted();
    } catch (e) {
      setError(e instanceof Error ? e.message : String(e));
    }
  };

  const current = live ?? viewing;
  return (
    <div className="grid" style={{ gap: 14 }}>
      <ErrorBox error={error} />
      {!live && (
        <div className="card card-pad row" style={{ flexWrap: "wrap" }}>
          <select value={agent} onChange={(e) => setAgent(e.target.value)} aria-label="Agent">
            {agents.data?.map((a) => (
              <option
                key={a.profile.id}
                value={a.profile.id}
                disabled={!a.installed && a.profile.id !== "shell"}
              >
                {a.profile.name}
                {a.installed ? "" : " (not installed)"}
              </option>
            ))}
          </select>
          <input
            placeholder="declared model (optional)"
            value={model}
            onChange={(e) => setModel(e.target.value)}
            style={{ width: 200 }}
          />
          <input
            placeholder="command (defaults to the tool)"
            value={command}
            onChange={(e) => setCommand(e.target.value)}
            style={{ flex: 1, minWidth: 200 }}
          />
          <button className="btn btn-primary" onClick={() => void start()}>
            ▶ Start session
          </button>
        </div>
      )}
      {current ? (
        <div style={{ height: 520 }}>
          <SessionTerminal key={current.id} session={current} onExit={onStarted} />
        </div>
      ) : (
        <div className="card">
          <Empty title="No session running">
            Start a shell or a coding agent. Everything it changes is attributed to it.
          </Empty>
        </div>
      )}
      {recent.length > 0 && (
        <div className="card">
          <table className="data">
            <thead>
              <tr>
                <th>Session</th>
                <th>Actor</th>
                <th>Duration</th>
                <th>Changes</th>
                <th>State</th>
              </tr>
            </thead>
            <tbody>
              {recent.map((s) => (
                <tr key={s.id} className="clickable" onClick={() => setViewing(s)}>
                  <td className="mono">{shortId(s.id)}</td>
                  <td>
                    <ActorChip actor={s.actor} />
                  </td>
                  <td className="mono small">{duration(s.startedAt, s.endedAt)}</td>
                  <td className="mono small">
                    {plural(s.filesChanged, "file")} <span className="adds">+{s.linesAdded}</span>{" "}
                    <span className="dels">−{s.linesDeleted}</span>
                  </td>
                  <td>
                    <Badge value={s.state} pulse={s.state === "running"} />
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}
    </div>
  );
}

function ChangesTab({ name, version }: { name: string; version: number }) {
  const diff = useResource<{ files: FileChange[]; patch: string; truncated: boolean }>(
    `/workspaces/${encodeURIComponent(name)}/diff`,
    [version],
  );
  return (
    <div>
      <ErrorBox error={diff.error} />
      <div className="row spread" style={{ marginBottom: 12 }}>
        <span className="muted small">
          {diff.data ? plural(diff.data.files.length, "file") + " changed since the last commit" : "Loading…"}
        </span>
        <button className="btn btn-sm" onClick={diff.reload}>
          Refresh
        </button>
      </div>
      {diff.data && <DiffView patch={diff.data.patch} />}
      {diff.data?.truncated && <p className="muted small">Diff truncated at 2 MiB.</p>}
    </div>
  );
}

function SegmentsTab({ preview }: { preview?: Preview }) {
  const segments = preview?.Segments ?? [];
  if (segments.length === 0) {
    return (
      <div className="card">
        <Empty title="No pending segments">Changes appear here after each session or manual edit.</Empty>
      </div>
    );
  }
  return (
    <div className="timeline">
      {segments.map((s) => {
        const add = (s.files ?? []).reduce((n, f) => n + f.linesAdded, 0);
        const rem = (s.files ?? []).reduce((n, f) => n + f.linesDeleted, 0);
        return (
          <div key={s.id} className={`timeline-item ${s.actor.kind}`}>
            <div className="card card-pad grid" style={{ gap: 8 }}>
              <div className="row spread">
                <div className="row">
                  <ActorChip actor={s.actor} />
                  <span className="small muted">{s.kind}</span>
                </div>
                <span className="small muted">{relativeTime(s.endedAt)}</span>
              </div>
              {s.command && <code className="muted">{s.command.join(" ")}</code>}
              <div className="small mono">
                {(s.files ?? []).map((f) => (
                  <div key={f.path} className="row spread">
                    <span>{f.path}</span>
                    <span>
                      <span className="adds">+{f.linesAdded}</span>{" "}
                      <span className="dels">−{f.linesDeleted}</span>
                    </span>
                  </div>
                ))}
              </div>
              <div className="small muted">
                {plural((s.files ?? []).length, "file")} · +{add} −{rem}
              </div>
            </div>
          </div>
        );
      })}
    </div>
  );
}

function CommitPanel({
  name,
  preview,
  running,
  onCommitted,
}: {
  name: string;
  preview?: Preview;
  running: boolean;
  onCommitted: () => void;
}) {
  const [message, setMessage] = useState("");
  const [error, setError] = useState<string>();
  const [result, setResult] = useState<string>();
  const [busy, setBusy] = useState(false);
  const pending = (preview?.Segments ?? []).length;
  const effect = preview?.Effect ?? "allow";

  const commit = async (force = false) => {
    setBusy(true);
    setError(undefined);
    try {
      const res = await post<{ commit: string }>(`/workspaces/${encodeURIComponent(name)}/commit`, {
        message,
        force,
      });
      setResult(res.commit);
      setMessage("");
      onCommitted();
    } catch (e) {
      setError(e instanceof Error ? e.message : String(e));
    } finally {
      setBusy(false);
    }
  };

  return (
    <aside className="grid" style={{ gap: 14 }}>
      <div className="card card-pad grid" style={{ gap: 12 }}>
        <div className="row spread">
          <strong>Attribution</strong>
          <span className="small muted">{plural(pending, "segment")}</span>
        </div>
        <AttributionBar lines={preview?.Summary?.linesByKind} />
      </div>
      <div className="card card-pad grid" style={{ gap: 12 }}>
        <div className="row spread">
          <strong>Policy</strong>
          <Badge value={effect} />
        </div>
        <Decisions decisions={preview?.Policy?.decisions} />
      </div>
      <div className="card card-pad grid" style={{ gap: 10 }}>
        <strong>Commit with attestation</strong>
        <ErrorBox error={error} />
        {result && (
          <div className="small">
            Committed{" "}
            <a className="mono" href={href("reviews", result)}>
              {shortSha(result)}
            </a>{" "}
            and signed its attestation.
          </div>
        )}
        <textarea
          rows={3}
          placeholder="feat(scope): describe the change"
          value={message}
          onChange={(e) => setMessage(e.target.value)}
        />
        <button
          className="btn btn-primary"
          disabled={busy || running || pending === 0 || !message.trim()}
          onClick={() => void commit(false)}
        >
          {running ? "Session running…" : "Commit and sign"}
        </button>
        {effect === "deny" && (
          <button
            className="btn btn-danger btn-sm"
            disabled={busy || !message.trim()}
            onClick={() => void commit(true)}
          >
            Override deny (recorded in ledger)
          </button>
        )}
      </div>
    </aside>
  );
}
