import { act, render, screen, within } from "@testing-library/react";
import userEvent, { PointerEventsCheckLevel } from "@testing-library/user-event";
import { NextIntlClientProvider } from "next-intl";
import { useEffect, useState, type ReactNode } from "react";
import { afterEach, describe, expect, it, vi } from "vitest";

import messages from "@/messages/en.json";

import { Dialog, DialogContent } from "./Dialog";
import { INFO_TOAST_MS, MAX_TOASTS, ToastProvider, useToast, type ToastInput } from "./Toast";

const probe: { show?: (input: ToastInput) => void } = {};
const show = (input: ToastInput) => probe.show?.(input);
function Probe() {
  const api = useToast();
  useEffect(() => {
    probe.show = api.show;
  }, [api]);
  return null;
}

function renderToasts(page?: ReactNode) {
  return render(
    <NextIntlClientProvider locale="en" messages={messages}>
      <ToastProvider>
        <Probe />
        {page}
      </ToastProvider>
    </NextIntlClientProvider>,
  );
}

const dismissName = (title: string) => messages.common.dismissNotification.replace("{title}", title);

/** A modal dialog that is already open, like the app's org and KB dialogs. */
function OpenDialog() {
  const [open, setOpen] = useState(true);
  return (
    <Dialog open={open} onOpenChange={setOpen}>
      <DialogContent title="Organization settings">
        <button>Inside</button>
      </DialogContent>
    </Dialog>
  );
}

afterEach(() => vi.useRealTimers());

describe("Toast", () => {
  it("announces errors in the alert region and info in the status region, both mounted before any toast", () => {
    renderToasts();
    const alert = screen.getByRole("alert");
    const status = screen.getByRole("status");
    expect(alert).toBeEmptyDOMElement();
    act(() => show({ tone: "error", title: "a.md", detail: "Storage refused the upload." }));
    act(() => show({ tone: "info", title: "Saved" }));
    expect(within(alert).getByText("Storage refused the upload.")).toBeInTheDocument();
    expect(within(status).getByText("Saved")).toBeInTheDocument();
  });

  it("is dismissed by its button", async () => {
    renderToasts();
    act(() => show({ tone: "error", title: "a.md" }));
    await userEvent.click(screen.getByRole("button", { name: dismissName("a.md") }));
    expect(screen.getByRole("alert")).toBeEmptyDOMElement();
  });

  it("keeps an error until dismissed and lets info leave on its own", () => {
    vi.useFakeTimers();
    renderToasts();
    act(() => show({ tone: "error", title: "a.md" }));
    act(() => show({ tone: "info", title: "Saved" }));
    act(() => vi.advanceTimersByTime(INFO_TOAST_MS));
    expect(screen.getByRole("status")).toBeEmptyDOMElement();
    expect(screen.getByRole("alert")).toHaveTextContent("a.md");
  });

  it(`shows at most ${MAX_TOASTS}, dropping the oldest`, () => {
    renderToasts();
    act(() => {
      for (let i = 0; i <= MAX_TOASTS; i++) show({ tone: "error", title: `file-${i}` });
    });
    expect(screen.queryByText("file-0")).toBeNull();
    expect(screen.getByText(`file-${MAX_TOASTS}`)).toBeInTheDocument();
  });

  it("keeps both regions live and non-atomic, so a new error is read on its own", () => {
    renderToasts();
    const alert = document.querySelector('[role="alert"]');
    const status = document.querySelector('[role="status"]');
    expect(alert).toHaveAttribute("aria-live", "assertive");
    expect(alert).toHaveAttribute("aria-atomic", "false");
    expect(status).toHaveAttribute("aria-live", "polite");
    expect(status).toHaveAttribute("aria-atomic", "false");
  });

  it("is still announced while a modal dialog hides the rest of the page", async () => {
    renderToasts(<OpenDialog />);
    expect(await screen.findByRole("dialog")).toBeInTheDocument();
    act(() => show({ tone: "error", title: "a.md", detail: "Storage refused the upload." }));
    const alert = document.querySelector('[role="alert"]');
    expect(alert).toHaveTextContent("Storage refused the upload.");
    expect(alert?.closest('[aria-hidden="true"]')).toBeNull();
    expect(screen.getByRole("alert")).toBe(alert);
  });

  it("does not close an open dialog when a toast is dismissed", async () => {
    // A modal Radix layer sets pointer-events: none on <body>; the toast opts back in with a class jsdom cannot apply.
    const user = userEvent.setup({ pointerEventsCheck: PointerEventsCheckLevel.Never });
    renderToasts(<OpenDialog />);
    expect(await screen.findByRole("dialog")).toBeInTheDocument();
    act(() => show({ tone: "error", title: "a.md" }));
    await user.click(screen.getByRole("button", { name: dismissName("a.md") }));
    expect(screen.getByRole("alert")).toBeEmptyDOMElement();
    expect(screen.getByRole("dialog")).toBeInTheDocument();
  });

  it("names the toast in its dismiss button", () => {
    renderToasts();
    act(() => show({ tone: "error", title: "report.pdf" }));
    expect(screen.getByRole("button", { name: dismissName("report.pdf") })).toBeInTheDocument();
  });

  it("moves focus to the next toast, then to the region, never to <body>", async () => {
    renderToasts();
    act(() => {
      show({ tone: "error", title: "a.md" });
      show({ tone: "error", title: "b.md" });
    });
    await userEvent.click(screen.getByRole("button", { name: dismissName("a.md") }));
    expect(screen.getByRole("button", { name: dismissName("b.md") })).toHaveFocus();
    await userEvent.keyboard("{Enter}");
    expect(screen.getByRole("alert")).toHaveFocus();
  });

  it("clears an info timer when the toast is dismissed early or the provider unmounts", async () => {
    vi.useFakeTimers();
    const view = renderToasts();
    act(() => show({ tone: "info", title: "Saved" }));
    expect(vi.getTimerCount()).toBe(1);
    act(() => screen.getByRole("button", { name: dismissName("Saved") }).click());
    expect(vi.getTimerCount()).toBe(0);
    act(() => show({ tone: "info", title: "Saved again" }));
    expect(vi.getTimerCount()).toBe(1);
    view.unmount();
    expect(vi.getTimerCount()).toBe(0);
  });
});
