"use client";

import { useTranslations } from "next-intl";
import { useState } from "react";

import {
  AlertDialog,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle,
} from "@/components/ui/AlertDialog";
import { Button } from "@/components/ui/Button";
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
    <AlertDialog open onOpenChange={onOpenChange}>
      <AlertDialogContent>
        <AlertDialogHeader>
          <AlertDialogTitle>{title}</AlertDialogTitle>
          <AlertDialogDescription>{body}</AlertDialogDescription>
        </AlertDialogHeader>
        {failure && <InlineAlert title={failure} />}
        <AlertDialogFooter>
          <AlertDialogCancel variant="ghost">{t("cancel")}</AlertDialogCancel>
          {/* A plain Button, not AlertDialogAction: the dialog must stay open while the request runs or fails. */}
          <Button variant="destructive" onClick={() => void confirm()} pending={pending}>
            {t("delete")}
          </Button>
        </AlertDialogFooter>
      </AlertDialogContent>
    </AlertDialog>
  );
}
