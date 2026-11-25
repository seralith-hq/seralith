import type { Actor, ActorKind } from "./types";

export function shortSha(sha: string | undefined, n = 8): string {
  return (sha ?? "").slice(0, n);
}

export function shortId(id: string): string {
  const cut = id.indexOf("_");
  const rest = cut < 0 ? "" : id.slice(cut + 1);
  return rest.length <= 8 ? id : `${id.slice(0, cut)}_${rest.slice(-8)}`;
}

export function relativeTime(iso: string | undefined, now: Date = new Date()): string {
  if (!iso) return "";
  const then = new Date(iso).getTime();
  const seconds = Math.round((now.getTime() - then) / 1000);
  if (Number.isNaN(seconds)) return "";
  if (seconds < 45) return "just now";
  const minutes = Math.round(seconds / 60);
  if (minutes < 60) return `${minutes}m ago`;
  const hours = Math.round(minutes / 60);
  if (hours < 24) return `${hours}h ago`;
  const days = Math.round(hours / 24);
  return `${days}d ago`;
}

export function actorLabel(a: Actor | undefined): string {
  if (!a) return "unknown";
  if (a.kind === "agent") return a.model ? `${a.tool ?? a.id} · ${a.model}` : (a.tool ?? a.id);
  return a.id;
}

export function percentages(lines: Partial<Record<ActorKind, number>> | undefined): {
  human: number;
  agent: number;
  total: number;
} {
  const human = lines?.human ?? 0;
  const agent = lines?.agent ?? 0;
  const total = human + agent;
  if (total === 0) return { human: 0, agent: 0, total };
  const agentPct = Math.round((agent / total) * 100);
  return { human: 100 - agentPct, agent: agentPct, total };
}

export function plural(n: number, word: string): string {
  return `${n} ${word}${n === 1 ? "" : "s"}`;
}

export function duration(start: string, end?: string): string {
  const ms = (end ? new Date(end).getTime() : Date.now()) - new Date(start).getTime();
  const s = Math.max(0, Math.round(ms / 1000));
  if (s < 60) return `${s}s`;
  const m = Math.floor(s / 60);
  if (m < 60) return `${m}m ${s % 60}s`;
  return `${Math.floor(m / 60)}h ${m % 60}m`;
}
