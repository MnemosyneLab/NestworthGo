import { render, screen } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "./tabs";

function renderTabs(variant?: "pill" | "line") {
  render(
    <Tabs defaultValue="a">
      <TabsList variant={variant} aria-label="Views">
        <TabsTrigger value="a">A</TabsTrigger>
        <TabsTrigger value="b">B</TabsTrigger>
      </TabsList>
      <TabsContent value="a">Panel A</TabsContent>
      <TabsContent value="b">Panel B</TabsContent>
    </Tabs>,
  );
}

describe("Tabs", () => {
  it("defaults to the page-level pill style", () => {
    renderTabs();
    expect(screen.getByRole("tablist", { name: "Views" })).toHaveAttribute("data-variant", "pill");
    expect(screen.getByRole("tab", { name: "A" }).className).toContain("aria-selected:bg-card");
  });

  it("gives nested tabs a distinct underline style", () => {
    renderTabs("line");
    expect(screen.getByRole("tablist", { name: "Views" })).toHaveAttribute("data-variant", "line");
    const trigger = screen.getByRole("tab", { name: "A" });
    expect(trigger.className).toContain("aria-selected:border-primary");
    expect(trigger.className).not.toContain("aria-selected:bg-card");
    expect(trigger).toHaveAttribute("aria-selected", "true");
  });
});
