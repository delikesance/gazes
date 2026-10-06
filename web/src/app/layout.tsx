import { Suspense } from "react";
import { SiteHeader, SiteHeaderFallback } from "@/components/SiteHeader";
import { LocaleDocument } from "@/components/LocaleDocument";
import { SiteFooter } from "@/components/SiteFooter";
import { ServiceWorker } from "@/components/ServiceWorker";
import { MobileNav } from "@/components/MobileNav";
import { SiteChrome } from "@/components/SiteChrome";
import { ChangelogBanner } from "@/components/ChangelogBanner";
import { AuthProvider } from "@/components/AuthProvider";
import type { Metadata } from "next";
import Script from "next/script";
import { SITE_URL, SITE_NAME } from "@/lib/site";
import { DM_Sans, Fraunces, Geist_Mono } from "next/font/google";
import "./globals.css";
import "./mobile.css";

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

const DESCRIPTION = "Découvrez les animes du moment, explorez leurs saisons et regardez vos épisodes sur Gazes.";

export const metadata: Metadata = {
  metadataBase: new URL(SITE_URL),
  title: { default: "Gazes — Découvrez votre prochain anime", template: `%s · ${SITE_NAME}` },
  description: DESCRIPTION,
  verification: { google: "s6pY-8MjEo77FWK_KuhEdtTAmWgsHbIeWlkzYSMIPac" },
  openGraph: { type: "website", siteName: SITE_NAME, locale: "fr_FR", title: "Gazes — Découvrez votre prochain anime", description: DESCRIPTION },
  twitter: { card: "summary_large_image", title: "Gazes — Découvrez votre prochain anime", description: DESCRIPTION },
};

export default function RootLayout({
  children,
}: Readonly<{
  children: React.ReactNode;
}>) {
  return (
    <html lang="fr" className="dark" suppressHydrationWarning>
      <head>
        <Script id="theme-init" strategy="beforeInteractive" dangerouslySetInnerHTML={{ __html: `try { if (localStorage.getItem("gazes-theme") === "light") { document.documentElement.classList.replace("dark", "light"); } } catch {}` }} />
        <link rel="preconnect" href="https://s4.anilist.co" crossOrigin="anonymous" />
        <link rel="dns-prefetch" href="https://s4.anilist.co" />
        <link rel="preconnect" href="https://graphql.anilist.co" />
      </head>
      <body id="top" className={`${dmSans.variable} ${fraunces.variable} ${geistMono.variable} antialiased`}>
        <LocaleDocument />
        <ServiceWorker />
        <AuthProvider>
          <SiteChrome><Suspense fallback={<SiteHeaderFallback />}><SiteHeader /></Suspense></SiteChrome>
          <div className="site-content">{children}</div>
        </AuthProvider>
        <SiteChrome><SiteFooter /><Suspense fallback={null}><MobileNav /></Suspense><ChangelogBanner /></SiteChrome>
      </body>
    </html>
  );
}
