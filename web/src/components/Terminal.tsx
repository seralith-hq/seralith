import { useEffect, useRef, useState } from "react";
import { Terminal as XTerm } from "@xterm/xterm";
import { FitAddon } from "@xterm/addon-fit";
import "@xterm/xterm/css/xterm.css";
import { post, wsURL } from "../lib/api";
import type { SessionInfo } from "../lib/types";
import { ActorChip, Badge } from "./ui";
import { duration } from "../lib/format";

const theme = {
  background: "#07080b",
  foreground: "#e9e6df",
  cursor: "#ff5a1f",
  selectionBackground: "#ff5a1f55",
  black: "#0e1116",
  red: "#f0435a",
  green: "#3ddc97",
  yellow: "#f5b83d",
  blue: "#7aa2ff",
  magenta: "#c792ea",
  cyan: "#5ccfe6",
  white: "#e9e6df",
};

export function SessionTerminal({
  session,
  onExit,
}: {
  session: SessionInfo;
  onExit?: (s: SessionInfo) => void;
}) {
  const host = useRef<HTMLDivElement>(null);
  const [info, setInfo] = useState(session);
  const exitRef = useRef(onExit);
  useEffect(() => {
    exitRef.current = onExit;
  });

  useEffect(() => {
    if (!host.current) return;
    const term = new XTerm({
      fontFamily: '"JetBrains Mono", ui-monospace, Menlo, monospace',
      fontSize: 13,
      cursorBlink: true,
      scrollback: 5000,
      theme,
    });
    const fit = new FitAddon();
    term.loadAddon(fit);
    term.open(host.current);
    fit.fit();

    const socket = new WebSocket(wsURL(`/sessions/${session.id}/pty`));
    socket.binaryType = "arraybuffer";
    const send = (msg: object) => socket.readyState === WebSocket.OPEN && socket.send(JSON.stringify(msg));
    socket.onopen = () => send({ type: "resize", cols: term.cols, rows: term.rows });
    socket.onmessage = (ev: MessageEvent<ArrayBuffer | string>) => {
      if (typeof ev.data === "string") {
        const msg = JSON.parse(ev.data) as { type: string; session?: SessionInfo };
        if (msg.type === "exit" && msg.session) {
          setInfo(msg.session);
          exitRef.current?.(msg.session);
          term.write(`\r\n\x1b[38;5;202m◆ seralith\x1b[0m session exited ${msg.session.exitCode ?? ""}\r\n`);
        }
        return;
      }
      term.write(new Uint8Array(ev.data));
    };
    const input = term.onData((data) => send({ type: "input", data }));
    const observer = new ResizeObserver(() => {
      fit.fit();
      send({ type: "resize", cols: term.cols, rows: term.rows });
    });
    observer.observe(host.current);
    term.focus();
    return () => {
      observer.disconnect();
      input.dispose();
      socket.close();
      term.dispose();
    };
  }, [session.id]);

  const running = info.state === "running";
  return (
    <div className="terminal-shell" style={{ height: "100%" }}>
      <div className="terminal-bar">
        <div className="row">
          <ActorChip actor={info.actor} />
          <span className="mono muted terminal-command" title={info.command.join(" ")}>
            {info.command.join(" ")}
          </span>
        </div>
        <div className="row">
          <span className="muted small mono">{duration(info.startedAt, info.endedAt)}</span>
          {!running && (
            <span className="small mono">
              <span className="adds">+{info.linesAdded}</span>{" "}
              <span className="dels">−{info.linesDeleted}</span>
            </span>
          )}
          <Badge value={info.state} pulse={running} />
          {running && (
            <button
              className="btn btn-sm"
              onClick={() => void post(`/sessions/${info.id}/signal`, { signal: "interrupt" })}
            >
              Interrupt
            </button>
          )}
        </div>
      </div>
      <div className="terminal-body" ref={host} />
    </div>
  );
}
