import { getTranslations } from "next-intl/server";

import { BrandMark } from "@/components/BrandMark";
import { LoginForm } from "@/features/auth/LoginForm";
import { safeNext } from "@/features/auth/paths";
import { PreferencesMenu } from "@/features/preferences/PreferencesMenu";

/** The product's second hero: a centred column on the page background, no card, no side panel. */
export default async function LoginPage({ searchParams }: { searchParams: Promise<{ next?: string }> }) {
  const t = await getTranslations("auth");
  const { next } = await searchParams;
  return (
    <main className="flex min-h-dvh flex-col">
      <header className="flex h-14 shrink-0 items-center justify-between px-4 md:px-6 lg:px-8">
        <BrandMark label="ThomasAgent" />
        <PreferencesMenu />
      </header>
      {/* The bottom padding offsets the header, so the form sits on the optical centre of the viewport. */}
      <section className="animate-rise mx-auto flex w-full max-w-sm flex-1 flex-col justify-center px-4 pt-8 pb-16 md:pb-20">
        <h1 className="font-serif text-[clamp(2rem,4vw,2.75rem)] leading-[1.15] font-medium tracking-[-0.01em]">
          {t("title")}
        </h1>
        <p className="text-muted-foreground mt-3 mb-8">{t("subtitle")}</p>
        <LoginForm next={safeNext(next)} />
      </section>
    </main>
  );
}
