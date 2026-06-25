import styles from "./ConfigSection.module.css";

export default function ConfigSection() {
  return (
    <section className={styles.section} id="config">
      <div className="container">
        <div className="section-head">
          <span className="eyebrow">declare it once</span>
          <h2>Your stack, in one file.</h2>
          <p>
            Describe every app in <code className="tok">runix.yaml</code>. Edit it,
            run <code className="tok">runix config reload</code>, and Runix makes
            reality match — no manual start/stop dance.
          </p>
        </div>

        <div className={styles.split}>
          <figure className={styles.window}>
            <figcaption className={styles.bar}>
              <span className={styles.dots}>
                <i /> <i /> <i />
              </span>
              <span className={styles.title}>runix.yaml</span>
            </figcaption>
            <pre className={styles.code}>
              <span className={styles.k}>agent:</span>{"\n"}
              {"  "}<span className={styles.k}>name:</span> <span className={styles.s}>production</span>{"\n"}
              {"\n"}
              <span className={styles.k}>apps:</span>{"\n"}
              {"  "}<span className={styles.k}>api:</span>{"\n"}
              {"    "}<span className={styles.k}>command:</span> <span className={styles.s}>&quot;./api&quot;</span>{"\n"}
              {"    "}<span className={styles.k}>restart:</span>{"\n"}
              {"      "}<span className={styles.k}>policy:</span> always{"\n"}
              {"      "}<span className={styles.k}>max_retries:</span> <span className={styles.n}>5</span>{"\n"}
              {"    "}<span className={styles.k}>environment:</span>{"\n"}
              {"      "}<span className={styles.k}>PORT:</span> <span className={styles.s}>&quot;8080&quot;</span>{"\n"}
              {"\n"}
              {"  "}<span className={styles.k}>worker:</span>{"\n"}
              {"    "}<span className={styles.k}>command:</span> <span className={styles.s}>&quot;python worker.py&quot;</span>{"\n"}
              {"    "}<span className={styles.k}>restart:</span>{"\n"}
              {"      "}<span className={styles.k}>policy:</span> on-failure{"\n"}
              {"\n"}
              <span className={styles.k}>notifications:</span>{"\n"}
              {"  "}<span className={styles.k}>discord:</span>{"\n"}
              {"    "}<span className={styles.k}>enabled:</span> <span className={styles.n}>true</span>{"\n"}
              {"    "}<span className={styles.k}>webhook:</span> <span className={styles.s}>&quot;https://discord.com/api/...&quot;</span>
            </pre>
          </figure>

          <figure className={styles.window}>
            <figcaption className={styles.bar}>
              <span className={styles.dots}>
                <i /> <i /> <i />
              </span>
              <span className={styles.title}>zsh</span>
            </figcaption>
            <pre className={styles.code}>
              <span className={styles.prompt}>$</span> runix config reload{"\n"}
              <span className={styles.ok}>applied runix.yaml</span>{"\n"}
              <span className={styles.dim}>NAME     STATE     PID    RESTARTS</span>{"\n"}
              api      <span className={styles.ok}>RUNNING</span>   2101   0{"\n"}
              worker   <span className={styles.ok}>RUNNING</span>   2102   0{"\n"}
              {"\n"}
              <span className={styles.prompt}>$</span> <span className={styles.dim}># edit runix.yaml, then reload again</span>{"\n"}
              <span className={styles.prompt}>$</span> runix config reload{"\n"}
              <span className={styles.info}>reconcile: started cache</span>{"\n"}
              <span className={styles.info}>reconcile: updated api</span>{"\n"}
              <span className={styles.info}>reconcile: removed worker</span>{"\n"}
              <span className={styles.ok}>applied runix.yaml</span>
            </pre>
          </figure>
        </div>
      </div>
    </section>
  );
}
