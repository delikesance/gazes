import { Suspense } from "react";
import type { Metadata } from "next";
import { AuthPage } from "@/components/AuthPage";

export const metadata: Metadata = { title: "Inscription — Gazes" };

export default function RegisterPage() {
  return <Suspense><AuthPage mode="register" /></Suspense>;
}
