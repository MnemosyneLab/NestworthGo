import { expect, it } from "vitest";

const sources = import.meta.glob<string>("../**/*.tsx", { query: "?raw", import: "default", eager: true });
const production = Object.entries(sources).filter(([path]) => !/\.test\.tsx$/.test(path));
const importing = (name: string) => production.filter(([, source]) => new RegExp(`import[^;]*\\b${name}\\b[^;]*from`).test(source)).map(([path]) => path).sort();

it("renders every holdings list through the shared grouped list so Portfolio and Accounts cannot drift", () => {
  expect(importing("HoldingGroupsList")).toEqual(["../accounts/AccountDetail.tsx", "./HoldingsTab.tsx"]);
  // Only the shared list may open a holding's detail panel.
  expect(importing("HoldingDetailPanel")).toEqual(["./HoldingGroupsList.tsx"]);
});

it("keeps price performance on the shared panel and gain direction on the shared ChangeText", () => {
  expect(importing("PricePerformancePanel")).toEqual(["./HoldingDetailPanel.tsx"]);
  for (const [path, source] of production) {
    if (!path.startsWith("./Holding") && !path.startsWith("./PricePerformance")) continue;
    // A literal green/red class would be color-only; direction goes through ChangeText.
    expect(source, path).not.toMatch(/text-gain-(positive|negative)/);
  }
});
