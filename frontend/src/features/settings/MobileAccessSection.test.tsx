import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { beforeEach, expect, it, vi } from "vitest";
import i18n from "@/i18n";
import { MobileAccessSection } from "./MobileAccessSection";

const mobile = vi.hoisted(() => ({
  Status: vi.fn(),
  Configure: vi.fn(),
  Pair: vi.fn(),
  Revoke: vi.fn(),
}));
vi.mock(
  "../../../bindings/github.com/waltwang/nestworth-go/internal/wailsapi/mobile/service",
  () => mobile,
);
vi.mock("@/lib/wails", () => ({
  callService: (run: () => Promise<unknown>) => run(),
}));
const status = {
  config: {
    enabled: true,
    origin: "https://synthetic.private.example",
    listen: "127.0.0.1:9443",
    mode: "loopback_proxy",
    certificate: "",
    key: "",
  },
  running: true,
  devices: [{ id: "synthetic-device", name: "Synthetic phone" }],
};
beforeEach(async () => {
  vi.clearAllMocks();
  await i18n.changeLanguage("en");
  mobile.Status.mockResolvedValue(status);
  mobile.Pair.mockResolvedValue({
    token: "synthetic-one-use-token",
    expiresAt: "2026-10-30T12:10:00Z",
    url: status.config.origin,
  });
  mobile.Revoke.mockResolvedValue(status);
});

it("requires explicit manual permission and clears the once-only token when leaving the section", async () => {
  const user = userEvent.setup();
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  const view = render(
    <QueryClientProvider client={client}>
      <MobileAccessSection />
    </QueryClientProvider>,
  );
  await screen.findByText("Running");
  expect(screen.getByLabelText("Add manual observations")).not.toBeChecked();
  await user.type(screen.getByLabelText("Device name"), "QA phone");
  await user.click(screen.getByLabelText("Add manual observations"));
  await user.click(
    screen.getByRole("button", { name: "Generate one-use token" }),
  );
  expect(
    await screen.findByDisplayValue("synthetic-one-use-token"),
  ).toHaveAttribute("readonly");
  expect(mobile.Pair).toHaveBeenCalledWith("QA phone", {
    read: true,
    record: true,
    market_refresh: true,
    manual_quote: true,
  });
  view.rerender(
    <QueryClientProvider client={client}>
      <MobileAccessSection active={false} />
    </QueryClientProvider>,
  );
  expect(
    screen.queryByDisplayValue("synthetic-one-use-token"),
  ).not.toBeInTheDocument();
});

it("revokes one named phone and all phones without exposing token history", async () => {
  const user = userEvent.setup();
  const client = new QueryClient();
  render(
    <QueryClientProvider client={client}>
      <MobileAccessSection />
    </QueryClientProvider>,
  );
  await screen.findByText("Synthetic phone");
  await user.click(screen.getByRole("button", { name: "Revoke device" }));
  expect(mobile.Revoke).toHaveBeenCalledWith("synthetic-device");
  await user.click(screen.getByRole("button", { name: "Sign out all phones" }));
  expect(mobile.Revoke).toHaveBeenLastCalledWith("");
  expect(screen.queryByLabelText("Mobile token")).not.toBeInTheDocument();
});
