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
    frontendOrigin: "",
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
  const qrBeforePairing = screen.getByRole("img", {
    name: "Open mobile connection",
  }).outerHTML;
  expect(screen.getByLabelText("Add manual observations")).not.toBeChecked();
  await user.type(screen.getByLabelText("Device name"), "QA phone");
  await user.click(screen.getByLabelText("Add manual observations"));
  await user.click(
    screen.getByRole("button", { name: "Generate one-use token" }),
  );
  expect(
    await screen.findByDisplayValue("synthetic-one-use-token"),
  ).toHaveAttribute("readonly");
  expect(
    screen.getByRole("img", { name: "Open mobile connection" }).outerHTML,
  ).toBe(qrBeforePairing);
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

it.each([
  [
    "en",
    /computer hosting Nestworth/,
    /Closing the last window exits the app\./,
  ],
  ["zh-CN", /运行 Nestworth 的电脑/, /关闭最后一个窗口会退出应用。/],
  ["zh-TW", /執行 Nestworth 的電腦/, /關閉最後一個視窗會結束應用程式。/],
] as const)(
  "renders platform-neutral hosting and exit guidance from real %s resources",
  async (language, host, exit) => {
    await i18n.changeLanguage(language);
    const client = new QueryClient({
      defaultOptions: { queries: { retry: false } },
    });
    render(
      <QueryClientProvider client={client}>
        <MobileAccessSection />
      </QueryClientProvider>,
    );
    const help = i18n.getResource(
      language,
      "translation",
      "mobile.connectionHelp",
    );
    expect(await screen.findByText(help)).toHaveTextContent(host);
    expect(screen.getByText(help)).toHaveTextContent(exit);
    const unavailable = i18n.getResource(
      language,
      "translation",
      "mobile.connectionUnavailable",
    );
    expect(unavailable).toMatch(host);
    for (const copy of [help, unavailable])
      expect(copy).not.toMatch(/Mac(?:Book|OS)?|Windows|Linux/i);
  },
);

it("saves an explicit trusted frontend while retaining the private listener", async () => {
  const user = userEvent.setup();
  mobile.Configure.mockResolvedValue(status);
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  render(
    <QueryClientProvider client={client}>
      <MobileAccessSection />
    </QueryClientProvider>,
  );
  await screen.findByText("Running");
  await user.type(
    screen.getByLabelText("Trusted frontend origin (optional)"),
    "https://synthetic-frontend.example",
  );
  await user.click(
    screen.getByRole("button", { name: i18n.t("mobile.saveAccess") }),
  );
  expect(mobile.Configure).toHaveBeenCalledWith(
    expect.objectContaining({
      frontendOrigin: "https://synthetic-frontend.example",
      origin: status.config.origin,
      listen: status.config.listen,
    }),
  );
});
