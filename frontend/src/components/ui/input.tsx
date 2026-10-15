import * as React from "react";
import { useTranslation } from "react-i18next";
import { utf8ByteLength } from "@/lib/utf8";
import { cn } from "@/lib/utils";
import { FieldError } from "./field-error";

export type InputProps = React.InputHTMLAttributes<HTMLInputElement> & {
  maxBytes?: number;
};

const Input = React.forwardRef<HTMLInputElement, InputProps>(({ className, type, maxBytes, ...props }, ref) => {
  const { t } = useTranslation();
  const counterId = React.useId();
  const [draft, setDraft] = React.useState(String(props.defaultValue ?? ""));
  const bytes = utf8ByteLength(String(props.value ?? draft));
  const exceeded = maxBytes !== undefined && bytes > maxBytes;
  const input = (
    <input
      ref={ref}
      type={type}
      className={cn(
        "flex h-9 w-full rounded-xl border border-border bg-card px-3 py-1 text-sm text-foreground shadow-xs transition-[border-color,box-shadow] placeholder:text-muted-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-primary/50 focus-visible:border-primary/50 disabled:cursor-not-allowed disabled:opacity-50",
        className,
      )}
      {...props}
      onChange={maxBytes === undefined ? props.onChange : (event) => {
        setDraft(event.target.value);
        props.onChange?.(event);
      }}
      aria-invalid={exceeded || props["aria-invalid"]}
      aria-describedby={[
        props["aria-describedby"],
        maxBytes !== undefined ? counterId : undefined,
      ].filter(Boolean).join(" ") || undefined}
    />
  );
  if (maxBytes === undefined) return input;
  return (
    <div className="flex min-w-0 flex-col gap-1">
      {input}
      {exceeded ? (
        <FieldError id={counterId}>
          {t("inputBytes.exceeded", { count: bytes, limit: maxBytes })}
        </FieldError>
      ) : (
        <p id={counterId} className="text-xs text-muted-foreground">
          {t("inputBytes.count", { count: bytes, limit: maxBytes })}
        </p>
      )}
    </div>
  );
});
Input.displayName = "Input";

export { Input };
