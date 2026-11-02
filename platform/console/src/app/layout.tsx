import type { Metadata } from "next";
import { Geist, JetBrains_Mono } from "next/font/google";
import "./globals.css";
import { ErrorBoundary } from "@/components/ErrorBoundary";

const geistSans = Geist({
  variable: "--font-geist-sans",
  subsets: ["latin"],
});

const jetbrains = JetBrains_Mono({
  variable: "--font-jetbrains",
  subsets: ["latin"],
  weight: ["400", "500", "600"],
});

export const metadata: Metadata = {
  title: "Kubona — Sovereign Quantum-AI Science Discovery Platform",
  description: "Autonomous quantum research observatory with post-QIR universal circuits, 4D flight recorder, and multi-agent tribunal governance.",
  icons: {
    icon: [
      { url: "/favicon.ico" },
      { url: "/favicon-32x32.png", sizes: "32x32", type: "image/png" },
      { url: "/favicon-16x16.png", sizes: "16x16", type: "image/png" },
      { url: "/assets/favicon_io/favicon.ico" },
      { url: "/assets/favicon_io/favicon-32x32.png", sizes: "32x32", type: "image/png" },
      { url: "/assets/favicon_io/favicon-16x16.png", sizes: "16x16", type: "image/png" },
    ],
    apple: [
      { url: "/apple-touch-icon.png", sizes: "180x180", type: "image/png" },
      { url: "/assets/favicon_io/apple-touch-icon.png", sizes: "180x180", type: "image/png" },
    ],
  },
  manifest: "/site.webmanifest",
};

export default function RootLayout({
  children,
}: Readonly<{
  children: React.ReactNode;
}>) {
  return (
    <html
      lang="en"
      className={`${geistSans.variable} ${jetbrains.variable} dark h-full antialiased`}
    >
      <body className="min-h-full flex flex-col bg-[#07080B] text-slate-100 font-sovereign selection:bg-cyan-500/30 selection:text-cyan-200">
        {/* Wave-2 a11y: page.tsx owns the single skip link + <main>
            landmarks; this wrapper stays a plain landmark container. */}
        <h1 className="sr-only">Kubona Sovereign Console</h1>
        <div id="main-content" tabIndex={-1}><ErrorBoundary>{children}</ErrorBoundary></div>
      </body>
    </html>
  );
}
