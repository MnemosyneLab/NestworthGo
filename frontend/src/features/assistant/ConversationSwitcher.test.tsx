import { expect, it, vi } from "vitest";
import { render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import i18n from "@/i18n";
import { ConversationSwitcher, groupKey, relativeUpdated, type SwitcherConversation } from "./ConversationSwitcher";

const now = new Date();
const iso = (offsetMs: number) => new Date(now.getTime() - offsetMs).toISOString();
const conversations: SwitcherConversation[] = [
  { id: "a", title: "Net worth review", savedTitle: "Net worth review", updated: iso(60_000) },
  { id: "b", title: "Conversation 2", savedTitle: "", updated: iso(5 * 86_400_000) },
];

function setup(overrides: Partial<Parameters<typeof ConversationSwitcher>[0]> = {}) {
  const props = {
    conversations, currentId: "a", disabled: false, mutationDisabled: false, createDisabled: false,
    onSelect: vi.fn(), onCreate: vi.fn(), onRename: vi.fn().mockResolvedValue(true), onDelete: vi.fn(), ...overrides,
  };
  return { props, user: userEvent.setup(), ...render(<ConversationSwitcher {...props} />) };
}

it("computes display-only groups and relative times", () => {
  const noon = new Date(2026, 10, 9, 12);
  expect(groupKey(new Date(2026, 10, 9, 1).toISOString(), noon)).toBe("groupToday");
  expect(groupKey(new Date(2026, 10, 8, 23).toISOString(), noon)).toBe("groupYesterday");
  expect(groupKey(new Date(2026, 10, 1).toISOString(), noon)).toBe("groupEarlier");
  expect(groupKey("", noon)).toBe("groupEarlier");
  expect(relativeUpdated(new Date(noon.getTime() - 3 * 3600_000).toISOString(), "en", noon.getTime())).toBe("3 hours ago");
  expect(relativeUpdated("not-a-date", "en")).toBe("");
});

it("shows the active conversation, groups the list and selects another row", async () => {
  await i18n.changeLanguage("en");
  const { props, user } = setup();
  const trigger = screen.getByRole("button", { name: "Conversations: Net worth review" });
  await user.click(trigger);
  const popover = await screen.findByRole("dialog", { name: "Conversations" });
  expect(within(popover).getByRole("region", { name: "Today" })).toBeInTheDocument();
  expect(within(popover).getByRole("region", { name: "Earlier" })).toBeInTheDocument();
  expect(within(popover).getByRole("button", { name: /^Net worth review/ })).toHaveAttribute("aria-current", "true");
  await user.click(within(popover).getByRole("button", { name: /^Conversation 2/ }));
  expect(props.onSelect).toHaveBeenCalledExactlyOnceWith("b");
  await waitFor(() => expect(screen.queryByRole("dialog", { name: "Conversations" })).not.toBeInTheDocument());
});

it("renames a row inline from its stored title and keeps the draft when storage fails", async () => {
  const onRename = vi.fn().mockResolvedValueOnce(false).mockResolvedValueOnce(true);
  const { user } = setup({ onRename });
  await user.click(screen.getByRole("button", { name: /^Conversations:/ }));
  await user.click(await screen.findByRole("button", { name: "Rename conversation: Conversation 2" }));
  const input = screen.getByRole("textbox", { name: "Conversation title" });
  expect(input).toHaveValue("");
  await user.type(input, "Budget");
  await user.click(screen.getByRole("button", { name: "Save title" }));
  await waitFor(() => expect(onRename).toHaveBeenCalledWith("b", "Budget"));
  expect(screen.getByRole("textbox", { name: "Conversation title" })).toHaveValue("Budget");
  await user.click(screen.getByRole("button", { name: "Save title" }));
  await waitFor(() => expect(screen.queryByRole("textbox", { name: "Conversation title" })).not.toBeInTheDocument());
});

it("cancels an inline rename with Escape without closing the list or writing", async () => {
  const { props, user } = setup();
  await user.click(screen.getByRole("button", { name: /^Conversations:/ }));
  await user.click(await screen.findByRole("button", { name: "Rename conversation: Net worth review" }));
  await user.type(screen.getByRole("textbox", { name: "Conversation title" }), "x{Escape}");
  expect(screen.queryByRole("textbox", { name: "Conversation title" })).not.toBeInTheDocument();
  expect(screen.getByRole("dialog", { name: "Conversations" })).toBeVisible();
  expect(props.onRename).not.toHaveBeenCalled();
});

it("requires confirmation before deleting and lets the user keep the conversation", async () => {
  const { props, user } = setup();
  await user.click(screen.getByRole("button", { name: /^Conversations:/ }));
  await user.click(await screen.findByRole("button", { name: "Delete conversation: Conversation 2" }));
  const dialog = await screen.findByRole("alertdialog");
  expect(dialog).toHaveTextContent("“Conversation 2” will be removed from the local history.");
  await user.click(within(dialog).getByRole("button", { name: "Keep" }));
  await waitFor(() => expect(screen.queryByRole("alertdialog")).not.toBeInTheDocument());
  expect(props.onDelete).not.toHaveBeenCalled();
  await user.click(screen.getByRole("button", { name: /^Conversations:/ }));
  await user.click(await screen.findByRole("button", { name: "Delete conversation: Conversation 2" }));
  await user.click(within(await screen.findByRole("alertdialog")).getByRole("button", { name: "Delete" }));
  expect(props.onDelete).toHaveBeenCalledExactlyOnceWith("b");
});

it("disables rename and delete while a mutation is blocked and exposes New conversation", async () => {
  const { props, user } = setup({ mutationDisabled: true });
  await user.click(screen.getByRole("button", { name: /^Conversations:/ }));
  expect(await screen.findByRole("button", { name: "Rename conversation: Conversation 2" })).toBeDisabled();
  expect(screen.getByRole("button", { name: "Delete conversation: Conversation 2" })).toBeDisabled();
  await user.keyboard("{Escape}");
  await user.click(screen.getByRole("button", { name: "New conversation" }));
  expect(props.onCreate).toHaveBeenCalledOnce();
});

it("disables the list while busy or empty and never shows identifiers", () => {
  const { unmount } = setup({ disabled: true });
  expect(screen.getByRole("button", { name: /^Conversations:/ })).toBeDisabled();
  expect(document.body).not.toHaveTextContent(/\bid\b|uuid/i);
  unmount();
  setup({ conversations: [], currentId: "" });
  expect(screen.getByRole("button", { name: "Conversations" })).toBeDisabled();
});


it.each(["disabled", "mutationDisabled"] as const)("blocks an existing delete confirmation when %s changes", async blocked => {
  const { props, user, rerender } = setup();
  await user.click(screen.getByRole("button", { name: /^Conversations:/ }));
  await user.click(await screen.findByRole("button", { name: "Delete conversation: Conversation 2" }));
  const dialog = await screen.findByRole("alertdialog");
  rerender(<ConversationSwitcher {...props} {...{ [blocked]: true }} />);
  const confirm = within(dialog).getByRole("button", { name: "Delete" });
  expect(confirm).toBeDisabled();
  await user.click(confirm);
  await user.keyboard("{Enter}");
  expect(props.onDelete).not.toHaveBeenCalled();
  await user.click(within(dialog).getByRole("button", { name: "Keep" }));
  await waitFor(() => expect(screen.queryByRole("alertdialog")).not.toBeInTheDocument());
});


it("groups by local calendar days across both daylight-saving transitions", () => {
  vi.stubEnv("TZ", "America/New_York");
  try {
    // Fall back: yesterday lasts 25 hours; its first hour must stay Yesterday.
    const fall = new Date(2026, 10, 2, 12);
    expect(new Date(2026, 10, 1).getTimezoneOffset()).toBe(240);
    expect(fall.getTimezoneOffset()).toBe(300);
    expect(groupKey(new Date(2026, 10, 1, 0, 30).toISOString(), fall)).toBe("groupYesterday");
    expect(groupKey(new Date(2026, 9, 31, 23, 59).toISOString(), fall)).toBe("groupEarlier");
    // Spring forward: yesterday lasts 23 hours; the day before stays Earlier.
    const spring = new Date(2026, 2, 9, 12);
    expect(new Date(2026, 2, 8).getTimezoneOffset()).toBe(300);
    expect(spring.getTimezoneOffset()).toBe(240);
    expect(groupKey(new Date(2026, 2, 7, 23, 30).toISOString(), spring)).toBe("groupEarlier");
    expect(groupKey(new Date(2026, 2, 8, 0, 0).toISOString(), spring)).toBe("groupYesterday");
    expect(groupKey(new Date(2026, 2, 9, 0, 0).toISOString(), spring)).toBe("groupToday");
  } finally {
    vi.unstubAllEnvs();
  }
});
