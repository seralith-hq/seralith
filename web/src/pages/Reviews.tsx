import { useState } from "react";
import { post } from "../lib/api";
import { useResource } from "../lib/hooks";
import { href } from "../lib/router";
import { relativeTime, shortSha } from "../lib/format";
import type { ChangePredicate, CommitReport, CommitRow, ProjectInfo } from "../lib/types";
import { ActorChip, AttributionBar, Badge, Decisions, Empty, ErrorBox } from "../components/ui";

export function ReviewsPage({ version }: { version: number }) {
  const { data, error } = useResource<CommitRow[]>("/commits", [version]);
  const counts = { pass: 0, warn: 0, fail: 0 };
  data?.forEach((c) => {
    if (c.status in counts) counts[c.status as keyof typeof counts]++;
  });

  return (
    <div className="page">
      <div className="page-header">
        <div>
          <h1 className="page-title">Reviews</h1>
          <p className="page-subtitle">
            Commits on workspace branches, verified against your policy and trust store.
          </p>
        </div>
      </div>
      <ErrorBox error={error} />
      <div className="stat-grid">
        <Stat label="Passing" value={counts.pass} color="var(--verified)" />
        <Stat label="Warnings" value={counts.warn} color="var(--caution)" />
        <Stat label="Blocked" value={counts.fail} color="var(--breach)" />
      </div>
      <div className="card">
        {data && data.length === 0 ? (
          <Empty title="No workspace commits yet">
            Commit from a workspace to produce an attested change.
          </Empty>
        ) : (
          <table className="data">
            <thead>
              <tr>
                <th>Status</th>
                <th>Commit</th>
                <th>Subject</th>
                <th style={{ width: 200 }}>Attribution</th>
                <th>Branch</th>
                <th>When</th>
              </tr>
            </thead>
            <tbody>
              {data?.map((c) => (
                <tr
                  key={c.commit}
                  className="clickable"
                  onClick={() => (location.hash = href("reviews", c.commit))}
                >
                  <td>
                    <Badge value={c.status} />
                  </td>
                  <td className="mono">{shortSha(c.commit)}</td>
                  <td>{c.subject}</td>
                  <td>
                    <AttributionBar lines={c.lines} showLegend={false} />
                  </td>
                  <td className="mono small muted">{c.branches.join(", ")}</td>
                  <td className="small muted">{relativeTime(c.date)}</td>
                </tr>
              ))}
            </tbody>
          </table>
        )}
      </div>
    </div>
  );
}

function Stat({ label, value, color }: { label: string; value: number; color: string }) {
  return (
    <div className="card stat">
      <div className="stat-value" style={{ color }}>
        {value}
      </div>
      <div className="stat-label">{label}</div>
    </div>
  );
}

export function ReviewPage({ sha, version }: { sha: string; version: number }) {
  const { data, error, reload } = useResource<{ report: CommitReport; predicate?: ChangePredicate }>(
    `/commits/${sha}`,
    [version],
  );
  const project = useResource<ProjectInfo>("/project");
  const report = data?.report;
  const pred = data?.predicate;

  return (
    <div className="page">
      <div className="page-header">
        <div>
          <div className="small muted">
            <a href={href("reviews")}>Reviews</a> /
          </div>
          <h1 className="page-title">{report?.subject ?? shortSha(sha)}</h1>
          <p className="page-subtitle mono">
            {sha} {pred?.branch ? `· ${pred.branch}` : ""}
          </p>
        </div>
        {report && <Badge value={report.status} />}
      </div>
      <ErrorBox error={error} />
      {report && (
        <div className="split">
          <div className="grid" style={{ gap: 14, minWidth: 0 }}>
            {(report.problems?.length ?? 0) > 0 && (
              <div className="card card-pad">
                <strong>Problems</strong>
                <ul className="dels small">
                  {report.problems?.map((p) => (
                    <li key={p}>{p}</li>
                  ))}
                </ul>
              </div>
            )}
            {(report.warnings?.length ?? 0) > 0 && (
              <div className="card card-pad">
                <strong>Warnings</strong>
                <ul className="small" style={{ color: "var(--caution)" }}>
                  {report.warnings?.map((p) => (
                    <li key={p}>{p}</li>
                  ))}
                </ul>
              </div>
            )}
            <div className="card card-pad grid" style={{ gap: 10 }}>
              <strong>Policy decisions</strong>
              <Decisions decisions={report.decisions} />
            </div>
            {pred && (
              <div className="card card-pad grid" style={{ gap: 10 }}>
                <strong>Observed segments</strong>
                {pred.segments.map((s) => (
                  <div key={s.id} className="row spread small">
                    <div className="row">
                      <ActorChip actor={s.actor} />
                      <span className="muted">{s.kind}</span>
                    </div>
                    <span className="mono muted">{(s.files ?? []).map((f) => f.path).join(", ")}</span>
                  </div>
                ))}
              </div>
            )}
          </div>
          <aside className="grid" style={{ gap: 14 }}>
            <div className="card card-pad grid" style={{ gap: 10 }}>
              <strong>Attribution</strong>
              <AttributionBar lines={report.summary?.linesByKind} />
            </div>
            <div className="card card-pad grid small" style={{ gap: 6 }}>
              <strong style={{ fontSize: 14 }}>Attestation</strong>
              <div>
                Signed by <span className="mono">{report.signers?.join(", ") || "—"}</span>
              </div>
              <div className="mono muted" style={{ wordBreak: "break-all" }}>
                {report.attestation ?? "none"}
              </div>
            </div>
            <ApprovalPanel sha={sha} report={report} roles={project.data?.roles ?? []} onDone={reload} />
          </aside>
        </div>
      )}
    </div>
  );
}

function ApprovalPanel({
  sha,
  report,
  roles,
  onDone,
}: {
  sha: string;
  report: CommitReport;
  roles: string[];
  onDone: () => void;
}) {
  const [role, setRole] = useState("");
  const [comment, setComment] = useState("");
  const [error, setError] = useState<string>();

  const submit = async (reject: boolean) => {
    setError(undefined);
    try {
      await post(`/commits/${sha}/approve`, { role: role || undefined, comment, reject });
      setComment("");
      onDone();
    } catch (e) {
      setError(e instanceof Error ? e.message : String(e));
    }
  };

  return (
    <div className="card card-pad grid" style={{ gap: 10 }}>
      <strong>Review</strong>
      {(report.approvals ?? []).map((a, i) => (
        <div key={i} className="row spread small">
          <span>
            {a.approver} {a.role && <span className="muted">as {a.role}</span>}
          </span>
          <Badge value={a.valid ? a.decision : "fail"} />
        </div>
      ))}
      <ErrorBox error={error} />
      <select value={role} onChange={(e) => setRole(e.target.value)} aria-label="Role">
        <option value="">No role</option>
        {roles.map((r) => (
          <option key={r} value={r}>
            {r}
          </option>
        ))}
      </select>
      <textarea
        rows={2}
        placeholder="Review comment"
        value={comment}
        onChange={(e) => setComment(e.target.value)}
      />
      <div className="row">
        <button className="btn btn-primary" onClick={() => void submit(false)}>
          Approve and sign
        </button>
        <button className="btn btn-danger" onClick={() => void submit(true)}>
          Reject
        </button>
      </div>
    </div>
  );
}
