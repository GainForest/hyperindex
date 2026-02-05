'use client'

import { useState, useRef, useEffect } from 'react'
import Link from 'next/link'
import Image from 'next/image'
import { usePathname } from 'next/navigation'
import { useAuth } from '@/lib/auth'

const navLinks = [
  { href: '/', label: 'Dashboard' },
  { href: '/lexicons', label: 'Lexicons' },
  { href: '/backfill', label: 'Backfill' },
  { href: '/docs', label: 'API Docs' },
]

export function Header() {
  const pathname = usePathname()
  const { isAuthenticated, isLoading, session, login, logout } = useAuth()
  const [showDropdown, setShowDropdown] = useState(false)
  const [showLoginModal, setShowLoginModal] = useState(false)
  const [handle, setHandle] = useState('')
  const [isSubmitting, setIsSubmitting] = useState(false)
  const [error, setError] = useState('')
  const dropdownRef = useRef<HTMLDivElement>(null)

  const isActive = (href: string) => {
    if (href === '/') return pathname === '/'
    return pathname.startsWith(href)
  }

  // Close dropdown on outside click
  useEffect(() => {
    if (!showDropdown) return
    const handleClickOutside = (e: MouseEvent) => {
      if (dropdownRef.current && !dropdownRef.current.contains(e.target as Node)) {
        setShowDropdown(false)
      }
    }
    document.addEventListener('mousedown', handleClickOutside)
    return () => document.removeEventListener('mousedown', handleClickOutside)
  }, [showDropdown])

  const handleLogin = async (e: React.FormEvent) => {
    e.preventDefault()
    if (!handle.trim()) return
    setIsSubmitting(true)
    setError('')
    try {
      await login(handle.trim())
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Login failed')
      setIsSubmitting(false)
    }
  }

  const handleLogout = async () => {
    setShowDropdown(false)
    await logout()
  }

  return (
    <>
      <nav className="relative z-10 py-6">
        <div className="max-w-3xl mx-auto px-4 sm:px-6">
          <div className="flex items-center">
            {/* Logo */}
            <Link href="/" className="flex items-center gap-2.5">
              <Image
                src="/logo.png"
                alt="Hypergoat"
                width={20}
                height={20}
                className="opacity-80"
              />
              <span className="text-lg font-medium text-zinc-800 tracking-tight">
                Hypergoat
              </span>
            </Link>

            {/* Nav Links */}
            <div className="hidden sm:flex items-center gap-1 ml-8">
              {navLinks.map(({ href, label }) => (
                <Link
                  key={href}
                  href={href}
                  className={`px-3 py-1.5 text-sm rounded-lg transition-colors ${
                    isActive(href)
                      ? 'text-zinc-900 font-medium'
                      : 'text-zinc-400 hover:text-zinc-600'
                  }`}
                >
                  {label}
                </Link>
              ))}
            </div>

            {/* Right side - User menu */}
            <div className="relative ml-auto" ref={dropdownRef}>
              {isLoading ? (
                <div className="w-8 h-8 rounded-full bg-zinc-100 animate-pulse" />
              ) : (
                <button
                  onClick={() => setShowDropdown(!showDropdown)}
                  className="flex items-center cursor-pointer"
                >
                  {isAuthenticated && session ? (
                    session.avatar ? (
                      <Image
                        src={session.avatar}
                        alt=""
                        width={30}
                        height={30}
                        className="rounded-full"
                      />
                    ) : (
                      <div className="w-[30px] h-[30px] rounded-full bg-emerald-100 flex items-center justify-center text-sm font-medium text-emerald-700">
                        {(session.displayName || session.handle).charAt(0).toUpperCase()}
                      </div>
                    )
                  ) : (
                    <span className="text-sm text-zinc-400 hover:text-zinc-600 transition-colors">
                      Sign in
                    </span>
                  )}
                </button>
              )}

              {/* Dropdown Menu */}
              {showDropdown && (
                <div className="absolute right-0 top-full mt-2 w-48 bg-white rounded-xl shadow-lg border border-zinc-200/60 py-2 z-50">
                  {/* User info (if authenticated) */}
                  {isAuthenticated && session && (
                    <div className="px-4 py-2 border-b border-zinc-100 mb-1">
                      <p className="text-sm font-medium text-zinc-800 truncate">
                        {session.displayName || session.handle}
                      </p>
                      <p className="text-xs text-zinc-400 truncate">
                        @{session.handle}
                      </p>
                    </div>
                  )}

                  {/* Mobile nav (only show on small screens) */}
                  <div className="sm:hidden py-1 border-b border-zinc-100 mb-1">
                    {navLinks.map(({ href, label }) => (
                      <Link
                        key={href}
                        href={href}
                        onClick={() => setShowDropdown(false)}
                        className={`block px-4 py-2 text-sm transition-colors ${
                          isActive(href)
                            ? 'text-emerald-600 bg-emerald-50/50'
                            : 'text-zinc-600 hover:bg-zinc-50'
                        }`}
                      >
                        {label}
                      </Link>
                    ))}
                  </div>

                  {/* Extra links */}
                  <div className="py-1">
                    <Link
                      href="/settings"
                      onClick={() => setShowDropdown(false)}
                      className={`block px-4 py-2 text-sm transition-colors ${
                        isActive('/settings')
                          ? 'text-emerald-600 bg-emerald-50/50'
                          : 'text-zinc-600 hover:bg-zinc-50'
                      }`}
                    >
                      Settings
                    </Link>
                    <a
                      href="/graphiql"
                      target="_blank"
                      rel="noopener noreferrer"
                      onClick={() => setShowDropdown(false)}
                      className="flex items-center justify-between px-4 py-2 text-sm text-zinc-600 hover:bg-zinc-50 transition-colors"
                    >
                      GraphiQL
                      <svg className="w-3 h-3 text-zinc-300" fill="none" viewBox="0 0 24 24" strokeWidth={2} stroke="currentColor">
                        <path strokeLinecap="round" strokeLinejoin="round" d="m4.5 19.5 15-15m0 0H8.25m11.25 0v11.25" />
                      </svg>
                    </a>
                  </div>

                  {/* Auth action */}
                  <div className="border-t border-zinc-100 mt-1 pt-1">
                    {isAuthenticated ? (
                      <button
                        onClick={handleLogout}
                        className="block w-full text-left px-4 py-2 text-sm text-zinc-500 hover:text-zinc-700 hover:bg-zinc-50 transition-colors"
                      >
                        Sign out
                      </button>
                    ) : (
                      <button
                        onClick={() => {
                          setShowDropdown(false)
                          setShowLoginModal(true)
                        }}
                        className="block w-full text-left px-4 py-2 text-sm text-emerald-600 hover:bg-emerald-50/50 transition-colors"
                      >
                        Sign in
                      </button>
                    )}
                  </div>
                </div>
              )}
            </div>
          </div>
        </div>
      </nav>

      {/* Login Modal */}
      {showLoginModal && (
        <div
          className="fixed inset-0 z-50 flex items-start justify-center pt-[20vh]"
          onClick={() => setShowLoginModal(false)}
        >
          <div className="absolute inset-0 bg-black/20 backdrop-blur-sm" />
          <div
            className="relative w-full max-w-sm mx-4 bg-white rounded-xl shadow-lg border border-zinc-200/60 p-6"
            onClick={(e) => e.stopPropagation()}
          >
            <h2 className="font-[family-name:var(--font-garamond)] text-xl text-zinc-900 mb-1">
              Sign in with ATProto
            </h2>
            <p className="text-sm text-zinc-400 mb-5">
              Enter your Bluesky handle to connect.
            </p>

            <form onSubmit={handleLogin}>
              <label htmlFor="auth-handle" className="block text-sm text-zinc-600 mb-1.5">
                Handle
              </label>
              <input
                id="auth-handle"
                type="text"
                value={handle}
                onChange={(e) => setHandle(e.target.value)}
                placeholder="alice.bsky.social"
                disabled={isSubmitting}
                autoFocus
                className="w-full px-3 py-2 text-sm bg-white border border-zinc-200/60 rounded-lg
                           placeholder:text-zinc-300
                           focus:outline-none focus:ring-2 focus:ring-emerald-500/30 focus:border-emerald-400
                           disabled:opacity-50 disabled:cursor-not-allowed"
              />
              <p className="text-xs text-zinc-300 mt-1.5">
                Just a username? We&apos;ll add .bsky.social for you.
              </p>

              {error && (
                <p className="text-sm text-red-500 mt-2">{error}</p>
              )}

              <div className="flex gap-2 mt-5">
                <button
                  type="button"
                  onClick={() => setShowLoginModal(false)}
                  disabled={isSubmitting}
                  className="flex-1 px-3 py-2 text-sm text-zinc-600 bg-zinc-50 rounded-lg
                             hover:bg-zinc-100 transition-colors
                             disabled:opacity-50 disabled:cursor-not-allowed"
                >
                  Cancel
                </button>
                <button
                  type="submit"
                  disabled={isSubmitting || !handle.trim()}
                  className="flex-1 px-3 py-2 text-sm text-white bg-emerald-600 rounded-lg
                             hover:bg-emerald-700 transition-colors
                             disabled:opacity-50 disabled:cursor-not-allowed"
                >
                  {isSubmitting ? 'Connecting...' : 'Connect'}
                </button>
              </div>
            </form>
          </div>
        </div>
      )}
    </>
  )
}
