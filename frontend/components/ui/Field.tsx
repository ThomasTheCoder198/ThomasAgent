"use client";

import { useId, type ReactNode } from "react";

import { cn } from "@/lib/cn";

import { Label } from "./Label";
import { Switch } from "./Switch";

/** What the control inside a Field must receive so its label, hint and error are announced with it. */
export type FieldControlProps = {
  id: string;
  "aria-describedby"?: string;
  "aria-invalid"?: true;
};

type Props = {
  label: string;
  hideLabel?: boolean;
  /** Guidance under the control (13px, muted). */
  hint?: string;
  /** Field-level validation message; marks the control invalid. */
  error?: string;
  /** A counter or limit, right-aligned under the control in mono. */
  footer?: string;
  className?: string;
  children: (control: FieldControlProps) => ReactNode;
};

/** Label above, control, then hint / error below. The control is built by the caller from `control`. */
export function Field({ label, hideLabel, hint, error, footer, className, children }: Props) {
  const id = useId();
  const hintId = `${id}-hint`;
  const errorId = `${id}-error`;
  const describedBy = [hint && hintId, error && errorId].filter(Boolean).join(" ");
  return (
    <div data-slot="field" className={cn("flex flex-col gap-1.5", className)}>
      <Label htmlFor={id} className={cn(hideLabel && "sr-only")}>
        {label}
      </Label>
      {children({
        id,
        "aria-describedby": describedBy || undefined,
        "aria-invalid": error ? true : undefined,
      })}
      {hint && (
        <p id={hintId} className="text-muted-foreground text-[0.8125rem] leading-snug">
          {hint}
        </p>
      )}
      {error && (
        <p id={errorId} className="text-destructive animate-rise text-[0.8125rem] leading-snug font-medium">
          {error}
        </p>
      )}
      {footer && <p className="tabular text-muted-foreground self-end text-xs">{footer}</p>}
    </div>
  );
}

/** A labelled on/off row; the hint says what changes, not how. The whole row is at least 44px tall below md. */
export function SwitchField({
  label,
  hint,
  checked,
  onCheckedChange,
  disabled,
}: {
  label: string;
  hint?: string;
  checked: boolean;
  onCheckedChange: (checked: boolean) => void;
  disabled?: boolean;
}) {
  const id = useId();
  const hintId = `${id}-hint`;
  return (
    <div data-slot="switch-field" className="flex items-start justify-between gap-4 max-md:min-h-11">
      <div className="flex flex-col gap-0.5">
        <Label htmlFor={id}>{label}</Label>
        {hint && (
          <p id={hintId} className="text-muted-foreground text-[0.8125rem] leading-snug">
            {hint}
          </p>
        )}
      </div>
      <Switch
        id={id}
        checked={checked}
        onCheckedChange={onCheckedChange}
        disabled={disabled}
        aria-describedby={hint ? hintId : undefined}
        className="mt-px"
      />
    </div>
  );
}
