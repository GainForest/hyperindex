import type { Metadata } from "next";
import { Inter, EB_Garamond } from "next/font/google";
import Image from "next/image";
import { Header } from "@/components/layout/Header";
import { GeometricBackground } from "@/components/layout/GeometricBackground";
import { Providers } from "@/components/Providers";
import "./globals.css";

const inter = Inter({
  variable: "--font-inter",
  subsets: ["latin"],
});

const garamond = EB_Garamond({
  variable: "--font-garamond",
  subsets: ["latin"],
  weight: ["400", "500", "600", "700"],
});

export const metadata: Metadata = {
  title: "Hypergoat",
  description: "AT Protocol AppView Server",
  icons: {
    icon: "/gainforest-logo.png",
    apple: "/gainforest-logo.png",
  },
};

export default function RootLayout({
  children,
}: Readonly<{
  children: React.ReactNode;
}>) {
  return (
    <html lang="en">
      <body
        className={`${inter.variable} ${garamond.variable} antialiased bg-white text-zinc-800`}
      >
        <Providers>
          <div className="relative min-h-screen overflow-hidden flex flex-col">
            {/* Animated geometric background */}
            <GeometricBackground />

            <Header />

            {/* Main content */}
            <main className="relative flex-1 max-w-3xl w-full mx-auto px-4 sm:px-6 pb-8">
              {children}
            </main>

            {/* Footer */}
            <footer className="relative py-6 mt-auto">
              <div className="max-w-3xl mx-auto px-4 sm:px-6">
                <a
                  href="https://gainforest.earth"
                  target="_blank"
                  rel="noopener noreferrer"
                  className="flex items-center justify-center gap-1.5 hover:opacity-80 transition-opacity"
                >
                  <span className="text-[11px] text-zinc-400 tracking-wide">
                    Made by
                  </span>
                  <Image
                    src="/gainforest-logo.png"
                    alt="GainForest"
                    width={14}
                    height={14}
                    className="inline-block"
                  />
                  <span className="text-[11px] text-emerald-600 font-medium tracking-wide">
                    GainForest
                  </span>
                </a>
              </div>
            </footer>
          </div>
        </Providers>
      </body>
    </html>
  );
}
