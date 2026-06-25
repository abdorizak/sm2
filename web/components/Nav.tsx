import styles from "./Nav.module.css";

export default function Nav() {
  return (
    <header className={styles.nav}>
      <div className={`container ${styles.inner}`}>
        <a href="#top" className={styles.brand}>
          <span className={styles.mark} aria-hidden="true">
            ◆
          </span>
          runix
        </a>
        <nav className={styles.links}>
          <a href="#features">features</a>
          <a href="#start">quickstart</a>
          <a href="#config">config</a>
          <a href="#architecture">architecture</a>
          <a
            className={styles.cta}
            href="https://github.com/cabdirizaaqyare/runix"
            target="_blank"
            rel="noopener noreferrer"
          >
            GitHub ↗
          </a>
        </nav>
      </div>
    </header>
  );
}
