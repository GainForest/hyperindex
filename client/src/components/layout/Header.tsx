'use client'

import { useState, useRef, useEffect } from 'react'
import Link from 'next/link'
import Image from 'next/image'
import { usePathname } from 'next/navigation'
import { useAuth } from '@/lib/auth'
import { ThemeToggle } from '@/components/ThemeToggle'

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
      <nav className="sticky top-0 z-50 glass-panel border-b" style={{ borderColor: 'var(--border)' }}>
        <div className="h-16 max-w-3xl mx-auto px-4 sm:px-6 flex items-center">
          {/* Logo */}
          <Link href="/" className="flex items-center gap-2.5">
            <Image
              src="/hypercerts_logo.png"
              alt="Hyperindex"
              width={22}
              height={22}
            />
            <span
              className="text-lg font-[family-name:var(--font-syne)] font-bold tracking-tight"
              style={{ color: 'var(--foreground)' }}
            >
              Hyperindex
            </span>
          </Link>

          {/* Nav Links */}
          <div className="hidden sm:flex items-center gap-1 ml-8">
            {navLinks.map(({ href, label }) => (
              <Link
                key={href}
                href={href}
                className={`px-3 py-1.5 text-sm rounded-lg transition-colors font-[family-name:var(--font-outfit)] ${
                  isActive(href) ? 'font-medium' : ''
                }`}
                style={{ color: isActive(href) ? 'var(--foreground)' : 'var(--muted-foreground)' }}
              >
                {label}
              </Link>
            ))}
          </div>

          {/* Right side */}
          <div className="flex items-center gap-2 ml-auto">
            <ThemeToggle />

            {/* User menu */}
            <div className="relative" ref={dropdownRef}>
              {isLoading ? (
                <div className="w-8 h-8 rounded-full animate-pulse" style={{ backgroundColor: 'var(--accent)' }} />
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
                      <div
                        className="w-[30px] h-[30px] rounded-full flex items-center justify-center text-sm font-[family-name:var(--font-syne)] font-semibold"
                        style={{ backgroundColor: 'var(--accent)', color: 'var(--accent-foreground)' }}
                      >
                        {(session.displayName || session.handle).charAt(0).toUpperCase()}
                      </div>
                    )
                  ) : (
                    <span
                      className="text-sm transition-colors"
                      style={{ color: 'var(--muted-foreground)' }}
                    >
                      Sign in
                    </span>
                  )}
                </button>
              )}

              {/* Dropdown Menu */}
              {showDropdown && (
                <div
                  className="absolute right-0 top-full mt-2 w-48 glass-panel rounded-xl shadow-lg py-2 z-50"
                  style={{ borderColor: 'var(--border)' }}
                >
                  {/* User info (if authenticated) */}
                  {isAuthenticated && session && (
                    <div className="px-4 py-2 mb-1" style={{ borderBottom: '1px solid var(--border)' }}>
                      <p className="text-sm font-medium truncate" style={{ color: 'var(--foreground)' }}>
                        {session.displayName || session.handle}
                      </p>
                      <p className="text-xs truncate" style={{ color: 'var(--muted-foreground)' }}>
                        @{session.handle}
                      </p>
                    </div>
                  )}

                  {/* Mobile nav (only show on small screens) */}
                  <div className="sm:hidden py-1 mb-1" style={{ borderBottom: '1px solid var(--border)' }}>
                    {navLinks.map(({ href, label }) => (
                      <Link
                        key={href}
                        href={href}
                        onClick={() => setShowDropdown(false)}
                        className="block px-4 py-2 text-sm transition-colors"
                        style={{ color: isActive(href) ? 'var(--primary)' : 'var(--foreground)' }}
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
                      className="block px-4 py-2 text-sm transition-colors"
                      style={{ color: isActive('/settings') ? 'var(--primary)' : 'var(--foreground)' }}
                    >
                      Settings
                    </Link>
                    <a
                      href="/graphiql"
                      target="_blank"
                      rel="noopener noreferrer"
                      onClick={() => setShowDropdown(false)}
                      className="flex items-center justify-between px-4 py-2 text-sm transition-colors"
                      style={{ color: 'var(--foreground)' }}
                    >
                      GraphiQL
                      <svg className="w-3 h-3" style={{ color: 'var(--muted-foreground)' }} fill="none" viewBox="0 0 24 24" strokeWidth={2} stroke="currentColor">
                        <path strokeLinecap="round" strokeLinejoin="round" d="m4.5 19.5 15-15m0 0H8.25m11.25 0v11.25" />
                      </svg>
                    </a>
                  </div>

                  {/* Auth action */}
                  <div className="mt-1 pt-1" style={{ borderTop: '1px solid var(--border)' }}>
                    {isAuthenticated ? (
                      <button
                        onClick={handleLogout}
                        className="block w-full text-left px-4 py-2 text-sm transition-colors"
                        style={{ color: 'var(--muted-foreground)' }}
                      >
                        Sign out
                      </button>
                    ) : (
                      <button
                        onClick={() => {
                          setShowDropdown(false)
                          setShowLoginModal(true)
                        }}
                        className="block w-full text-left px-4 py-2 text-sm transition-colors"
                        style={{ color: 'var(--primary)' }}
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
            className="relative w-full max-w-sm mx-4 glass-panel rounded-xl shadow-lg p-6"
            onClick={(e) => e.stopPropagation()}
          >
            <h2
              className="font-[family-name:var(--font-syne)] text-xl mb-1"
              style={{ color: 'var(--foreground)' }}
            >
              Sign in with ATProto
            </h2>
            <p className="text-sm mb-5" style={{ color: 'var(--muted-foreground)' }}>
              Enter your Bluesky handle to connect.
            </p>

            <form onSubmit={handleLogin}>
              <label
                htmlFor="auth-handle"
                className="block text-sm mb-1.5"
                style={{ color: 'var(--foreground)' }}
              >
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
                className="w-full px-3 py-2 text-sm rounded-lg
                           focus:outline-none focus:ring-2
                           disabled:opacity-50 disabled:cursor-not-allowed"
                style={{
                  backgroundColor: 'var(--background)',
                  border: '1px solid var(--border)',
                  color: 'var(--foreground)',
                }}
              />
              <p className="text-xs mt-1.5" style={{ color: 'var(--muted-foreground)' }}>
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
                  className="flex-1 px-3 py-2 text-sm rounded-lg transition-colors
                             disabled:opacity-50 disabled:cursor-not-allowed"
                  style={{
                    backgroundColor: 'var(--secondary)',
                    color: 'var(--secondary-foreground)',
                  }}
                >
                  Cancel
                </button>
                <button
                  type="submit"
                  disabled={isSubmitting || !handle.trim()}
                  className="flex-1 px-3 py-2 text-sm rounded-lg transition-colors
                             disabled:opacity-50 disabled:cursor-not-allowed"
                  style={{
                    backgroundColor: 'var(--primary)',
                    color: 'var(--primary-foreground)',
                  }}
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
