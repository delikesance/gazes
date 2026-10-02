"use client";
import { useEffect, useState, type ReactNode } from "react";
import { createPortal } from "react-dom";

export function HeaderBreadcrumb({children}: {children: ReactNode}) {
 const [target,setTarget]=useState<HTMLElement|null>(null);
 useEffect(()=>{
  const connect=()=>{const slot=document.getElementById("header-breadcrumb");if(slot?.dataset.ready)setTarget(slot);};
  connect();window.addEventListener("gazes-header-ready",connect);
  return()=>window.removeEventListener("gazes-header-ready",connect);
 },[]);
 return target?createPortal(children,target):null;
}
