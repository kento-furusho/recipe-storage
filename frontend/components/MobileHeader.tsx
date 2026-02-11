"use client";

import Link from "next/link";
import { ReactNode } from "react";

interface MobileHeaderProps {
  title: string;
  backHref?: string;
  rightAction?: ReactNode;
}

export default function MobileHeader({ title, backHref, rightAction }: MobileHeaderProps) {
  return (
    <header className="sticky top-0 z-40 bg-white/80 backdrop-blur-sm border-b border-stone-200">
      <div className="flex items-center justify-between h-12 px-3 sm:px-4 lg:px-6">
        <div className="flex items-center gap-2 min-w-0 flex-1">
          {backHref && (
            <Link
              href={backHref}
              className="flex items-center justify-center min-h-[44px] min-w-[44px] -ml-2 text-stone-600"
              aria-label="戻る"
            >
              <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round" className="w-5 h-5">
                <polyline points="15 18 9 12 15 6" />
              </svg>
            </Link>
          )}
          <h1 className="text-lg font-bold sm:text-xl lg:text-2xl text-stone-800 truncate">
            {title}
          </h1>
        </div>
        {rightAction && (
          <div className="flex items-center gap-2 ml-2 shrink-0">
            {rightAction}
          </div>
        )}
      </div>
    </header>
  );
}
