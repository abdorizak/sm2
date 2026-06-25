"use client";

import { useEffect, useRef, useState } from "react";
import styles from "./SupervisorBoard.module.css";

type ProcState = "RUNNING" | "RESTARTING" | "FAILED" | "STOPPED";

interface Proc {
  name: string;
  cmd: string;
  state: ProcState;
  cpu: number;
  mem: number; // MB
  uptime: number; // seconds
  restarts: number;
}

interface LogLine {
  t: string;
  text: string;
  tone: "ok" | "warn" | "fail" | "info";
}

const SEED: Proc[] = [
  { name: "api", cmd: "./api", state: "RUNNING", cpu: 1.2, mem: 18.4, uptime: 8124, restarts: 0 },
  { name: "worker", cmd: "python worker.py", state: "RUNNING", cpu: 0.6, mem: 31.0, uptime: 740, restarts: 2 },
  { name: "web", cmd: "npm run start", state: "RUNNING", cpu: 2.1, mem: 64.2, uptime: 8110, restarts: 0 },
  { name: "cache", cmd: "redis-server", state: "RUNNING", cpu: 0.3, mem: 9.1, uptime: 8132, restarts: 0 },
];

function clock(offset = 0): string {
  // A stable, render-safe pseudo time so SSR and client agree on first paint.
  const base = 14 * 3600 + 22 * 60 + 7 + offset;
  const h = Math.floor(base / 3600) % 24;
  const m = Math.floor((base % 3600) / 60);
  const s = base % 60;
  return [h, m, s].map((n) => String(n).padStart(2, "0")).join(":");
}

function fmtUptime(sec: number): string {
  if (sec < 60) return `${sec}s`;
  if (sec < 3600) return `${Math.floor(sec / 60)}m ${sec % 60}s`;
  return `${Math.floor(sec / 3600)}h ${Math.floor((sec % 3600) / 60)}m`;
}

export default function SupervisorBoard() {
  const [procs, setProcs] = useState<Proc[]>(SEED);
  const [log, setLog] = useState<LogLine>({
    t: clock(),
    text: "agent listening · 4 apps supervised",
    tone: "info",
  });
  const tick = useRef(0);

  useEffect(() => {
    const reduce = window.matchMedia("(prefers-reduced-motion: reduce)").matches;
    if (reduce) return;

    // 1s heartbeat: uptimes advance, CPU jitters, the clock moves.
    const beat = setInterval(() => {
      tick.current += 1;
      setProcs((prev) =>
        prev.map((p) => ({
          ...p,
          uptime: p.state === "RUNNING" ? p.uptime + 1 : p.uptime,
          cpu:
            p.state === "RUNNING"
              ? Math.max(0.1, +(p.cpu + (Math.sin(tick.current / 3 + p.name.length) * 0.4)).toFixed(1))
              : p.cpu,
        }))
      );
    }, 1000);

    // The signature: worker crashes, Runix restarts it. On a loop.
    const cycle: number[] = [];
    function schedule() {
      const set = (name: string, patch: Partial<Proc>) =>
        setProcs((prev) => prev.map((p) => (p.name === name ? { ...p, ...patch } : p)));

      cycle.push(
        window.setTimeout(() => {
          set("worker", { state: "FAILED", cpu: 0 });
          setLog({ t: clock(tick.current), text: "worker exited (status 1)", tone: "fail" });
        }, 4200),
        window.setTimeout(() => {
          setProcs((prev) =>
            prev.map((p) =>
              p.name === "worker"
                ? { ...p, state: "RESTARTING", restarts: p.restarts + 1, uptime: 0 }
                : p
            )
          );
          setLog({ t: clock(tick.current), text: "worker · restarting (policy: on-failure)", tone: "warn" });
        }, 5400),
        window.setTimeout(() => {
          set("worker", { state: "RUNNING", cpu: 0.6, uptime: 1 });
          setLog({ t: clock(tick.current), text: "worker · back up", tone: "ok" });
        }, 6800),
        window.setTimeout(schedule, 11000)
      );
    }
    const kickoff = window.setTimeout(schedule, 2600);

    return () => {
      clearInterval(beat);
      clearTimeout(kickoff);
      cycle.forEach(clearTimeout);
    };
  }, []);

  return (
    <div className={styles.window} role="img" aria-label="A live view of Runix supervising four processes; the worker crashes and Runix restarts it automatically.">
      <div className={styles.bar}>
        <span className={`${styles.dot} ${styles.r}`} />
        <span className={`${styles.dot} ${styles.y}`} />
        <span className={`${styles.dot} ${styles.g}`} />
        <span className={styles.barTitle}>runix status — live</span>
      </div>

      <div className={styles.body}>
        <div className={`${styles.row} ${styles.head}`}>
          <span>app</span>
          <span>state</span>
          <span className={styles.cpuCol}>cpu</span>
          <span className={styles.numCol}>uptime</span>
          <span className={styles.numCol}>restarts</span>
        </div>

        {procs.map((p) => (
          <div className={styles.row} key={p.name}>
            <span className={styles.name}>
              {p.name}
              <span className={styles.cmd}>{p.cmd}</span>
            </span>
            <span>
              <span
                className={`${styles.pill} ${styles[p.state.toLowerCase()]}`}
                data-state={p.state}
              >
                <span className={styles.led} />
                {p.state}
              </span>
            </span>
            <span className={styles.cpuCol}>
              <span className={styles.cpuBar}>
                <span
                  className={styles.cpuFill}
                  style={{ width: `${Math.min(p.cpu * 22, 100)}%` }}
                />
              </span>
              <span className={styles.cpuNum}>{p.cpu.toFixed(1)}%</span>
            </span>
            <span className={styles.numCol}>{fmtUptime(p.uptime)}</span>
            <span className={`${styles.numCol} ${p.restarts > 0 ? styles.warnText : ""}`}>
              {p.restarts}
            </span>
          </div>
        ))}

        <div className={styles.logline}>
          <span className={styles.logT}>{log.t}</span>
          <span className={styles[`tone_${log.tone}`]}>{log.text}</span>
        </div>
      </div>
    </div>
  );
}
