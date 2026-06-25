import styles from "./Footer.module.css";

export default function Footer() {
  return (
    <footer className={styles.footer}>
      <div className={`container ${styles.inner}`}>
        <div className={styles.brand}>
          <span className={styles.mark} aria-hidden="true">
            ◆
          </span>
          runix
        </div>
        <p className={styles.tag}>
          A universal application operations agent, written in Go.
        </p>
        <p className={styles.meta}>
          MIT licensed · built with Go, Cobra &amp; zerolog
        </p>
      </div>
    </footer>
  );
}
