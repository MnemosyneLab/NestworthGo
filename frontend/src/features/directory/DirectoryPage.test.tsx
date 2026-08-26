import { beforeEach, describe, expect, it, vi } from "vitest";
import { render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { QueryClientProvider } from "@tanstack/react-query";
import { createTestQueryClient } from "@/test/queryClient";
import { DirectoryPage } from "./DirectoryPage";

const listMembers = vi.fn();
const createMember = vi.fn();
const updateMember = vi.fn();
const archiveMember = vi.fn().mockResolvedValue(undefined);
const setMemberAvatar = vi.fn().mockResolvedValue(undefined);
const pickImage = vi.fn().mockResolvedValue("");
const createMediaAsset = vi.fn();

vi.mock("../../../bindings/github.com/waltwang/nestworth-go/internal/wailsapi/directory", () => ({
  Service: {
    ListMembers: (...args: unknown[]) => listMembers(...args),
    CreateMember: (...args: unknown[]) => createMember(...args),
    UpdateMember: (...args: unknown[]) => updateMember(...args),
    ArchiveMember: (...args: unknown[]) => archiveMember(...args),
    SetMemberAvatar: (...args: unknown[]) => setMemberAvatar(...args),
    ListInstitutions: () => Promise.resolve([]),
    ListGroups: () => Promise.resolve([]),
    CreateInstitution: vi.fn(),
    CreateGroup: vi.fn(),
    ArchiveInstitution: vi.fn(),
    ArchiveGroup: vi.fn(),
    UpdateInstitution: vi.fn(),
    UpdateGroup: vi.fn(),
    SetInstitutionLogo: vi.fn(),
    SetGroupLogo: vi.fn(),
  },
}));
vi.mock("../../../bindings/github.com/waltwang/nestworth-go/internal/wailsapi/media", () => ({
  Service: {
    PickImage: (...args: unknown[]) => pickImage(...args),
    CreateMediaAsset: (...args: unknown[]) => createMediaAsset(...args),
  },
}));

function renderPage() {
  const queryClient = createTestQueryClient();
  return render(
    <QueryClientProvider client={queryClient}>
      <DirectoryPage />
    </QueryClientProvider>,
  );
}

beforeEach(() => {
  listMembers.mockReset();
  createMember.mockReset();
  updateMember.mockReset();
  archiveMember.mockClear();
  setMemberAvatar.mockClear();
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

  it("renames a Member through UpdateMember", async () => {
    renderPage();
    await screen.findByText("Alice");
    await userEvent.click(screen.getByRole("button", { name: "Edit" }));
    const nameInput = screen.getByDisplayValue("Alice");
    await userEvent.clear(nameInput);
    await userEvent.type(nameInput, "Alicia");
    await userEvent.click(screen.getByRole("button", { name: "Save" }));
    expect(updateMember).toHaveBeenCalledWith("m1", "Alicia");
  });
});
