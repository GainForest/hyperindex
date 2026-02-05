'use client'

import { useState, useEffect } from 'react'

export function GeometricBackground() {
  const [showLogo, setShowLogo] = useState(false)

  // Show logo every 30 seconds
  useEffect(() => {
    const interval = setInterval(() => {
      setShowLogo(true)
      // Hide after animation completes (8s flow down)
      setTimeout(() => setShowLogo(false), 8000)
    }, 30000)

    // Show once on initial load after a short delay
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
      className="fixed inset-0 pointer-events-none select-none overflow-hidden"
      aria-hidden="true"
    >
      {/* Flow column 1 */}
      <div className="absolute right-[220px] top-0">
        <div className="w-1 h-20 bg-gradient-to-b from-transparent via-emerald-400/40 to-transparent rounded-full animate-[flowDown_9s_linear_infinite] [animation-fill-mode:backwards]" />
        <div className="absolute w-6 h-6 border-[1.5px] border-emerald-400/50 animate-[flowDown_12s_linear_infinite_1s] [animation-fill-mode:backwards]" />
        <svg className="absolute w-3 h-3 animate-[flowDown_8s_linear_infinite_5s] [animation-fill-mode:backwards]" viewBox="0 0 12 12">
          <polygon points="6,0 12,12 0,12" fill="rgb(52, 211, 153)" opacity="0.4" />
        </svg>
      </div>

      {/* Flow column 2 */}
      <div className="absolute right-[150px] top-0">
        <div className="w-1 h-16 bg-gradient-to-b from-transparent via-emerald-400/40 to-transparent rounded-full animate-[flowDown_7s_linear_infinite_2s] [animation-fill-mode:backwards]" />
        <svg className="absolute w-5 h-5 animate-[flowDown_10s_linear_infinite] [animation-fill-mode:backwards]" viewBox="0 0 16 16">
          <polygon points="8,0 16,16 0,16" fill="none" stroke="rgb(52, 211, 153)" strokeWidth="1.5" opacity="0.5" />
        </svg>
        <div className="absolute w-3 h-3 bg-emerald-400/30 animate-[flowDown_15s_linear_infinite_4s] [animation-fill-mode:backwards]" />
      </div>

      {/* Flow column 3 */}
      <div className="absolute right-[80px] top-0">
        <div className="w-1 h-[70px] bg-gradient-to-b from-transparent via-emerald-400/40 to-transparent rounded-full animate-[flowDown_11s_linear_infinite_4s] [animation-fill-mode:backwards]" />
        <div className="absolute w-4 h-4 border-[1.5px] border-emerald-400/50 animate-[flowDown_14s_linear_infinite_3s] [animation-fill-mode:backwards]" />
        <svg className="absolute w-4 h-4 animate-[flowDown_18s_linear_infinite_2s] [animation-fill-mode:backwards]" viewBox="0 0 12 12">
          <polygon points="6,0 12,12 0,12" fill="none" stroke="rgb(52, 211, 153)" strokeWidth="1.5" opacity="0.4" />
        </svg>
      </div>

      {/* GainForest logo - appears every 30s */}
      {showLogo && (
        <div className="absolute right-[140px] top-0 animate-[flowDownLogo_8s_ease-in-out_forwards]">
          <svg 
            className="w-10 h-10 opacity-40" 
            viewBox="0 0 200 200" 
            fill="none"
          >
            <path 
              fill="#34d399" 
              d="M108.186920,157.196320 
                C118.768028,146.561264 129.156982,136.234772 139.404739,125.769997 
                C150.157120,114.789909 152.155594,97.886940 144.131287,83.621307 
                C138.375931,89.289230 132.819656,94.910568 127.099205,100.359543 
                C118.552223,108.500908 108.333878,110.164650 98.189201,105.083656 
                C96.006401,103.990395 94.086143,102.372932 92.418434,101.248711 
                C110.258148,83.349327 127.828522,65.720184 145.423294,48.066566 
                C181.375259,75.208214 182.268341,118.425926 156.024506,144.623352 
                C137.644089,162.971252 119.186386,181.241745 99.898994,200.405502 
                C95.099167,195.322769 90.324440,190.069946 85.339081,185.025314 
                C71.875595,171.401749 58.280392,157.908356 44.814175,144.287445 
                C21.163662,120.365242 21.230278,82.262932 44.945442,58.452305 
                C62.106354,41.222309 79.317841,24.042683 96.515533,6.849345 
                C97.795906,5.569302 99.146828,4.359818 100.760529,2.838964 
                C109.526825,11.915108 118.168358,20.862080 126.476990,29.464386 
                C121.129425,35.018162 115.522278,40.841518 109.320015,47.282944 
                C106.720047,44.115860 104.101326,40.925930 101.008034,37.157913 
                C87.763069,50.551346 75.358055,63.057655 62.996449,75.606720 
                C51.793400,86.979660 48.934738,102.634224 55.802773,117.020897 
                C57.558064,120.697754 60.168159,124.150711 63.023388,127.086090 
                C74.750366,139.142258 86.720108,150.962357 98.616280,162.853561 
                C99.181381,163.418427 99.896935,163.832779 101.470276,165.013596 
                C103.633583,162.479706 105.783142,159.961914 108.186920,157.196320z"
            />
          </svg>
        </div>
      )}

      <style jsx>{`
        @keyframes flowDown {
          0% { transform: translateY(-100px); }
          100% { transform: translateY(100vh); }
        }
        @keyframes flowDownLogo {
          0% { 
            transform: translateY(-60px); 
            opacity: 0;
          }
          10% {
            opacity: 1;
          }
          90% {
            opacity: 1;
          }
          100% { 
            transform: translateY(100vh); 
            opacity: 0;
          }
        }
      `}</style>
    </div>
  )
}
