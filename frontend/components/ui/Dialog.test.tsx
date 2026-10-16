import { act, render, screen, waitFor } from "@testing-library/react";
import userEvent, { PointerEventsCheckLevel } from "@testing-library/user-event";
import { NextIntlClientProvider } from "next-intl";
import { useState } from "react";
import { describe, expect, it } from "vitest";

import messages from "@/messages/en.json";

import { notifyProblem } from "@/lib/notify";

import { Dialog, DialogContent, DialogDescription, DialogHeader, DialogTitle } from "./Dialog";
import { Toaster } from "./Sonner";

/** Mounts the dialog already open, like the app's dialogs do: there is no Radix Trigger. */
function Harness({ removeOpenerOnOpen = false }: { removeOpenerOnOpen?: boolean }) {
  const [open, setOpen] = useState(false);
  const [showOpener, setShowOpener] = useState(true);
  return (
    <main>
      {showOpener && (
        <button
          onClick={() => {
            setOpen(true);
            if (removeOpenerOnOpen) setShowOpener(false);
          }}
        >
          Open
        </button>
      )}
      {open && (
        <Dialog open onOpenChange={setOpen}>
          <DialogContent>
            <DialogHeader>
              <DialogTitle>Title</DialogTitle>
              <DialogDescription>Body</DialogDescription>
            </DialogHeader>
            <button>Inside</button>
          </DialogContent>
        </Dialog>
      )}
    </main>
  );
}

function renderWithIntl(ui: React.ReactNode) {
  return render(
    <NextIntlClientProvider locale="en" messages={messages}>
      {ui}
    </NextIntlClientProvider>,
  );
}

describe("DialogContent focus return", () => {
  it("keeps the title as the accessible name and closes from the titlebar", async () => {
    renderWithIntl(<Harness />);
    await userEvent.click(screen.getByRole("button", { name: "Open" }));
    expect(screen.getByRole("dialog", { name: "Title" })).toBeInTheDocument();
    await userEvent.click(screen.getByRole("button", { name: messages.common.close }));
    await waitFor(() => expect(screen.queryByRole("dialog")).not.toBeInTheDocument());
  });

  it("returns focus to the element that opened it, not to <body>", async () => {
    renderWithIntl(<Harness />);
    await userEvent.click(screen.getByRole("button", { name: "Open" }));
    expect(await screen.findByRole("dialog")).toBeInTheDocument();
    await userEvent.keyboard("{Escape}");
    await waitFor(() => expect(screen.queryByRole("dialog")).not.toBeInTheDocument());
    await waitFor(() => expect(screen.getByRole("button", { name: "Open" })).toHaveFocus());
  });

  it("falls back to the main landmark when the opener is gone", async () => {
    renderWithIntl(<Harness removeOpenerOnOpen />);
    await userEvent.click(screen.getByRole("button", { name: "Open" }));
    expect(await screen.findByRole("dialog")).toBeInTheDocument();
    await userEvent.keyboard("{Escape}");
    await waitFor(() => expect(screen.queryByRole("dialog")).not.toBeInTheDocument());
    await waitFor(() => expect(screen.getByRole("main")).toHaveFocus());
  });
});

describe("DialogContent with toasts", () => {
  it("does not close when a toast above it is dismissed", async () => {
    // A modal Radix layer sets pointer-events: none on <body>; the toast opts back in with a class jsdom cannot apply.
    const user = userEvent.setup({ pointerEventsCheck: PointerEventsCheckLevel.Never });
    renderWithIntl(
      <>
        <Harness />
        <Toaster />
      </>,
    );
    await user.click(screen.getByRole("button", { name: "Open" }));
    expect(await screen.findByRole("dialog")).toBeInTheDocument();
    act(() => void notifyProblem("a.md"));
    const close = await screen.findByRole("button", { name: messages.common.closeNotification });
    await user.click(close);
    expect(screen.getByRole("dialog")).toBeInTheDocument();
  });
});
