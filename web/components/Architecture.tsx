import styles from "./Architecture.module.css";

const NODES = [
  { key: "process", label: "Process", sub: "spawn · signal · supervise" },
  { key: "config", label: "Config", sub: "parse · validate · reconcile" },
  { key: "events", label: "Events", sub: "emit · notify" },
  { key: "monitor", label: "Monitor", sub: "cpu · mem · uptime" },
];

export default function Architecture() {
  return (
    <section className={styles.section} id="architecture">
      <div className="container">
        <div className="section-head">
          <span className="eyebrow">how it fits together</span>
          <h2>One binary. Two halves.</h2>
          <p>
            The CLI is what you type. The agent is what keeps running. They talk
            over a local Unix socket, so there&apos;s no port to expose and nothing
            to secure on day one.
          </p>
        </div>

        <div className={styles.diagram}>
          <div className={`${styles.node} ${styles.cli}`}>
            <span className={styles.kind}>CLI</span>
            <code>runix start · stop · status · logs · config</code>
          </div>

          <div className={styles.link}>
            <span className={styles.linkLabel}>JSON over Unix socket</span>
          </div>

          <div className={`${styles.node} ${styles.agent}`}>
            <span className={styles.kind}>AGENT</span>
            <code>background daemon</code>
          </div>

          <div className={styles.fan}>
            {NODES.map((n) => (
              <div className={styles.sub} key={n.key}>
                <span className={styles.subLabel}>{n.label}</span>
                <span className={styles.subSub}>{n.sub}</span>
              </div>
            ))}
          </div>

          <div className={styles.apps}>
            your apps · go · node · python · rust · shell · any executable
          </div>
        </div>
      </div>
    </section>
  );
}
