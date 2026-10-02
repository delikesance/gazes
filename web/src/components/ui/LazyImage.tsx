"use client";

import React, { useState, useEffect, useRef } from "react";
import { Film } from "lucide-react";

interface LazyImageProps {
  src?: string | null;
  alt: string;
  className?: string;
  imgClassName?: string;
  aspectRatio?: string; // e.g. "aspect-[2/3]", "aspect-video", "aspect-square"
  priority?: boolean;
}

export const LazyImage: React.FC<LazyImageProps> = ({
  src,
  alt,
  className = "",
  imgClassName = "",
  aspectRatio = "aspect-[2/3]",
  priority = false,
}) => {
  const [isInView, setIsInView] = useState(() => {
    if (priority) return true;
    if (typeof window === "undefined") return false;
    return !("IntersectionObserver" in window);
  });
  const [isLoaded, setIsLoaded] = useState(false);
  const [hasError, setHasError] = useState(false);
  const containerRef = useRef<HTMLDivElement>(null);

  useEffect(() => {
    if (priority || isInView) return;

    const el = containerRef.current;
    if (!el) return;

    if ("IntersectionObserver" in window) {
      const observer = new IntersectionObserver(
        (entries) => {
          if (entries[0].isIntersecting) {
            setIsInView(true);
            observer.disconnect();
          }
        },
        { rootMargin: "200px 0px" }
      );
      observer.observe(el);
      return () => observer.disconnect();
    }
  }, [priority, isInView]);

  const showPlaceholder = !src || hasError;

  return (
    <div
      ref={containerRef}
      className={`relative overflow-hidden bg-zinc-900 border border-zinc-800/60 ${aspectRatio} ${className}`}
    >
      {showPlaceholder ? (
        <div className="flex h-full w-full items-center justify-center bg-zinc-950 text-zinc-700">
          <Film className="h-7 w-7 opacity-30 stroke-1" />
        </div>
      ) : isInView ? (
        <>
          {!isLoaded && (
            <div className="absolute inset-0 bg-zinc-900 animate-pulse" />
          )}
          {/* eslint-disable-next-line @next/next/no-img-element */}
          <img
            src={src}
            alt={alt}
            loading={priority ? "eager" : "lazy"}
            decoding="async"
            onLoad={() => setIsLoaded(true)}
            onError={() => setHasError(true)}
            className={`h-full w-full object-cover object-center transition-opacity duration-300 ${
              isLoaded ? "opacity-100" : "opacity-0"
            } ${imgClassName}`}
          />
        </>
      ) : (
        <div className="h-full w-full bg-zinc-900/40" />
      )}
    </div>
  );
};
