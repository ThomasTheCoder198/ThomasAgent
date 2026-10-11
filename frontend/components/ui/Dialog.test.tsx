import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { useState } from "react";
import { describe, expect, it } from "vitest";

import { Dialog, DialogContent } from "./Dialog";

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
          <DialogContent title="Title">
            <button>Inside</button>
          </DialogContent>
        </Dialog>
      )}
    </main>
  );
}

describe("DialogContent focus return", () => {
  it("returns focus to the element that opened it, not to <body>", async () => {
    render(<Harness />);
    await userEvent.click(screen.getByRole("button", { name: "Open" }));
    expect(await screen.findByRole("dialog")).toBeInTheDocument();
    await userEvent.keyboard("{Escape}");
    await waitFor(() => expect(screen.queryByRole("dialog")).not.toBeInTheDocument());
    await waitFor(() => expect(screen.getByRole("button", { name: "Open" })).toHaveFocus());
  });

  it("falls back to the main landmark when the opener is gone", async () => {
    render(<Harness removeOpenerOnOpen />);
    await userEvent.click(screen.getByRole("button", { name: "Open" }));
    expect(await screen.findByRole("dialog")).toBeInTheDocument();
    await userEvent.keyboard("{Escape}");
    await waitFor(() => expect(screen.queryByRole("dialog")).not.toBeInTheDocument());
    await waitFor(() => expect(screen.getByRole("main")).toHaveFocus());
  });
});
