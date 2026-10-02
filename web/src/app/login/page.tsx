import { Suspense } from "react";
import type { Metadata } from "next";
import { AuthPage } from "@/components/AuthPage";

export const metadata: Metadata = { title: "Connexion — Gazes" };

export default function LoginPage() {
  return <Suspense><AuthPage mode="login" /></Suspense>;
}
