import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { NextIntlClientProvider } from "next-intl";
import { afterEach, describe, expect, it, vi } from "vitest";

import messages from "@/messages/vi.json";

import { LoginForm } from "./LoginForm";

const { replace } = vi.hoisted(() => ({ replace: vi.fn() }));
vi.mock("next/navigation", () => ({ useRouter: () => ({ replace }) }));

afterEach(() => {
  vi.unstubAllGlobals();
  replace.mockReset();
});

function renderForm() {
  render(
    <NextIntlClientProvider locale="vi" messages={messages}>
      <LoginForm next="/chat" />
    </NextIntlClientProvider>,
  );
}

async function submit() {
  await userEvent.type(screen.getByLabelText("Email"), "thomas@example.com");
  await userEvent.type(screen.getByLabelText("Mật khẩu"), "correct horse battery");
  await userEvent.click(screen.getByRole("button", { name: "Đăng nhập" }));
}

describe("LoginForm", () => {
  it("shows the server's localized error", async () => {
    vi.stubGlobal(
      "fetch",
      vi.fn(
        async () =>
          new Response(
            JSON.stringify({
              error: { code: "AUTH_INVALID_CREDENTIALS", message: "Email hoặc mật khẩu không đúng." },
            }),
            { status: 401 },
          ),
      ),
    );
    renderForm();
    await submit();
    expect(await screen.findByRole("alert")).toHaveTextContent("Email hoặc mật khẩu không đúng.");
    expect(replace).not.toHaveBeenCalled();
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
