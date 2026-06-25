"use client";

import { useState } from "react";
import styles from "./CopyButton.module.css";

export default function CopyButton({ text }: { text: string }) {
  const [copied, setCopied] = useState(false);

  async function copy() {
    try {
      await navigator.clipboard.writeText(text);
      setCopied(true);
      setTimeout(() => setCopied(false), 1400);
    } catch {
      /* clipboard unavailable — leave the label unchanged */
    }
  }

  return (
    <button
      type="button"
      className={`${styles.btn} ${copied ? styles.copied : ""}`}
      onClick={copy}
      aria-label={copied ? "Copied" : "Copy command"}
    >
      {copied ? "copied ✓" : "copy"}
    </button>
  );
}
