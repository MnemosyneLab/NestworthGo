import { useEffect, useId, useRef, useState } from "react";
import { parseWailsError } from "@/lib/wails";

/** A submission error stays associated with its editable field until that field
 * changes. Unknown/unavailable fields use the same focusable form summary. */
export function useSubmissionFeedback(cause: unknown, message: string | undefined, pending: boolean, formRef: React.RefObject<HTMLFormElement>, summaryRef: React.RefObject<HTMLParagraphElement>) {
  const errorId = useId();
  const [dismissed, setDismissed] = useState<unknown>();
  const handled = useRef<unknown>();
  if ((pending || !message) && dismissed !== undefined) setDismissed(undefined);
  const identity = cause ?? message;
  const field = cause ? parseWailsError(cause).field : undefined;
  const visible = !pending && identity !== dismissed ? message : undefined;

  useEffect(() => {
    if (pending || !message) {
      handled.current = undefined;
      return;
    }
    if (!visible || handled.current === identity) return;
    const form = formRef.current;
    if (!form || form.closest('[hidden], [inert], [aria-hidden="true"]')) return;
    handled.current = identity;
    const target = Array.from(form.querySelectorAll<HTMLElement>("[data-submission-field]"))
      .find(element => element.dataset.submissionField === field &&
        !element.matches(":disabled, [readonly]") &&
        !element.closest('[hidden], [inert], [aria-hidden="true"], details:not([open])'));
    (target ?? summaryRef.current)?.focus();
  }, [identity, message, pending, visible, field, formRef, summaryRef]);

  const clear = (changed: string) => {
    if (changed === field) setDismissed(identity);
  };
  const fieldProps = (name: string, invalid = false, description?: string) => ({
    "data-submission-field": name,
    "aria-invalid": invalid || Boolean(visible && field === name),
    "aria-describedby": [description, visible && field === name ? errorId : undefined].filter(Boolean).join(" ") || undefined,
  });
  return {
    errorId, message: visible, fieldProps, clear,
    onChangeCapture: (event: React.FormEvent<HTMLFormElement>) => {
      const target = event.target as HTMLElement;
      const control = target.closest<HTMLElement>("[data-submission-field]");
      if (control?.dataset.submissionField && !control.hasAttribute("data-submission-commit-only")) {
        clear(control.dataset.submissionField);
      }
    },
  };
}
