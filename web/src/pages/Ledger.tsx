import { useResource } from "../lib/hooks";
import type { LedgerEntry, LedgerReport } from "../lib/types";
import { Badge, ErrorBox } from "../components/ui";
import { relativeTime } from "../lib/format";

export function LedgerPage({ version }: { version: number }) {
  const entries = useResource<LedgerEntry[]>("/ledger?limit=300", [version]);
  const report = useResource<LedgerReport>("/ledger/verify", [version]);
  const ok = report.data && (report.data.issues?.length ?? 0) === 0;

  return (
    <div className="page">
      <div className="page-header">
        <div>
          <h1 className="page-title">Audit ledger</h1>
          <p className="page-subtitle">
            Append-only and hash-chained. Every entry commits to the one before it.
          </p>
        </div>
        {report.data && (
          <div className="row">
            <Badge value={ok ? "pass" : "fail"} />
            <span className="small muted">
              {report.data.entries} entries · {report.data.verifiedCheckpoints}/{report.data.checkpoints}{" "}
              signed checkpoints
            </span>
          </div>
        )}
      </div>
      <ErrorBox error={entries.error ?? report.error} />
      {report.data?.issues?.map((i) => (
        <div key={i.seq} className="error-box">
          Entry {i.seq}: {i.problem}
        </div>
      ))}
      <div className="card">
        <table className="data">
          <thead>
            <tr>
              <th>#</th>
              <th>Event</th>
              <th>Subject</th>
              <th>Actor</th>
              <th>When</th>
              <th>Hash</th>
            </tr>
          </thead>
          <tbody>
            {[...(entries.data ?? [])].reverse().map((e) => (
              <tr key={e.seq}>
                <td className="mono muted">{e.seq}</td>
                <td className="mono" style={{ color: "var(--signal)" }}>
                  {e.type}
                </td>
                <td className="mono small">{(e.subject ?? "").slice(0, 40)}</td>
                <td className="small">{e.actor}</td>
                <td className="small muted" title={e.time}>
                  {relativeTime(e.time)}
                </td>
                <td className="mono small muted">{e.hash.slice(0, 12)}</td>
              </tr>
            ))}
          </tbody>
        </table>
      </div>
    </div>
  );
}
