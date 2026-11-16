import { useRef } from "react";
import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { expect, it } from "vitest";
import { useSubmissionFeedback } from "./useSubmissionFeedback";

const rejection = (field: string) => Object.assign(new Error("Please correct this value"), { wireError: { code: "validation", field, message: "invalid" } });
function Fixture({ cause, pending = false, unavailable = false }: { cause?: Error; pending?: boolean; unavailable?: boolean }) {
  const formRef = useRef<HTMLFormElement>(null);
  const summaryRef = useRef<HTMLParagraphElement>(null);
  const feedback = useSubmissionFeedback(cause, cause?.message, pending, formRef, summaryRef);
  return <form ref={formRef} onChangeCapture={feedback.onChangeCapture}>
    <input aria-label="Name" disabled={pending} readOnly={unavailable} {...feedback.fieldProps("name", false, "name-help")} />
    <span id="name-help">Existing help</span>
    <input aria-label="Other" />
    {feedback.message && <p role="alert" tabIndex={-1} id={feedback.errorId} ref={summaryRef}>{feedback.message}</p>}
  </form>;
}

it("waits for enabled controls, preserves help and clears only after the rejected field changes", async () => {
  const cause = rejection("name");
  const view = render(<Fixture cause={cause} pending />);
  expect(screen.queryByRole("alert")).not.toBeInTheDocument();
  view.rerender(<Fixture cause={cause} />);
  const name = screen.getByLabelText("Name");
  await waitFor(() => expect(name).toHaveFocus());
  expect(name).toHaveAttribute("aria-invalid", "true");
  expect(name).toHaveAccessibleDescription("Existing help Please correct this value");
  await userEvent.type(screen.getByLabelText("Other"), "unrelated");
  expect(screen.getByRole("alert")).toBeInTheDocument();
  await userEvent.type(name, "corrected");
  expect(name).toHaveAttribute("aria-invalid", "false");
  expect(name).toHaveAccessibleDescription("Existing help");
  expect(screen.queryByRole("alert")).not.toBeInTheDocument();
  // A retry can return the same error object; it must announce and focus again.
  view.rerender(<Fixture cause={cause} pending />);
  view.rerender(<Fixture cause={cause} />);
  await waitFor(() => expect(name).toHaveFocus());
  expect(screen.getByRole("alert")).toBeInTheDocument();
});

it.each(["unknown", "readonly"])("focuses the summary for an %s target", async kind => {
  render(<Fixture cause={rejection(kind === "unknown" ? "unmapped" : "name")} unavailable={kind === "readonly"} />);
  await waitFor(() => expect(screen.getByRole("alert")).toHaveFocus());
});
