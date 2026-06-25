import type { Metadata } from "next";
import "./globals.css";

export const metadata: Metadata = {
  title: "Runix — keep everything running",
  description:
    "A universal process supervisor written in Go. Run any app in any language with auto-restart, live monitoring, declarative config, and notifications.",
  keywords: ["process manager", "pm2 alternative", "systemd", "go", "supervisor", "devops"],
  openGraph: {
    title: "Runix — keep everything running",
    description:
      "A universal process supervisor written in Go. Auto-restart, live monitoring, declarative config, notifications.",
    type: "website",
  },
};

export default function RootLayout({
  children,
}: {
  children: React.ReactNode;
}) {
  return (
    <html lang="en">
      <head>
        <link rel="preconnect" href="https://fonts.googleapis.com" />
        <link
          rel="preconnect"
          href="https://fonts.gstatic.com"
          crossOrigin=""
        />
        <link
          href="https://fonts.googleapis.com/css2?family=IBM+Plex+Mono:wght@400;500;600;700&family=IBM+Plex+Sans:wght@400;500;600&family=Space+Grotesk:wght@500;600;700&display=swap"
          rel="stylesheet"
        />
      </head>
      <body>
        <div className="crt" aria-hidden="true" />
        {children}
      </body>
    </html>
  );
}
