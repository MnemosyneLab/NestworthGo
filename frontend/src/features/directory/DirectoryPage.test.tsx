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
const setMemberIcon = vi.fn().mockResolvedValue(undefined);
const createInstitution = vi.fn();

vi.mock("../../../bindings/github.com/waltwang/nestworth-go/internal/wailsapi/directory", () => ({
  Service: {
    ListMembers: (...args: unknown[]) => listMembers(...args),
    CreateMember: (...args: unknown[]) => createMember(...args),
    UpdateMember: (...args: unknown[]) => updateMember(...args),
    ArchiveMember: (...args: unknown[]) => archiveMember(...args),
    SetMemberIcon: (...args: unknown[]) => setMemberIcon(...args),
    ListInstitutions: () => Promise.resolve([]),
    ListGroups: () => Promise.resolve([]),
    CreateInstitution: (...args: unknown[]) => createInstitution(...args),
    CreateGroup: vi.fn(),
    ArchiveInstitution: vi.fn(),
    ArchiveGroup: vi.fn(),
    UpdateInstitution: vi.fn(),
    UpdateGroup: vi.fn(),
    SetInstitutionIcon: vi.fn(),
    SetGroupIcon: vi.fn(),
  },
}));
vi.mock("../../../bindings/github.com/waltwang/nestworth-go/internal/wailsapi/catalog", async () => {
  const { TEST_CATALOG } = await import("@/test/catalog");
  return { Service: { Catalog: () => Promise.resolve(TEST_CATALOG) } };
});

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
  setMemberIcon.mockClear();
  createInstitution.mockReset();
  listMembers.mockResolvedValue([{ id: "m1", name: "Alice", iconKey: "user" }]);
  createMember.mockResolvedValue({ id: "m2", name: "Bob", iconKey: "user" });
  createInstitution.mockResolvedValue({ id: "i1", name: "Acme", institutionType: "insurer", iconKey: "shield-plus" });
});

describe("DirectoryPage", () => {
  it("lists existing Members on the default tab", async () => {
    renderPage();
    expect(await screen.findByText("Alice")).toBeInTheDocument();
  });

  it("creates a new Member from the add sheet", async () => {
    renderPage();
    await screen.findByText("Alice");
    await userEvent.click(screen.getByRole("button", { name: "Add a member" }));
    const form = await screen.findByRole("form", { name: "Add a member" });
    const name = within(form).getByLabelText("Member name");
    const icon = within(form).getByLabelText("Choose icon");
    expect(name.compareDocumentPosition(icon) & Node.DOCUMENT_POSITION_FOLLOWING).toBeTruthy();
    expect(name).toHaveAttribute("placeholder", "e.g. Alice");
    await userEvent.type(within(form).getByLabelText("Member name"), "Bob");
    await userEvent.click(within(form).getByRole("button", { name: /add/i }));
    expect(createMember).toHaveBeenCalledWith("Bob", "user");
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
    expect(screen.getByRole("tab", { name: "Institutions" })).toHaveAttribute("aria-selected", "true");
    expect(screen.getByRole("tab", { name: "Members" })).toHaveAttribute("aria-selected", "false");
    expect(screen.getByRole("tabpanel", { name: "Institutions" })).toBeInTheDocument();
  });

  it("requires an institution type and follows it with a default icon", async () => {
    renderPage();
    await screen.findByText("Alice");
    await userEvent.click(screen.getByRole("tab", { name: "Institutions" }));
    await userEvent.click(screen.getByRole("button", { name: "Add an institution" }));
    const form = await screen.findByRole("form", { name: "Add an institution" });
    const name = within(form).getByLabelText("Institution name");
    const type = within(form).getByLabelText("Institution type");
    const icon = within(form).getByLabelText("Choose icon");
    expect(name.compareDocumentPosition(type) & Node.DOCUMENT_POSITION_FOLLOWING).toBeTruthy();
    expect(type.compareDocumentPosition(icon) & Node.DOCUMENT_POSITION_FOLLOWING).toBeTruthy();
    expect(name).toHaveAttribute("placeholder", "e.g. Chase");
    await userEvent.type(within(form).getByLabelText("Institution name"), "Acme");
    await userEvent.selectOptions(within(form).getByLabelText("Institution type"), "insurer");
    await userEvent.click(within(form).getByRole("button", { name: "Add" }));
    expect(createInstitution).toHaveBeenCalledWith("Acme", "insurer", "shield-plus");
  });

  it("uses a group-name hint instead of the add-group title as the placeholder", async () => {
    renderPage();
    await screen.findByText("Alice");
    await userEvent.click(screen.getByRole("tab", { name: "Groups" }));
    await userEvent.click(screen.getByRole("button", { name: "Add a group" }));
    const form = await screen.findByRole("form", { name: "Add a group" });
    expect(within(form).getByLabelText("Group name")).toHaveAttribute("placeholder", "e.g. Retirement");
    expect(within(form).getByLabelText("Group name")).not.toHaveAttribute("placeholder", "Add a group");
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
