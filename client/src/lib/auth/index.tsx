"use client";

import {
  useState,
  useEffect,
  useCallback,
  createContext,
  useContext,
} from "react";

export interface AuthSession {
  did: string;
  handle: string;
  displayName?: string;
  avatar?: string;
}

export type AuthStatus = "idle" | "authorizing" | "authenticated" | "error";

interface AuthState {
  status: AuthStatus;
  session: AuthSession | null;
  error: Error | null;
  isLoading: boolean;
}

const AuthContext = createContext<{
  state: AuthState;
  login: (handle: string) => Promise<void>;
  logout: () => Promise<void>;
} | null>(null);

/**
 * Auth Provider - wraps the app to provide authentication state.
 * Checks session status on mount via /api/status.
 */
export function AuthProvider({ children }: { children: React.ReactNode }) {
  const [state, setState] = useState<AuthState>({
    status: "idle",
    session: null,
    error: null,
    isLoading: true,
  });

  // Check session status on mount
  useEffect(() => {
    let cancelled = false;

    const checkStatus = async () => {
      try {
        const response = await fetch("/api/status");
        if (response.ok) {
          const data = await response.json();
          if (data.did && !cancelled) {
            setState({
              status: "authenticated",
              session: {
                did: data.did,
                handle: data.handle || data.did,
                displayName: data.displayName,
                avatar: data.avatar,
              },
              error: null,
              isLoading: false,
            });
            return;
          }
        }
      } catch (error) {
        console.error("Failed to check auth status:", error);
      }
      if (!cancelled) {
        setState((prev) => ({ ...prev, isLoading: false }));
      }
    };

    checkStatus();
    return () => {
      cancelled = true;
    };
  }, []);

  // Login - initiates OAuth flow, redirects to PDS authorization
  const login = useCallback(async (handle: string) => {
    setState((prev) => ({
      ...prev,
      status: "authorizing",
      isLoading: true,
      error: null,
    }));

    try {
      const normalizedHandle = handle.includes(".")
        ? handle
        : `${handle}.bsky.social`;

      const response = await fetch("/api/login", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({
          handle: normalizedHandle,
          returnTo: window.location.pathname + window.location.search,
        }),
      });

      if (!response.ok) {
        const data = await response.json();
        throw new Error(data.error || "Login failed");
      }

      const data = await response.json();
      window.location.href = data.redirectUrl;
    } catch (err) {
      const error = err instanceof Error ? err : new Error("Login failed");
      setState({ status: "error", session: null, error, isLoading: false });
      throw error;
    }
  }, []);

  // Logout - clears server session
  const logout = useCallback(async () => {
    try {
      await fetch("/api/logout", { method: "POST" });
    } catch (error) {
      console.error("Logout request failed:", error);
    }
    setState({ status: "idle", session: null, error: null, isLoading: false });
  }, []);

  return (
    <AuthContext value={{ state, login, logout }}>{children}</AuthContext>
  );
}

/**
 * Hook to access auth state and actions.
 * Must be used within an AuthProvider.
 */
export function useAuth() {
  const context = useContext(AuthContext);

  if (!context) {
    throw new Error("useAuth must be used within an AuthProvider");
  }

  const { state, login, logout } = context;

  return {
    status: state.status,
    session: state.session,
    error: state.error,
    isLoading: state.isLoading,
    isAuthenticated: state.status === "authenticated",
    login,
    logout,
  };
}
