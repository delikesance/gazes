import { Suspense } from "react";
import { SiteHeader, SiteHeaderFallback } from "@/components/SiteHeader";
import { LocaleDocument } from "@/components/LocaleDocument";
import { SiteFooter } from "@/components/SiteFooter";
import type { Metadata } from "next";
import { DM_Sans, Fraunces, Geist_Mono } from "next/font/google";
import "./globals.css";

const dmSans = DM_Sans({
  variable: "--font-dm-sans",
  subsets: ["latin"],
});

const fraunces = Fraunces({
  variable: "--font-fraunces",
  subsets: ["latin"],
});

const geistMono = Geist_Mono({
  variable: "--font-geist-mono",
  subsets: ["latin"],
});

export const metadata: Metadata = {
  title: "Gazes — Découvrez votre prochain anime",
  description: "Découvrez les animes du moment, explorez leurs saisons et regardez vos épisodes sur Gazes.",
};

export default function RootLayout({
  children,
}: Readonly<{
  children: React.ReactNode;
}>) {
  return (
    <html lang="fr" className="dark" suppressHydrationWarning>
      <head>
        <script dangerouslySetInnerHTML={{ __html: `try { if (localStorage.getItem("gazes-theme") === "light") { document.documentElement.classList.replace("dark", "light"); } } catch {}` }} />
        <link rel="preconnect" href="https://s4.anilist.co" crossOrigin="anonymous" />
        <link rel="dns-prefetch" href="https://s4.anilist.co" />
        <link rel="preconnect" href="https://graphql.anilist.co" />
      </head>
      <body id="top" className={`${dmSans.variable} ${fraunces.variable} ${geistMono.variable} antialiased`}>
        <LocaleDocument />
        <Suspense fallback={<SiteHeaderFallback />}><SiteHeader /></Suspense>
        <div className="site-content">{children}</div>
        <SiteFooter />
      </body>
    </html>
  );
}
