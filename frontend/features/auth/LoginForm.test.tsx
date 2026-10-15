import { act, render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { NextIntlClientProvider } from "next-intl";
import { toast } from "sonner";
import { afterEach, describe, expect, it, vi } from "vitest";

import { Toaster } from "@/components/ui/Sonner";
import messages from "@/messages/vi.json";

import { LoginForm } from "./LoginForm";

const { replace } = vi.hoisted(() => ({ replace: vi.fn() }));
vi.mock("next/navigation", () => ({ useRouter: () => ({ replace }) }));

afterEach(() => {
  act(() => void toast.dismiss());
  vi.unstubAllGlobals();
  replace.mockReset();
});

function renderForm() {
  render(
    <NextIntlClientProvider locale="vi" messages={messages}>
      <LoginForm next="/chat" />
      <Toaster />
    </NextIntlClientProvider>,
  );
}

async function fill(email: string, password: string) {
  if (email) await userEvent.type(screen.getByLabelText("Email"), email);
  if (password) await userEvent.type(screen.getByLabelText("Mật khẩu"), password);
  await userEvent.click(screen.getByRole("button", { name: "Đăng nhập" }));
}

const submit = () => fill("thomas@example.com", "correct horse battery");

describe("LoginForm", () => {
  it("toasts the localized catalog message when core rejects the sign-in", async () => {
    vi.stubGlobal(
      "fetch",
      vi.fn(
        async () =>
          new Response(
            JSON.stringify({ error: { code: "AUTH_INVALID_CREDENTIALS", message: "raw server words" } }),
            { status: 401 },
          ),
      ),
    );
    renderForm();
    await submit();
    expect(await screen.findByRole("alert")).toHaveTextContent("Email hoặc mật khẩu không đúng.");
    expect(replace).not.toHaveBeenCalled();
    expect(screen.getByRole("button", { name: "Đăng nhập" })).toBeEnabled();
  });

  it("shows the request id in the toast for a 5xx", async () => {
    vi.stubGlobal(
      "fetch",
      vi.fn(
        async () =>
          new Response(
            JSON.stringify({ error: { code: "INTERNAL_ERROR", message: "x", requestId: "req-500" } }),
            { status: 500 },
          ),
      ),
    );
    renderForm();
    await submit();
    expect(await screen.findByRole("alert")).toHaveTextContent("req-500");
  });

  it("explains missing or malformed fields in place without calling core", async () => {
    const fetchMock = vi.fn();
    vi.stubGlobal("fetch", fetchMock);
    renderForm();
    await fill("not-an-email", "");
    expect(await screen.findByText(messages.auth.emailInvalid)).toBeInTheDocument();
    expect(screen.getByText(messages.auth.passwordRequired)).toBeInTheDocument();
    expect(screen.getByLabelText("Email")).toBeInvalid();
    expect(fetchMock).not.toHaveBeenCalled();
  });

  it("redirects to next on success", async () => {
    vi.stubGlobal(
      "fetch",
      vi.fn(
        async () =>
          new Response(
            JSON.stringify({ data: { user: {}, csrfToken: "c", expiresAt: "" }, meta: { requestId: "r" } }),
            { status: 200 },
          ),
      ),
    );
    renderForm();
    await submit();
    await vi.waitFor(() => expect(replace).toHaveBeenCalledWith("/chat"));
  });
});
