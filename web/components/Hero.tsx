import CopyButton from "./CopyButton";
import SupervisorBoard from "./SupervisorBoard";
import styles from "./Hero.module.css";

const INSTALL = "go install github.com/cabdirizaaqyare/runix/cmd/runix@latest";

export default function Hero() {
  return (
    <section className={styles.hero} id="top">
      <div className={`container ${styles.grid}`}>
        <div className={styles.copy}>
          <span className="eyebrow">process supervisor · written in go</span>
          <h1 className={styles.title}>
            Keep everything
            <br />
            <span className={styles.accent}>running.</span>
          </h1>
          <p className={styles.lede}>
            Runix runs your apps — any language, any command — and watches them.
            When something dies, it brings it back. One binary, a background
            agent, and a single file that describes your whole stack.
          </p>

          <div className={styles.actions}>
            <a className={styles.primary} href="#start">
              Get started
            </a>
            <a
              className={styles.secondary}
              href="https://github.com/cabdirizaaqyare/runix"
              target="_blank"
              rel="noopener noreferrer"
            >
              Read the source
            </a>
          </div>

          <div className={styles.install}>
            <span className={styles.prompt}>$</span>
            <code>{INSTALL}</code>
            <CopyButton text={INSTALL} />
          </div>
        </div>

        <SupervisorBoard />
      </div>
    </section>
  );
}
