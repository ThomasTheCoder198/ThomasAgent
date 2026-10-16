import type { ComponentProps, ReactNode } from "react";

import { cn } from "@/lib/cn";

export function OsWindow({ className, ...props }: ComponentProps<"div">) {
  return <div className={cn("os-window", className)} {...props} />;
}

export function OsTitlebar({
  title,
  closeControl,
  onClose,
  closeLabel,
  className,
}: {
  title?: ReactNode;
  closeControl?: ReactNode;
  onClose?: () => void;
  closeLabel?: string;
  className?: string;
}) {
  return (
    <div className={cn("os-titlebar", className)}>
      <div className="os-titlebar-close">
        {closeControl ??
          (onClose ? (
            <OsWindowClose onClick={onClose} aria-label={closeLabel} />
          ) : (
            <span className="os-titlebar-box" aria-hidden />
          ))}
      </div>
      <div className="os-titlebar-track">{title && <span className="os-titlebar-label">{title}</span>}</div>
      <span className="os-titlebar-box os-titlebar-min" aria-hidden />
      <span className="os-titlebar-box os-titlebar-max" aria-hidden />
    </div>
  );
}

export function OsWindowClose({ className, ...props }: ComponentProps<"button">) {
  return (
    <button type="button" className={cn("os-titlebar-close-button", className)} {...props}>
      <span className="os-titlebar-box" aria-hidden>
        ×
      </span>
    </button>
  );
}

export function OsWindowBody({ className, ...props }: ComponentProps<"div">) {
  return <div className={cn("os-window-body", className)} {...props} />;
}
