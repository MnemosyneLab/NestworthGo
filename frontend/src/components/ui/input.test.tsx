import { expect, it, vi } from "vitest";
import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { Input } from "./input";
import { utf8ByteLength } from "@/lib/utf8";
import i18n from "@/i18n";

it.each(["a", "中", "😀"])("counts UTF-8 bytes and validates actual pasted %s text", async (text) => {
  await i18n.changeLanguage("en");
  const onChange = vi.fn();
  render(<><label htmlFor="bounded">Source</label><Input id="bounded" maxBytes={12} onChange={onChange} /></>);
  const user = userEvent.setup();
  const input = screen.getByRole("textbox", { name: "Source" });
  await user.click(input);
  const valid = text.repeat(12 / utf8ByteLength(text));
  await user.paste(valid);
  expect(input).toHaveValue(valid);
  expect(input).not.toHaveAttribute("maxlength");
  expect(input).not.toHaveAttribute("aria-invalid", "true");
  expect(input).toHaveAccessibleDescription("12 / 12 bytes");
  await user.paste(text);
  expect(onChange).toHaveBeenCalled();
  expect(input).toHaveAttribute("aria-invalid", "true");
  expect(screen.getByRole("alert")).toHaveTextContent(`${12 + utf8ByteLength(text)} / 12 bytes`);
  expect(input).toHaveAccessibleDescription(/shorten this field/);
  await user.clear(input);
  expect(input).not.toHaveAttribute("aria-invalid", "true");
  expect(input).toHaveAccessibleDescription("0 / 12 bytes");
});
