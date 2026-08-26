import { beforeEach, describe, expect, it, vi } from "vitest";
import { render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { DirectoryPage } from "./DirectoryPage";

const listMembers = vi.fn();
const createMember = vi.fn();
const archiveMember = vi.fn().mockResolvedValue(undefined);

vi.mock("../../../bindings/github.com/waltwang/nestworth-go/internal/wailsapi/directory", () => ({
  Service: {
    ListMembers: (...args: unknown[]) => listMembers(...args),
    CreateMember: (...args: unknown[]) => createMember(...args),
    ArchiveMember: (...args: unknown[]) => archiveMember(...args),
    ListInstitutions: () => Promise.resolve([]),
    ListGroups: () => Promise.resolve([]),
    CreateInstitution: vi.fn(),
    CreateGroup: vi.fn(),
    ArchiveInstitution: vi.fn(),
    ArchiveGroup: vi.fn(),
  },
}));

function renderPage() {
  const queryClient = new QueryClient();
  return render(
    <QueryClientProvider client={queryClient}>
      <DirectoryPage />
    </QueryClientProvider>,
  );
}

beforeEach(() => {
  listMembers.mockReset();
  createMember.mockReset();
  archiveMember.mockClear();
  listMembers.mockResolvedValue([{ id: "m1", name: "Alice" }]);
  createMember.mockResolvedValue({ id: "m2", name: "Bob" });
});

describe("DirectoryPage", () => {
  it("lists existing Members on the default tab", async () => {
    renderPage();
    expect(await screen.findByText("Alice")).toBeInTheDocument();
  });

  it("creates a new Member from the inline form", async () => {
    renderPage();
    await screen.findByText("Alice");
    const form = screen.getByRole("form", { name: "Member name" });
    await userEvent.type(within(form).getByRole("textbox"), "Bob");
    await userEvent.click(within(form).getByRole("button", { name: /add/i }));
    expect(createMember).toHaveBeenCalledWith("Bob");
  });

  it("archives a Member after AlertDialog confirmation", async () => {
    renderPage();
    await screen.findByText("Alice");
    await userEvent.click(screen.getByRole("button", { name: "Archive" }));
    const dialog = await screen.findByRole("alertdialog");
    await userEvent.click(within(dialog).getByRole("button", { name: "Archive" }));
    expect(archiveMember).toHaveBeenCalledWith("m1", true);
  });

  it("switches to the Institutions tab", async () => {
    renderPage();
    await screen.findByText("Alice");
    await userEvent.click(screen.getByRole("tab", { name: "Institutions" }));
    expect(screen.getByRole("tabpanel", { name: "Institutions" })).toBeInTheDocument();
  });
});
