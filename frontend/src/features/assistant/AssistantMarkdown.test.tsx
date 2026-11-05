import { expect, it } from "vitest";
import { render, screen } from "@testing-library/react";
import { AssistantMarkdown } from "./AssistantMarkdown";

it("renders Markdown and unfinished fences without executable HTML or remote resources", () => {
  const { container, rerender } = render(<AssistantMarkdown text={'## Synthetic answer\n\n**USD 12.34**\n\n| Account | Value |\n|---|---|\n| Synthetic | 12.34 |\n\n```json\n{"amount":"12.'} />);
  expect(screen.getByRole("heading", { name: "Synthetic answer" })).toBeVisible();
  expect(container.querySelector("strong")).toHaveTextContent("USD 12.34");
  expect(container.querySelector("table")).toHaveTextContent("Synthetic");
  expect(container.querySelector("pre code")).toHaveTextContent('{"amount":"12.');
  rerender(<AssistantMarkdown text={'<script>alert(1)</script>\n<iframe src="https://attacker.invalid"></iframe>\n\n![Synthetic image](https://attacker.invalid/track)\n\n[x](javascript:alert%281%29) [link](https://example.invalid) <img src=x onerror=alert(1)>'} />);
  expect(container.querySelector("script,iframe,img,a,input,video,audio,object,svg")).toBeNull();
  expect(screen.getByText("Synthetic image")).toBeVisible();
  expect(screen.getByText("link")).toBeVisible();
});


it("preserves footnote references and qualifying text with inert links", () => {
  const { container } = render(<AssistantMarkdown text={'Synthetic return is **12.34%**[^coverage].\n\n[^coverage]: Coverage is incomplete; this is a synthetic estimate. [Source](javascript:alert%281%29) ![No remote load](https://attacker.invalid/track)'} />);
  expect(container.querySelector("sup")).toHaveTextContent("1");
  expect(screen.getByText(/Coverage is incomplete; this is a synthetic estimate/)).toBeVisible();
  expect(container.querySelector("section")).toHaveTextContent("Coverage is incomplete");
  expect(container.querySelector("a,img,script,iframe")).toBeNull();
});
