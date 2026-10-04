import type { Metadata, Viewport } from "next";
import { Geist_Mono, Lexend } from "next/font/google";
import { NextIntlClientProvider } from "next-intl";
import { getLocale } from "next-intl/server";
import { ThemeProvider } from "next-themes";
import type { ReactNode } from "react";

import "./globals.css";

const lexend = Lexend({
  subsets: ["latin", "latin-ext", "vietnamese"],
  variable: "--font-lexend",
  display: "swap",
});
const geistMono = Geist_Mono({
  subsets: ["latin", "latin-ext"],
  variable: "--font-geist-mono",
  display: "swap",
});

export const metadata: Metadata = { title: "ThomasAgent", description: "ThomasAgent workspace" };

export const viewport: Viewport = {
  themeColor: [
    { media: "(prefers-color-scheme: light)", color: "#141414" },
    { media: "(prefers-color-scheme: dark)", color: "#08090b" },
  ],
};

export default async function RootLayout({ children }: { children: ReactNode }) {
  const locale = await getLocale();
  return (
    <html lang={locale} suppressHydrationWarning className={`${lexend.variable} ${geistMono.variable}`}>
      <body className="bg-ground text-ink min-h-dvh">
        <ThemeProvider attribute="data-theme" defaultTheme="system" enableSystem disableTransitionOnChange>
          <NextIntlClientProvider>{children}</NextIntlClientProvider>
        </ThemeProvider>
      </body>
    </html>
  );
}
