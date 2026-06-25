import CopyButton from "./CopyButton";
import styles from "./CTA.module.css";

const INSTALL = "go install github.com/cabdirizaaqyare/runix/cmd/runix@latest";

export default function CTA() {
  return (
    <section className={styles.section}>
      <div className="container">
        <div className={styles.box}>
          <h2 className={styles.title}>
            Stop babysitting processes.
          </h2>
          <p className={styles.sub}>
            Install Runix, point it at your apps, and let the agent handle the
            3am restarts.
          </p>

          <div className={styles.install}>
            <span className={styles.prompt}>$</span>
            <code>{INSTALL}</code>
            <CopyButton text={INSTALL} />
          </div>

          <a
            className={styles.button}
            href="https://github.com/cabdirizaaqyare/runix"
            target="_blank"
            rel="noopener noreferrer"
          >
            Get started on GitHub ↗
          </a>
        </div>
      </div>
    </section>
  );
}
