import { act, fireEvent, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { expect, it, vi } from "vitest";
import { ErrorState, RetryableLoadState } from "./PageState";

function deferred() {
  let resolve!: (value?: unknown) => void;
  let reject!: (error: unknown) => void;
  const promise = new Promise<unknown>((yes, no) => { resolve = yes; reject = no; });
  return { promise, resolve, reject };
}
const copy = { title: "Unable to load", description: "Please try again." };

it("keeps focus, blocks repeated mouse and keyboard activation, and unlocks after rejection", async () => {
  const request = deferred();
  const retry = vi.fn().mockReturnValueOnce(request.promise).mockResolvedValue(undefined);
  render(<ErrorState {...copy} onRetry={retry} />);
  const button = screen.getByRole("button", { name: "Try again" });
  await userEvent.click(button);
  expect(button).toHaveFocus();
  expect(button).toHaveAttribute("aria-disabled", "true");
  expect(button).toHaveAttribute("aria-busy", "true");
  expect(button).toHaveTextContent("Loading");
  fireEvent.click(button);
  await userEvent.keyboard("{Enter} ");
  expect(retry).toHaveBeenCalledTimes(1);
  await act(async () => request.reject(new Error("private backend details")));
  await waitFor(() => expect(button).toHaveAttribute("aria-disabled", "false"));
  expect(screen.queryByText("private backend details")).not.toBeInTheDocument();
  expect(screen.getByText(copy.description)).toBeVisible();
  await userEvent.click(button);
  expect(retry).toHaveBeenCalledTimes(2);
});

it("waits for every request in an aggregate retry even when one rejects", async () => {
  const first = deferred(), second = deferred();
  render(<ErrorState {...copy} onRetry={() => Promise.allSettled([first.promise, second.promise])} />);
  await userEvent.click(screen.getByRole("button"));
  await act(async () => first.reject(new Error("failed")));
  expect(screen.getByRole("button")).toHaveAttribute("aria-busy", "true");
  await act(async () => second.resolve());
  expect(screen.getByRole("button")).toHaveTextContent("Try again");
});

it("recovers from synchronous throws and respects externally owned pending state", async () => {
  const retry = vi.fn(() => { throw new Error("failed"); });
  const view = render(<ErrorState {...copy} onRetry={retry} isRetrying />);
  fireEvent.click(screen.getByRole("button"));
  expect(retry).not.toHaveBeenCalled();
  view.rerender(<ErrorState {...copy} onRetry={retry} />);
  await userEvent.click(screen.getByRole("button"));
  expect(screen.getByRole("button")).toHaveAttribute("aria-busy", "false");
  await userEvent.click(screen.getByRole("button"));
  expect(retry).toHaveBeenCalledTimes(2);
});

it("allows completion after the error view unmounts", async () => {
  const request = deferred();
  const view = render(<ErrorState {...copy} onRetry={() => request.promise} />);
  await userEvent.click(screen.getByRole("button"));
  view.unmount();
  await act(async () => request.resolve());
  expect(screen.queryByRole("button")).not.toBeInTheDocument();
});


it("focuses pointer-activated retries even when the browser does not focus buttons", async () => {
  const request = deferred();
  render(<ErrorState {...copy} onRetry={() => request.promise} />);
  const button = screen.getByRole("button");
  // Unlike userEvent.click, fireEvent does not synthesize browser focus.
  fireEvent.click(button);
  expect(button).toHaveFocus();
  button.blur();
  fireEvent.click(button);
  expect(button).toHaveFocus();
  await act(async () => request.reject(new Error("failed")));
  expect(button).toHaveFocus();
});

it("retains the retry control through loading transitions without stealing moved focus", async () => {
  const request = deferred();
  const retry = () => request.promise;
  const view = (isError: boolean) => <><button>Other action</button><RetryableLoadState {...copy} loadingLabel="Loading page" isError={isError} onRetry={retry} /></>;
  const rendered = render(view(false));
  expect(screen.getByRole("status")).toHaveTextContent("Loading page");
  rendered.rerender(view(true));
  const button = screen.getByRole("button", { name: "Try again" });
  await userEvent.click(button);
  rendered.rerender(view(false));
  expect(screen.getByRole("button", { name: "Loading" })).toBe(button);
  expect(button).toHaveFocus();
  await userEvent.click(screen.getByRole("button", { name: "Other action" }));
  rendered.rerender(view(true));
  await act(async () => request.reject(new Error("failed")));
  expect(screen.getByRole("button", { name: "Other action" })).toHaveFocus();
  expect(screen.getByRole("button", { name: "Try again" })).toBe(button);
});
