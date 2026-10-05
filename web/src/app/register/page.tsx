import { Suspense } from "react";
import type { Metadata } from "next";
import { AuthPage } from "@/components/AuthPage";

export const metadata: Metadata = { title: "Inscription", robots: { index: false, follow: false } };

export default function RegisterPage() {
  return <Suspense><AuthPage mode="register" /></Suspense>;
}
