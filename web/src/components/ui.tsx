import type { ReactNode } from "react";
import type { Actor, ActorKind, Decision, DecisionStatus } from "../lib/types";
import { actorLabel, percentages } from "../lib/format";
import { mark, wordmark } from "./brand";

export function Logo({ size = 28 }: { size?: number }) {
  return (
    <svg width={size} height={size} viewBox={mark.viewBox} role="img" aria-label="Sigil">
      <path d={mark.path} fill="var(--signal)" fillRule="evenodd" />
    </svg>
  );
}

export function Wordmark({ height = 14 }: { height?: number }) {
  return (
    <svg height={height} viewBox={wordmark.viewBox} role="img" aria-label="Sigil" style={{ display: "block" }}>
      <path d={wordmark.path} fill="currentColor" fillRule="evenodd" />
    </svg>
  );
}

export function Badge({ value, pulse = false }: { value: string; pulse?: boolean }) {
  return (
    <span className={`badge badge-${value}`}>
      <span className={`dot${pulse ? " pulse" : ""}`} />
      {value}
    </span>
  );
}

export function ActorChip({ actor }: { actor: Actor }) {
  return (
    <span
      className={`chip chip-${actor.kind}`}
      title={actor.operator ? `on behalf of ${actor.operator}` : actor.id}
    >
      {actor.kind === "agent" ? "◆" : "●"} {actorLabel(actor)}
    </span>
  );
}

export function AttributionBar({
  lines,
  showLegend = true,
}: {
  lines?: Partial<Record<ActorKind, number>>;
  showLegend?: boolean;
}) {
  const p = percentages(lines);
  return (
    <div className="grid" style={{ gap: 6 }}>
      <div className="attr-bar" aria-label={`human ${p.human}%, agent ${p.agent}%`}>
        <div className="human" style={{ width: `${p.human}%` }} />
        <div className="agent" style={{ width: `${p.agent}%` }} />
      </div>
      {showLegend && (
        <div className="legend">
          <span>
            <i style={{ background: "var(--human)" }} />
            human {lines?.human ?? 0}
          </span>
          <span>
            <i style={{ background: "var(--agent)" }} />
            agent {lines?.agent ?? 0}
          </span>
        </div>
      )}
    </div>
  );
}

export function Empty({ title, children }: { title: string; children?: ReactNode }) {
  return (
    <div className="empty">
      <h3>{title}</h3>
      {children}
    </div>
  );
}

export function ErrorBox({ error }: { error?: string }) {
  if (!error) return null;
  return <div className="error-box">{error}</div>;
}

export function Modal({
  title,
  children,
  onClose,
}: {
  title: string;
  children: ReactNode;
  onClose: () => void;
}) {
  return (
    <div className="modal-backdrop" onMouseDown={(e) => e.target === e.currentTarget && onClose()}>
      <div className="modal" role="dialog" aria-modal="true" aria-label={title}>
        <h2>{title}</h2>
        {children}
      </div>
    </div>
  );
}

export function Decisions({ decisions }: { decisions?: (Decision | DecisionStatus)[] }) {
  if (!decisions || decisions.length === 0) {
    return (
      <div className="row small">
        <Badge value="pass" /> <span className="muted">No policy rules triggered</span>
      </div>
    );
  }
  return (
    <div className="grid" style={{ gap: 10 }}>
      {decisions.map((d) => {
        const status = "satisfied" in d ? d : undefined;
        return (
          <div key={d.rule} className="card card-pad" style={{ padding: "10px 12px" }}>
            <div className="row spread">
              <span className="mono">{d.rule}</span>
              <div className="row">
                {status && d.effect === "require-approval" && (
                  <Badge value={status.satisfied ? "pass" : "fail"} />
                )}
                <Badge value={d.effect} />
              </div>
            </div>
            {d.message && (
              <div className="small" style={{ marginTop: 4 }}>
                {d.message}
              </div>
            )}
            {d.effect === "require-approval" && (
              <div className="small muted">
                needs {d.approvals} approval{d.approvals === 1 ? "" : "s"}
                {d.roles?.length ? ` from ${d.roles.join(" / ")}` : ""}
                {status?.approvers?.length ? ` · approved by ${status.approvers.join(", ")}` : ""}
              </div>
            )}
            {d.paths && d.paths.length > 0 && (
              <div className="small muted mono" style={{ marginTop: 4 }}>
                {d.paths.join("  ")}
              </div>
            )}
          </div>
        );
      })}
    </div>
  );
}
