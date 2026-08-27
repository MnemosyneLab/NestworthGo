import { beforeEach, describe, expect, it, vi } from "vitest";
import { render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { QueryClientProvider } from "@tanstack/react-query";
import { createTestQueryClient } from "@/test/queryClient";
import { OnboardingPage } from "./OnboardingPage";

const completeOnboarding = vi.fn().mockResolvedValue({});

vi.mock("../../../bindings/github.com/waltwang/nestworth-go/internal/wailsapi/household", () => ({
  Service: { CompleteOnboarding: (request: unknown) => completeOnboarding(request) },
}));
vi.mock("../../../bindings/github.com/waltwang/nestworth-go/internal/wailsapi/settings", () => ({
  Service: { SupportedCurrencies: () => Promise.resolve(["USD", "SGD", "CNY"]) },
}));
vi.mock("../../../bindings/github.com/waltwang/nestworth-go/internal/wailsapi/catalog", async () => {
  const { TEST_CATALOG } = await import("@/test/catalog");
  return { Service: { Catalog: () => Promise.resolve(TEST_CATALOG) } };
});

function renderPage() {
  const queryClient = createTestQueryClient();
  return render(
    <QueryClientProvider client={queryClient}>
      <OnboardingPage />
    </QueryClientProvider>,
  );
}

beforeEach(() => {
  completeOnboarding.mockClear();
});

describe("OnboardingPage", () => {
  it("blocks submission and shows a validation error when the household name is empty", async () => {
    renderPage();
    await userEvent.click(screen.getByRole("button", { name: /get started/i }));
    expect(await screen.findAllByRole("alert")).not.toHaveLength(0);
    expect(completeOnboarding).not.toHaveBeenCalled();
  });

  it("submits householdName, baseCurrency, and every member name on success", async () => {
    const onCompleted = vi.fn();
    render(
      <QueryClientProvider client={createTestQueryClient()}>
        <OnboardingPage onCompleted={onCompleted} />
      </QueryClientProvider>,
    );
    const form = screen.getByRole("form", { name: "Onboarding" });
    await userEvent.type(within(form).getByLabelText(/household name/i), "The Tans");
    await userEvent.type(within(form).getByLabelText("Member 1 name"), "Alice");
    await userEvent.click(within(form).getByRole("button", { name: /add/i }));
    await userEvent.type(within(form).getByLabelText("Member 2 name"), "Bob");
    await userEvent.click(within(form).getByRole("button", { name: /get started/i }));

    expect(completeOnboarding).toHaveBeenCalledWith(
      expect.objectContaining({ householdName: "The Tans", memberNames: ["Alice", "Bob"] }),
    );
    await waitFor(() => expect(onCompleted).toHaveBeenCalledTimes(1));
  });

  it("ignores an extra empty member row on submit", async () => {
    renderPage();
    const form = screen.getByRole("form", { name: "Onboarding" });
    await userEvent.type(within(form).getByLabelText(/household name/i), "The Tans");
    await userEvent.type(within(form).getByLabelText("Member 1 name"), "Alice");
    await userEvent.click(within(form).getByRole("button", { name: /add/i }));
    expect(within(form).getByLabelText("Member 2 name")).toBeInTheDocument();
    await userEvent.click(within(form).getByRole("button", { name: /get started/i }));

    await waitFor(() =>
      expect(completeOnboarding).toHaveBeenCalledWith(
        expect.objectContaining({ householdName: "The Tans", memberNames: ["Alice"] }),
      ),
    );
  });

  it("allows removing a member row once more than one exists", async () => {
    renderPage();
    await userEvent.type(screen.getByLabelText("Member 1 name"), "Alice");
    await userEvent.click(screen.getByRole("button", { name: /add/i }));
    expect(screen.getAllByRole("button", { name: /remove member/i })).toHaveLength(2);
    await userEvent.click(screen.getAllByRole("button", { name: /remove member/i })[0]);
    expect(screen.queryAllByRole("button", { name: /remove member/i })).toHaveLength(0);
  });
});
