'use client'

export function GeometricBackground() {
  return (
    <div 
      className="fixed inset-0 pointer-events-none select-none overflow-hidden"
      aria-hidden="true"
    >
      <svg 
        className="absolute top-0 right-0 w-[400px] h-full"
        viewBox="0 0 300 800"
        preserveAspectRatio="xMaxYMid slice"
      >
        <defs>
          <linearGradient id="flowDown" x1="0%" y1="0%" x2="0%" y2="100%">
            <stop offset="0%" stopColor="rgb(52, 211, 153)" stopOpacity="0" />
            <stop offset="50%" stopColor="rgb(52, 211, 153)" stopOpacity="0.4" />
            <stop offset="100%" stopColor="rgb(52, 211, 153)" stopOpacity="0" />
          </linearGradient>
        </defs>

        {/* Flow column 1 */}
        <g>
          <rect x="78" width="4" height="80" fill="url(#flowDown)" rx="2">
            <animate attributeName="y" from="-80" to="800" dur="9s" repeatCount="indefinite" />
          </rect>
          <rect x="68" width="24" height="24" fill="none" stroke="rgb(52, 211, 153)" strokeWidth="1.5" opacity="0.5">
            <animate attributeName="y" from="-24" to="800" dur="12s" repeatCount="indefinite" begin="1s" />
          </rect>
          <polygon points="80,-5 85,5 75,5" fill="none" stroke="rgb(52, 211, 153)" strokeWidth="1" opacity="0.4">
            <animate attributeName="transform" values="translate(0,0);translate(0,816)" dur="8s" repeatCount="indefinite" begin="5s" />
          </polygon>
        </g>

        {/* Flow column 2 */}
        <g>
          <rect x="148" width="4" height="60" fill="url(#flowDown)" rx="2">
            <animate attributeName="y" from="-60" to="800" dur="7s" repeatCount="indefinite" begin="2s" />
          </rect>
          <polygon points="150,-12 162,12 138,12" fill="none" stroke="rgb(52, 211, 153)" strokeWidth="1.5" opacity="0.5">
            <animate attributeName="transform" values="translate(0,0);translate(0,816)" dur="10s" repeatCount="indefinite" />
          </polygon>
          <rect x="145" width="10" height="10" fill="none" stroke="rgb(52, 211, 153)" strokeWidth="1" opacity="0.4">
            <animate attributeName="y" from="-10" to="800" dur="15s" repeatCount="indefinite" begin="4s" />
          </rect>
        </g>

        {/* Flow column 3 */}
        <g>
          <rect x="218" width="4" height="70" fill="url(#flowDown)" rx="2">
            <animate attributeName="y" from="-70" to="800" dur="11s" repeatCount="indefinite" begin="4s" />
          </rect>
          <rect x="212" width="16" height="16" fill="none" stroke="rgb(52, 211, 153)" strokeWidth="1.5" opacity="0.5">
            <animate attributeName="y" from="-16" to="800" dur="14s" repeatCount="indefinite" begin="3s" />
          </rect>
          <polygon points="220,-7 227,7 213,7" fill="none" stroke="rgb(52, 211, 153)" strokeWidth="1" opacity="0.35">
            <animate attributeName="transform" values="translate(0,0);translate(0,816)" dur="18s" repeatCount="indefinite" begin="7s" />
          </polygon>
        </g>
      </svg>
    </div>
  )
}
