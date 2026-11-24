import { useCallback, useEffect, useRef, useState } from "react";
import { api, eventsURL } from "./api";

export interface Resource<T> {
  data: T | undefined;
  error: string | undefined;
  loading: boolean;
  reload: () => void;
}

export function useResource<T>(path: string | null, deps: unknown[] = []): Resource<T> {
  const [tick, setTick] = useState(0);
  const reload = useCallback(() => setTick((t) => t + 1), []);
  const key = path === null ? null : `${path}|${tick}|${JSON.stringify(deps)}`;
  const [state, setState] = useState<{ key: string | null; data?: T; error?: string }>({ key: null });

  useEffect(() => {
    if (key === null || path === null) return;
    let cancelled = false;
    api<T>(path)
      .then((data) => {
        if (!cancelled) setState({ key, data });
      })
      .catch((e: unknown) => {
        if (!cancelled)
          setState((s) => ({ key, data: s.data, error: e instanceof Error ? e.message : String(e) }));
      });
    return () => {
      cancelled = true;
    };
  }, [key, path]);

  return { data: state.data, error: state.error, loading: key !== null && state.key !== key, reload };
}

export type ServerEvent = { type: string; data: unknown };

export function useEvents(onEvent: (e: ServerEvent) => void): void {
  const handler = useRef(onEvent);
  useEffect(() => {
    handler.current = onEvent;
  });
  useEffect(() => {
    const source = new EventSource(eventsURL());
    const types = ["session", "workspace", "workspace.removed", "commit", "approval"];
    const listeners = types.map((type) => {
      const fn = (ev: MessageEvent<string>) => {
        try {
          const parsed = JSON.parse(ev.data) as ServerEvent;
          handler.current(parsed);
        } catch {
          handler.current({ type, data: undefined });
        }
      };
      source.addEventListener(type, fn);
      return [type, fn] as const;
    });
    return () => {
      listeners.forEach(([type, fn]) => source.removeEventListener(type, fn));
      source.close();
    };
  }, []);
}
