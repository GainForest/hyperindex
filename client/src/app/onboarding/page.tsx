"use client";

import { useState } from "react";
import { useMutation, useQuery } from "@tanstack/react-query";
import { useRouter } from "next/navigation";
import { graphqlClient } from "@/lib/graphql/client";
import { GET_SETTINGS } from "@/lib/graphql/queries";
import { UPDATE_SETTINGS, UPLOAD_LEXICONS } from "@/lib/graphql/mutations";
import { useAdminSession } from "@/lib/auth";
import { Button } from "@/components/ui/Button";
import { Input } from "@/components/ui/Input";
import { Alert } from "@/components/ui/Alert";
import type { SettingsResponse } from "@/types";

interface OnboardingState {
  domainAuthority: string;
  lexiconsUploaded: boolean;
}

const STEPS = [
  { id: "welcome", title: "Welcome" },
  { id: "domain", title: "Domain" },
  { id: "admin", title: "Admin" },
  { id: "lexicons", title: "Lexicons" },
  { id: "complete", title: "Complete" },
];

export default function OnboardingPage() {
  const router = useRouter();
  const { isAdmin, isLoading: isAdminLoading } = useAdminSession();
  const [currentStep, setCurrentStep] = useState(0);
  const [error, setError] = useState<string | null>(null);
  const [state, setState] = useState<OnboardingState>({
    domainAuthority: "",
    lexiconsUploaded: false,
  });

  // Update settings mutation
  const updateSettingsMutation = useMutation({
    mutationFn: (vars: { domainAuthority?: string }) =>
      graphqlClient.request(UPDATE_SETTINGS, vars),
    onError: (err: Error) => setError(err.message),
  });

	// Upload lexicons mutation
	const uploadLexiconsMutation = useMutation({
    mutationFn: (zipBase64: string) =>
      graphqlClient.request(UPLOAD_LEXICONS, { zipBase64 }),
    onSuccess: () => {
      setState((s) => ({ ...s, lexiconsUploaded: true }));
    },
    onError: (err: Error) => setError(err.message),
  });

  const { data: settingsData, isLoading: isSettingsLoading } = useQuery({
    queryKey: ["settings"],
    queryFn: () => graphqlClient.request<SettingsResponse>(GET_SETTINGS),
    enabled: isAdmin,
  });

  const adminDids = settingsData?.settings.adminDids ?? [];

  const handleNext = async () => {
    setError(null);

    if (currentStep === 1) {
      if (!state.domainAuthority.trim()) {
        setError("Please enter a domain authority");
        return;
      }
      try {
        await updateSettingsMutation.mutateAsync({
          domainAuthority: state.domainAuthority.trim(),
        });
      } catch {
        return;
      }
	}

    setCurrentStep((s) => Math.min(s + 1, STEPS.length - 1));
  };

  const handleBack = () => {
    setError(null);
    setCurrentStep((s) => Math.max(s - 1, 0));
  };

  const handleFileUpload = async (e: React.ChangeEvent<HTMLInputElement>) => {
    const file = e.target.files?.[0];
    if (!file) return;

    if (!file.name.endsWith(".zip")) {
      setError("Please upload a .zip file");
      return;
    }

    const reader = new FileReader();
    reader.onload = () => {
      const base64 = (reader.result as string).split(",")[1];
      uploadLexiconsMutation.mutate(base64);
    };
    reader.onerror = () => setError("Failed to read file");
    reader.readAsDataURL(file);
    e.target.value = "";
  };

  const handleComplete = () => {
    router.push("/");
  };

  const isLoading =
    updateSettingsMutation.isPending ||
    uploadLexiconsMutation.isPending;

  return (
    <div className="min-h-screen flex items-center justify-center p-6" style={{ backgroundColor: "var(--card)" }}>
      <div className="w-full max-w-lg">
        {/* Progress Steps */}
        <div className="flex items-center justify-center mb-10">
          {STEPS.map((step, index) => {
            const isActive = index === currentStep;
            const isComplete = index < currentStep;

            return (
              <div key={step.id} className="flex items-center">
                <div
                  className="flex items-center justify-center w-8 h-8 rounded-full text-sm font-medium transition-colors duration-200"
                  style={
                    isActive || isComplete
                      ? { backgroundColor: "var(--primary)", color: "var(--primary-foreground)" }
                      : { backgroundColor: "var(--muted)", color: "var(--muted-foreground)" }
                  }
                >
                  {isComplete ? (
                    <svg className="w-4 h-4" fill="none" viewBox="0 0 24 24" strokeWidth={2} stroke="currentColor">
                      <path strokeLinecap="round" strokeLinejoin="round" d="m4.5 12.75 6 6 9-13.5" />
                    </svg>
                  ) : (
                    index + 1
                  )}
                </div>
                {index < STEPS.length - 1 && (
                  <div
                    className="w-12 h-0.5 mx-2"
                    style={{ backgroundColor: isComplete ? "var(--primary)" : "var(--border)" }}
                  />
                )}
              </div>
            );
          })}
        </div>

        {/* Step Content */}
        <div className="rounded-xl border shadow-sm" style={{ backgroundColor: "var(--card)", borderColor: "var(--border)" }}>
          {error && (
            <div className="p-4 border-b" style={{ borderColor: "var(--border)" }}>
              <Alert variant="error" onClose={() => setError(null)}>
                {error}
              </Alert>
            </div>
          )}

          {/* Welcome Step */}
          {currentStep === 0 && (
            <div className="p-8 text-center">
              <div className="mx-auto w-16 h-16 rounded-full flex items-center justify-center mb-6" style={{ backgroundColor: "var(--muted)" }}>
                <svg className="h-8 w-8" style={{ color: "var(--primary)" }} fill="none" viewBox="0 0 24 24" strokeWidth={1.5} stroke="currentColor">
                  <path strokeLinecap="round" strokeLinejoin="round" d="M15.59 14.37a6 6 0 0 1-5.84 7.38v-4.8m5.84-2.58a14.98 14.98 0 0 0 6.16-12.12A14.98 14.98 0 0 0 9.631 8.41m5.96 5.96a14.926 14.926 0 0 1-5.841 2.58m-.119-8.54a6 6 0 0 0-7.381 5.84h4.8m2.581-5.84a14.927 14.927 0 0 0-2.58 5.84m2.699 2.7c-.103.021-.207.041-.311.06a15.09 15.09 0 0 1-2.448-2.448 14.9 14.9 0 0 1 .06-.312m-2.24 2.39a4.493 4.493 0 0 0-1.757 4.306 4.493 4.493 0 0 0 4.306-1.758M16.5 9a1.5 1.5 0 1 1-3 0 1.5 1.5 0 0 1 3 0Z" />
                </svg>
              </div>
              <h2 className="font-[family-name:var(--font-syne)] text-2xl mb-2" style={{ color: "var(--foreground)" }}>
                Welcome to Hyperindex
              </h2>
              <p className="mb-8" style={{ color: "var(--muted-foreground)" }}>
                Let&apos;s set up your AT Protocol AppView server. This wizard will guide you
                through the initial configuration.
              </p>
              <div className="space-y-3 text-sm" style={{ color: "var(--muted-foreground)" }}>
                <p>You&apos;ll configure:</p>
                <ul className="space-y-2">
                  <li className="flex items-center justify-center gap-2">
                    <svg className="h-4 w-4" style={{ color: "var(--primary)" }} fill="none" viewBox="0 0 24 24" strokeWidth={1.5} stroke="currentColor">
                      <path strokeLinecap="round" strokeLinejoin="round" d="M12 21a9.004 9.004 0 0 0 8.716-6.747M12 21a9.004 9.004 0 0 1-8.716-6.747M12 21c2.485 0 4.5-4.03 4.5-9S14.485 3 12 3m0 18c-2.485 0-4.5-4.03-4.5-9S9.515 3 12 3m0 0a8.997 8.997 0 0 1 7.843 4.582M12 3a8.997 8.997 0 0 0-7.843 4.582m15.686 0A11.953 11.953 0 0 1 12 10.5c-2.998 0-5.74-1.1-7.843-2.918m15.686 0A8.959 8.959 0 0 1 21 12c0 .778-.099 1.533-.284 2.253m0 0A17.919 17.919 0 0 1 12 16.5c-3.162 0-6.133-.815-8.716-2.247m0 0A9.015 9.015 0 0 1 3 12c0-1.605.42-3.113 1.157-4.418" />
                    </svg>
                    Your domain authority
                  </li>
                  <li className="flex items-center justify-center gap-2">
                    <svg className="h-4 w-4" style={{ color: "var(--primary)" }} fill="none" viewBox="0 0 24 24" strokeWidth={1.5} stroke="currentColor">
                      <path strokeLinecap="round" strokeLinejoin="round" d="M18 7.5v3m0 0v3m0-3h3m-3 0h-3m-2.25-4.125a3.375 3.375 0 1 1-6.75 0 3.375 3.375 0 0 1 6.75 0ZM3 19.235v-.11a6.375 6.375 0 0 1 12.75 0v.109A12.318 12.318 0 0 1 9.374 21c-2.331 0-4.512-.645-6.374-1.766Z" />
                    </svg>
                    Your environment-managed admin list
                  </li>
                  <li className="flex items-center justify-center gap-2">
                    <svg className="h-4 w-4" style={{ color: "var(--primary)" }} fill="none" viewBox="0 0 24 24" strokeWidth={1.5} stroke="currentColor">
                      <path strokeLinecap="round" strokeLinejoin="round" d="M19.5 14.25v-2.625a3.375 3.375 0 0 0-3.375-3.375h-1.5A1.125 1.125 0 0 1 13.5 7.125v-1.5a3.375 3.375 0 0 0-3.375-3.375H8.25m0 12.75h7.5m-7.5 3H12M10.5 2.25H5.625c-.621 0-1.125.504-1.125 1.125v17.25c0 .621.504 1.125 1.125 1.125h12.75c.621 0 1.125-.504 1.125-1.125V11.25a9 9 0 0 0-9-9Z" />
                    </svg>
                    Your lexicon definitions
                  </li>
                </ul>
              </div>
            </div>
          )}

          {/* Domain Step */}
          {currentStep === 1 && (
            <div className="p-8">
              <div className="flex items-center gap-3 mb-6">
                <div className="w-10 h-10 rounded-full flex items-center justify-center" style={{ backgroundColor: "var(--muted)" }}>
                  <svg className="h-5 w-5" style={{ color: "var(--primary)" }} fill="none" viewBox="0 0 24 24" strokeWidth={1.5} stroke="currentColor">
                    <path strokeLinecap="round" strokeLinejoin="round" d="M12 21a9.004 9.004 0 0 0 8.716-6.747M12 21a9.004 9.004 0 0 1-8.716-6.747M12 21c2.485 0 4.5-4.03 4.5-9S14.485 3 12 3m0 18c-2.485 0-4.5-4.03-4.5-9S9.515 3 12 3m0 0a8.997 8.997 0 0 1 7.843 4.582M12 3a8.997 8.997 0 0 0-7.843 4.582m15.686 0A11.953 11.953 0 0 1 12 10.5c-2.998 0-5.74-1.1-7.843-2.918m15.686 0A8.959 8.959 0 0 1 21 12c0 .778-.099 1.533-.284 2.253m0 0A17.919 17.919 0 0 1 12 16.5c-3.162 0-6.133-.815-8.716-2.247m0 0A9.015 9.015 0 0 1 3 12c0-1.605.42-3.113 1.157-4.418" />
                  </svg>
                </div>
                <div>
                  <h3 className="font-[family-name:var(--font-syne)] text-xl" style={{ color: "var(--foreground)" }}>
                    Domain Authority
                  </h3>
                  <p className="text-sm" style={{ color: "var(--muted-foreground)" }}>
                    The domain where your AppView will be hosted
                  </p>
                </div>
              </div>
              <div className="space-y-4">
                <Input
                  label="Domain Authority"
                  placeholder="appview.example.com"
                  value={state.domainAuthority}
                  onChange={(e) =>
                    setState((s) => ({ ...s, domainAuthority: e.target.value }))
                  }
                  hint="This should be the public domain where your AppView is accessible. It's used for OAuth and federation."
                />
              </div>
            </div>
          )}

          {/* Admin Step */}
          {currentStep === 2 && (
            <div className="p-8">
              <div className="flex items-center gap-3 mb-6">
                <div className="w-10 h-10 rounded-full flex items-center justify-center" style={{ backgroundColor: "var(--muted)" }}>
                  <svg className="h-5 w-5" style={{ color: "var(--primary)" }} fill="none" viewBox="0 0 24 24" strokeWidth={1.5} stroke="currentColor">
                    <path strokeLinecap="round" strokeLinejoin="round" d="M18 7.5v3m0 0v3m0-3h3m-3 0h-3m-2.25-4.125a3.375 3.375 0 1 1-6.75 0 3.375 3.375 0 0 1 6.75 0ZM3 19.235v-.11a6.375 6.375 0 0 1 12.75 0v.109A12.318 12.318 0 0 1 9.374 21c-2.331 0-4.512-.645-6.374-1.766Z" />
                  </svg>
                </div>
                <div>
                  <h3 className="font-[family-name:var(--font-syne)] text-xl" style={{ color: "var(--foreground)" }}>
                    Administrators
                  </h3>
                  <p className="text-sm" style={{ color: "var(--muted-foreground)" }}>
                    Review the admin DIDs configured via environment variables
                  </p>
                </div>
              </div>
              <div className="space-y-4">
                <div className="rounded-lg border p-4" style={{ borderColor: "var(--border)", backgroundColor: "var(--muted)" }}>
                  <p className="text-sm font-medium" style={{ color: "var(--foreground)" }}>
                    Current administrators
                  </p>
                  {isAdminLoading || isSettingsLoading ? (
                    <div className="mt-3 space-y-2">
                      <div className="h-4 w-40 animate-pulse rounded" style={{ backgroundColor: "var(--border)" }} />
                      <div className="h-4 w-56 animate-pulse rounded" style={{ backgroundColor: "var(--border)" }} />
                    </div>
                  ) : adminDids.length === 0 ? (
                    <p className="mt-2 text-sm" style={{ color: "var(--muted-foreground)" }}>
                      No admin DIDs are configured on the backend.
                    </p>
                  ) : (
                    <ul className="mt-3 space-y-2">
                      {adminDids.map((did) => (
                        <li key={did}>
                          <code className="text-sm font-mono" style={{ color: "var(--foreground)" }}>{did}</code>
                        </li>
                      ))}
                    </ul>
                  )}
                </div>
                <p className="text-sm" style={{ color: "var(--muted-foreground)" }}>
                  Admin access is managed outside the app. Update <code className="font-mono">ADMIN_DIDS</code> on the backend if you need to change administrators.
                </p>
              </div>
            </div>
          )}

          {/* Lexicons Step */}
          {currentStep === 3 && (
            <div className="p-8">
              <div className="flex items-center gap-3 mb-6">
                <div className="w-10 h-10 rounded-full flex items-center justify-center" style={{ backgroundColor: "var(--muted)" }}>
                  <svg className="h-5 w-5" style={{ color: "var(--primary)" }} fill="none" viewBox="0 0 24 24" strokeWidth={1.5} stroke="currentColor">
                    <path strokeLinecap="round" strokeLinejoin="round" d="M19.5 14.25v-2.625a3.375 3.375 0 0 0-3.375-3.375h-1.5A1.125 1.125 0 0 1 13.5 7.125v-1.5a3.375 3.375 0 0 0-3.375-3.375H8.25m0 12.75h7.5m-7.5 3H12M10.5 2.25H5.625c-.621 0-1.125.504-1.125 1.125v17.25c0 .621.504 1.125 1.125 1.125h12.75c.621 0 1.125-.504 1.125-1.125V11.25a9 9 0 0 0-9-9Z" />
                  </svg>
                </div>
                <div>
                  <h3 className="font-[family-name:var(--font-syne)] text-xl" style={{ color: "var(--foreground)" }}>
                    Lexicons
                  </h3>
                  <p className="text-sm" style={{ color: "var(--muted-foreground)" }}>
                    Upload your AT Protocol lexicon definitions
                  </p>
                </div>
              </div>
              <div className="space-y-4">
                {state.lexiconsUploaded ? (
                  <div className="flex items-center gap-3 p-4 rounded-lg" style={{ backgroundColor: "oklch(0.65 0.15 155 / 0.08)", borderColor: "oklch(0.65 0.15 155 / 0.2)", border: "1px solid" }}>
                    <svg className="h-5 w-5" style={{ color: "var(--primary)" }} fill="none" viewBox="0 0 24 24" strokeWidth={1.5} stroke="currentColor">
                      <path strokeLinecap="round" strokeLinejoin="round" d="M9 12.75 11.25 15 15 9.75M21 12a9 9 0 1 1-18 0 9 9 0 0 1 18 0Z" />
                    </svg>
                    <div>
                      <div className="font-medium" style={{ color: "var(--primary)" }}>
                        Lexicons uploaded successfully
                      </div>
                      <div className="text-sm" style={{ color: "var(--muted-foreground)" }}>
                        You can upload more lexicons later from the Lexicons page
                      </div>
                    </div>
                  </div>
                ) : (
                  <div className="border-2 border-dashed rounded-lg p-8 text-center" style={{ borderColor: "var(--border)" }}>
                    <svg className="h-10 w-10 mx-auto mb-4" style={{ color: "var(--border)" }} fill="none" viewBox="0 0 24 24" strokeWidth={1.5} stroke="currentColor">
                      <path strokeLinecap="round" strokeLinejoin="round" d="M3 16.5v2.25A2.25 2.25 0 0 0 5.25 21h13.5A2.25 2.25 0 0 0 21 18.75V16.5m-13.5-9L12 3m0 0 4.5 4.5M12 3v13.5" />
                    </svg>
                    <p className="mb-4" style={{ color: "var(--muted-foreground)" }}>
                      Upload a ZIP file containing your lexicon JSON files
                    </p>
                    <input
                      type="file"
                      id="lexicon-upload"
                      className="hidden"
                      accept=".zip"
                      onChange={handleFileUpload}
                    />
                    <label htmlFor="lexicon-upload">
                      <Button
                        as="span"
                        variant="primary"
                        disabled={uploadLexiconsMutation.isPending}
                        loading={uploadLexiconsMutation.isPending}
                        className="cursor-pointer"
                      >
                        {uploadLexiconsMutation.isPending ? "Uploading..." : "Choose File"}
                      </Button>
                    </label>
                  </div>
                )}
                <p className="text-xs" style={{ color: "var(--muted-foreground)" }}>
                  You can skip this step and upload lexicons later. Your AppView will only
                  index records matching your installed lexicons.
                </p>
              </div>
            </div>
          )}

          {/* Complete Step */}
          {currentStep === 4 && (
            <div className="p-8 text-center">
              <div className="mx-auto w-16 h-16 rounded-full flex items-center justify-center mb-6" style={{ backgroundColor: "var(--muted)" }}>
                <svg className="h-8 w-8" style={{ color: "var(--primary)" }} fill="none" viewBox="0 0 24 24" strokeWidth={1.5} stroke="currentColor">
                  <path strokeLinecap="round" strokeLinejoin="round" d="M9 12.75 11.25 15 15 9.75M21 12a9 9 0 1 1-18 0 9 9 0 0 1 18 0Z" />
                </svg>
              </div>
              <h2 className="font-[family-name:var(--font-syne)] text-2xl mb-2" style={{ color: "var(--foreground)" }}>
                Setup Complete!
              </h2>
              <p className="mb-8" style={{ color: "var(--muted-foreground)" }}>
                Your Hyperindex AppView is ready to go.
              </p>
              <div className="rounded-lg p-4 mb-6 text-left" style={{ backgroundColor: "var(--muted)" }}>
                <h4 className="text-sm font-medium mb-3" style={{ color: "var(--secondary-foreground)" }}>Configuration Summary</h4>
                <dl className="space-y-2 text-sm">
                  <div className="flex justify-between">
                    <dt style={{ color: "var(--muted-foreground)" }}>Domain:</dt>
                    <dd className="font-mono" style={{ color: "var(--foreground)" }}>{state.domainAuthority || "Not set"}</dd>
                  </div>
                  <div className="flex justify-between">
                    <dt style={{ color: "var(--muted-foreground)" }}>Admins:</dt>
                    <dd className="font-mono truncate max-w-[200px]" style={{ color: "var(--foreground)" }}>
                      {adminDids.length > 0 ? adminDids.join(", ") : "Not set"}
                    </dd>
                  </div>
                  <div className="flex justify-between">
                    <dt style={{ color: "var(--muted-foreground)" }}>Lexicons:</dt>
                    <dd style={{ color: "var(--foreground)" }}>
                      {state.lexiconsUploaded ? "Uploaded" : "Skipped"}
                    </dd>
                  </div>
                </dl>
              </div>
              <p className="text-sm" style={{ color: "var(--muted-foreground)" }}>
                You can modify these settings anytime from the Settings page.
              </p>
            </div>
          )}

          {/* Navigation */}
          <div className="flex items-center justify-between p-6 border-t" style={{ borderColor: "var(--border)" }}>
            <Button
              variant="outline"
              onClick={handleBack}
              disabled={currentStep === 0 || isLoading}
            >
              <svg className="h-4 w-4 mr-2" fill="none" viewBox="0 0 24 24" strokeWidth={1.5} stroke="currentColor">
                <path strokeLinecap="round" strokeLinejoin="round" d="M10.5 19.5 3 12m0 0 7.5-7.5M3 12h18" />
              </svg>
              Back
            </Button>

            {currentStep === STEPS.length - 1 ? (
              <Button variant="primary" onClick={handleComplete}>
                Go to Dashboard
                <svg className="h-4 w-4 ml-2" fill="none" viewBox="0 0 24 24" strokeWidth={1.5} stroke="currentColor">
                  <path strokeLinecap="round" strokeLinejoin="round" d="M13.5 4.5 21 12m0 0-7.5 7.5M21 12H3" />
                </svg>
              </Button>
            ) : (
              <Button variant="primary" onClick={handleNext} disabled={isLoading} loading={isLoading}>
                {isLoading ? "Saving..." : currentStep === 3 ? "Finish" : "Next"}
                {!isLoading && (
                  <svg className="h-4 w-4 ml-2" fill="none" viewBox="0 0 24 24" strokeWidth={1.5} stroke="currentColor">
                    <path strokeLinecap="round" strokeLinejoin="round" d="M13.5 4.5 21 12m0 0-7.5 7.5M21 12H3" />
                  </svg>
                )}
              </Button>
            )}
          </div>
        </div>
      </div>
    </div>
  );
}
