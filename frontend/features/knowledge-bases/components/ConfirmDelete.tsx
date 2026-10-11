"use client";

import { useTranslations } from "next-intl";
import { useState } from "react";

import { Button } from "@/components/ui/Button";
import { Dialog, DialogClose, DialogContent } from "@/components/ui/Dialog";
import { InlineAlert } from "@/components/ui/InlineAlert";
import { describeApiError } from "@/lib/api/client";

type Props = {
  onOpenChange: (open: boolean) => void;
  title: string;
  body: string;
  /** Resolves when the item is gone; a rejection is shown here with its request id. */
  onConfirm: () => Promise<void>;
};

/** Names the item and what goes with it; a refusal (the default KB answers CONFLICT) stays inside the dialog. */
export function ConfirmDelete({ onOpenChange, title, body, onConfirm }: Props) {
  const t = useTranslations("kb");
  const tErr = useTranslations("errors");
  const [pending, setPending] = useState(false);
  const [failure, setFailure] = useState<string>();

  async function confirm() {
    setPending(true);
    setFailure(undefined);
    try {
      await onConfirm();
      onOpenChange(false);
    } catch (err) {
      setFailure(describeApiError(err, t("errors.deleteFailed"), (id) => tErr("requestId", { id })));
    } finally {
      setPending(false);
    }
  }

  return (
    <Dialog open onOpenChange={onOpenChange}>
      <DialogContent title={title} description={body}>
        <div className="flex flex-col gap-4">
          {failure && <InlineAlert title={failure} />}
          <div className="flex flex-wrap justify-end gap-2">
            <DialogClose asChild>
              <Button variant="ghost">{t("cancel")}</Button>
            </DialogClose>
            <Button onClick={() => void confirm()} pending={pending} disabled={pending}>
              {t("delete")}
            </Button>
          </div>
        </div>
      </DialogContent>
    </Dialog>
  );
}
