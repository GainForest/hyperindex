"use client"

import { useState, useEffect } from "react"

export function GeometricBackground() {
  const [showLogo, setShowLogo] = useState(false)

  useEffect(() => {
    const interval = setInterval(() => {
      setShowLogo(true)
      setTimeout(() => setShowLogo(false), 8000)
    }, 30000)

    const initialTimeout = setTimeout(() => {
      setShowLogo(true)
      setTimeout(() => setShowLogo(false), 8000)
    }, 2000)

    return () => {
      clearInterval(interval)
      clearTimeout(initialTimeout)
    }
  }, [])

  return (
    <div
      className="fixed inset-0 pointer-events-none select-none overflow-hidden z-0"
      aria-hidden="true"
    >
      {/* Subtle flowing lines using theme-aware colors */}
      <div className="absolute right-[220px] top-0">
        <div className="w-0.5 h-20 rounded-full animate-[flowDown_9s_linear_infinite] [animation-fill-mode:backwards]"
             style={{ background: "linear-gradient(to bottom, transparent, var(--border), transparent)" }} />
      </div>
      <div className="absolute right-[150px] top-0">
        <div className="w-0.5 h-16 rounded-full animate-[flowDown_7s_linear_infinite_2s] [animation-fill-mode:backwards]"
             style={{ background: "linear-gradient(to bottom, transparent, var(--border), transparent)" }} />
      </div>
      <div className="absolute right-[80px] top-0">
        <div className="w-0.5 h-[70px] rounded-full animate-[flowDown_11s_linear_infinite_4s] [animation-fill-mode:backwards]"
             style={{ background: "linear-gradient(to bottom, transparent, var(--border), transparent)" }} />
      </div>

      {/* Hypercerts logo flow - appears every 30s */}
      {showLogo && (
        <div className="absolute right-[140px] top-0 animate-[flowDownLogo_8s_ease-in-out_forwards] opacity-20">
          {/* eslint-disable-next-line @next/next/no-img-element */}
          <img src="/hypercerts_logo.png" alt="" width={40} height={40} className="opacity-40" />
        </div>
      )}

      <style jsx>{`
        @keyframes flowDown {
          0% { transform: translateY(-100px); }
          100% { transform: translateY(100vh); }
        }
        @keyframes flowDownLogo {
          0% { transform: translateY(-60px); opacity: 0; }
          10% { opacity: 1; }
          90% { opacity: 1; }
          100% { transform: translateY(100vh); opacity: 0; }
        }
      `}</style>
    </div>
  )
}
