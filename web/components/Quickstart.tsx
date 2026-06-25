import CopyButton from "./CopyButton";
import styles from "./Quickstart.module.css";

type Tone = "cmd" | "ok" | "dim" | "row";
interface Line {
  text: string;
  prompt?: boolean;
  tone: Tone;
}
interface Step {
  n: string;
  title: string;
  copy: string;
  lines: Line[];
}

const STEPS: Step[] = [
  {
    n: "01",
    title: "Install",
    copy: "go install github.com/cabdirizaaqyare/runix/cmd/runix@latest",
    lines: [
      { text: "go install github.com/cabdirizaaqyare/runix/cmd/runix@latest", prompt: true, tone: "cmd" },
    ],
  },
  {
    n: "02",
    title: "Start an app",
    copy: 'runix start api --cmd "./api" --restart always',
    lines: [
      { text: 'runix start api --cmd "./api" --restart always', prompt: true, tone: "cmd" },
      { text: '✅ started "api"', tone: "ok" },
    ],
  },
  {
    n: "03",
    title: "Watch it",
    copy: "runix status",
    lines: [
      { text: "runix status", prompt: true, tone: "cmd" },
      { text: "NAME   STATE     PID    CPU    MEM      UPTIME", tone: "dim" },
      { text: "api    RUNNING   1234   1.2%   18.4MB   12s", tone: "row" },
    ],
  },
];

export default function Quickstart() {
  return (
    <section className={styles.section} id="start">
      <div className="container">
        <div className="section-head">
          <span className="eyebrow">quickstart</span>
          <h2>Up and supervising in three commands.</h2>
        </div>

        <ol className={styles.steps}>
          {STEPS.map((s) => (
            <li className={styles.step} key={s.n}>
              <div className={styles.num}>{s.n}</div>
              <div className={styles.content}>
                <h3>{s.title}</h3>
                <div className={styles.block}>
                  <CopyButton text={s.copy} />
                  <pre>
                    {s.lines.map((l, i) => (
                      <div key={i} className={styles[l.tone]}>
                        {l.prompt && <span className={styles.prompt}>$ </span>}
                        {l.text}
                      </div>
                    ))}
                  </pre>
                </div>
              </div>
            </li>
          ))}
        </ol>
      </div>
    </section>
  );
}
