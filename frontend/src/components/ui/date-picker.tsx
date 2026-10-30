import { forwardRef, type ComponentProps } from "react";
import { useSettings } from "@/queries/settings";
import { DatePickerControl } from "./date-picker-control";

// Desktop preference adapter; the canonical control stays transport independent.
export const DatePicker = forwardRef<
  HTMLButtonElement,
  ComponentProps<typeof DatePickerControl>
>(function DatePicker(props, ref) {
  const settings = useSettings();
  return (
    <DatePickerControl
      dateFormat={settings.data?.dateFormat}
      weekStart={settings.data?.weekStart}
      {...props}
      ref={ref}
    />
  );
});
