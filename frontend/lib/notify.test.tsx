import { act, render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { NextIntlClientProvider } from "next-intl";
import { toast } from "sonner";
import { afterEach, describe, expect, it, vi } from "vitest";

import { Toaster } from "@/components/ui/Sonner";
import { ApiError } from "@/lib/api/client";
import { ErrorCode, errorDefinitions } from "@/lib/errors/codes.gen";
import messages from "@/messages/en.json";

import {
  AUTO_DISMISS_MS,
  describeError,
  notifyError,
  notifyInfo,
  notifyProblem,
  notifySuccess,
} from "./notify";

const EN = { locale: "en", requestIdLabel: "Request ID" } as const;
const VI = { locale: "vi", requestIdLabel: "Mã yêu cầu" } as const;
const HTTP_UNAVAILABLE = 503;
const HTTP_FORBIDDEN = 403;

const forbidden = new ApiError(ErrorCode.FORBIDDEN, "raw server words", HTTP_FORBIDDEN, undefined, "req-403");
const internal = new ApiError(ErrorCode.INTERNAL_ERROR, "raw server words", 500, undefined, "01J-REQUEST-ID");

function renderToaster() {
  return render(
    <NextIntlClientProvider locale="en" messages={messages}>
      <Toaster />
    </NextIntlClientProvider>,
  );
}

afterEach(() => {
  act(() => void toast.dismiss());
  vi.restoreAllMocks();
});

describe("describeError", () => {
  it("uses the catalog message for the code in the user's language, not the raw server string", () => {
    expect(describeError(forbidden, "vi").title).toBe(errorDefinitions[ErrorCode.FORBIDDEN].message.vi);
    expect(describeError(forbidden, "en").title).toBe(errorDefinitions[ErrorCode.FORBIDDEN].message.en);
  });

  it("carries the request id for 5xx only", () => {
    expect(describeError(internal, "en").requestId).toBe("01J-REQUEST-ID");
    expect(describeError(forbidden, "en").requestId).toBeUndefined();
  });

  it("maps anything that is not an ApiError to the network message", () => {
    const network = errorDefinitions[ErrorCode.CLIENT_NETWORK_ERROR];
    expect(describeError(new TypeError("Failed to fetch"), "vi")).toEqual({ title: network.message.vi });
    expect(describeError("boom", "en").title).toBe(network.message.en);
    expect(network.httpStatus).toBe(HTTP_UNAVAILABLE);
  });
});

describe("notify", () => {
  it("confirms and informs with a 4s auto-dismiss", () => {
    const success = vi.spyOn(toast, "success");
    const info = vi.spyOn(toast, "info");
    notifySuccess("Saved", "Details");
    notifyInfo("FYI");
    expect(success).toHaveBeenCalledWith("Saved", { description: "Details", duration: AUTO_DISMISS_MS });
    expect(info).toHaveBeenCalledWith("FYI", { description: undefined, duration: AUTO_DISMISS_MS });
  });

  it("keeps an error until it is dismissed", () => {
    const error = vi.spyOn(toast, "error");
    notifyProblem("a.md");
    expect(error.mock.calls[0]?.[1]).toMatchObject({ duration: Infinity, dismissible: true });
  });

  it("announces an error as an alert with the localized catalog message", async () => {
    renderToaster();
    act(() => void notifyError(forbidden, VI));
    const alert = await screen.findByRole("alert");
    expect(alert).toHaveTextContent(errorDefinitions[ErrorCode.FORBIDDEN].message.vi);
    expect(alert).not.toHaveTextContent("raw server words");
  });

  it("shows the request id in mono for a 5xx and not for a 4xx", async () => {
    renderToaster();
    act(() => void notifyError(internal, EN));
    const alert = await screen.findByRole("alert");
    const toastItem = alert.closest("li") as HTMLElement;
    const id = await within(toastItem).findByText("01J-REQUEST-ID");
    expect(id).toHaveClass("tabular");
    expect(toastItem).toHaveTextContent("Request ID: 01J-REQUEST-ID");
    act(() => void toast.dismiss());
    act(() => void notifyError(forbidden, EN));
    expect(screen.queryByText("req-403")).toBeNull();
  });

  it("falls back to the network message for an unknown failure", async () => {
    renderToaster();
    act(() => void notifyError(new Error("socket hang up"), EN));
    expect(await screen.findByRole("alert")).toHaveTextContent(
      errorDefinitions[ErrorCode.CLIENT_NETWORK_ERROR].message.en,
    );
  });

  it("is dismissed by its close button, which is named in the user's language", async () => {
    renderToaster();
    act(() => void notifyProblem("a.md"));
    await userEvent.click(await screen.findByRole("button", { name: messages.common.closeNotification }));
    await vi.waitFor(() => expect(screen.queryByText("a.md")).toBeNull());
  });
});
