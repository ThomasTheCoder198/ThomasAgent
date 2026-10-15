"use client";

import { useTranslations } from "next-intl";
import { useState, type RefObject } from "react";

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
import { useNotify } from "@/lib/useNotify";

type Props = {
  onOpenChange: (open: boolean) => void;
  title: string;
  body: string;
  /** The success toast's text; a failure is toasted from its catalog code and the dialog stays for another try. */
  successMessage: string;
  /** Resolves when the item is gone. */
  onConfirm: () => Promise<void>;
  /** Where focus goes on close when the opener is gone (a menu item closes with its menu). */
  returnFocusRef?: RefObject<HTMLElement | null>;
};

/** Names the item and what goes with it, and reports the outcome as a toast. */
export function ConfirmDelete({
  onOpenChange,
  title,
  body,
  successMessage,
  onConfirm,
  returnFocusRef,
}: Props) {
  const t = useTranslations("kb");
  const notify = useNotify();
  const [pending, setPending] = useState(false);

  async function confirm() {
    setPending(true);
    try {
      await onConfirm();
      notify.success(successMessage);
      onOpenChange(false);
    } catch (err) {
      notify.error(err);
    } finally {
      setPending(false);
    }
  }

  return (
    <AlertDialog open onOpenChange={onOpenChange}>
      <AlertDialogContent returnFocusRef={returnFocusRef}>
        <AlertDialogHeader>
          <AlertDialogTitle>{title}</AlertDialogTitle>
          <AlertDialogDescription>{body}</AlertDialogDescription>
        </AlertDialogHeader>
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
