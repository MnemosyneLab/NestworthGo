import * as React from "react";
import { DateLib, DayPicker, type DropdownProps, type Labels } from "react-day-picker";
import { ChevronDown, ChevronLeft, ChevronRight } from "lucide-react";
import { useTranslation } from "react-i18next";
import { cn } from "@/lib/utils";
import { buttonVariants } from "@/components/ui/button";
import { NativeSelect } from "@/components/ui/select";

export type CalendarProps = React.ComponentProps<typeof DayPicker>;

function CalendarDropdown({ options, components: _components, classNames: _classNames, className, ...props }: DropdownProps) {
  return (
    <NativeSelect {...props} className={cn("h-8 min-h-8 px-2 font-medium", className)}>
      {options?.map(({ value, label, disabled }) => <option key={value} value={value} disabled={disabled}>{label}</option>)}
    </NativeSelect>
  );
}

function Calendar({
  className,
  classNames,
  showOutsideDays = true,
  captionLayout = "label",
  formatters,
  components,
  labels,
  ...props
}: CalendarProps) {
  const { t, i18n } = useTranslation();
  const labelDayButton: Labels["labelDayButton"] = (date, modifiers, options, dateLib) => {
    let label = (dateLib ?? new DateLib(options)).format(date, "PPPP");
    if (modifiers.today) label = t("datePicker.today", { date: label });
    if (modifiers.selected) label = t("datePicker.selected", { date: label });
    return label;
  };

  return (
    <DayPicker
      showOutsideDays={showOutsideDays}
      captionLayout={captionLayout}
      className={cn("p-2 [--cell-size:2rem]", className)}
      formatters={{
        formatMonthDropdown: (date) => date.toLocaleString(i18n.language || undefined, { month: "short" }),
        ...formatters,
      }}
      labels={{
        labelPrevious: () => t("datePicker.previousMonth"),
        labelNext: () => t("datePicker.nextMonth"),
        labelMonthDropdown: () => t("datePicker.chooseMonth"),
        labelYearDropdown: () => t("datePicker.chooseYear"),
        labelDayButton,
        labelGridcell: (date, modifiers, options, dateLib) => labelDayButton(date, modifiers ?? {}, options, dateLib),
        labelWeekNumberHeader: () => t("datePicker.weekNumber"),
        labelWeekNumber: (week) => t("datePicker.week", { week }),
        ...labels,
      }}
      classNames={{
        months: "relative flex flex-col gap-4",
        month: "flex w-full flex-col gap-4",
        month_caption: "relative flex h-8 w-full items-center justify-center px-8",
        caption_label: "select-none text-sm font-medium",
        dropdowns: "flex h-8 w-full items-center justify-center gap-1.5 text-sm font-medium",
        nav: "pointer-events-none absolute inset-x-0 top-0 z-20 flex items-center justify-between",
        button_previous: cn(buttonVariants({ variant: "ghost", size: "icon" }), "pointer-events-auto size-8"),
        button_next: cn(buttonVariants({ variant: "ghost", size: "icon" }), "pointer-events-auto size-8"),
        month_grid: "w-full border-collapse",
        weekdays: "flex",
        weekday: "w-8 text-[0.8rem] font-normal text-muted-foreground",
        week: "mt-1 flex w-full",
        day: "group size-8 p-0 text-center text-sm",
        day_button: cn(
          buttonVariants({ variant: "ghost" }),
          "size-8 rounded-full p-0 font-normal aria-selected:opacity-100",
        ),
        selected: "rounded-full bg-primary text-primary-foreground hover:bg-primary hover:text-primary-foreground",
        today: "rounded-full bg-muted text-foreground",
        outside: "text-muted-foreground opacity-50",
        disabled: "text-muted-foreground opacity-50",
        hidden: "invisible",
        ...classNames,
      }}
      components={{
        Dropdown: CalendarDropdown,
        Chevron: ({ orientation, className: chevronClass, ...chevronProps }) => {
          const Icon = orientation === "left" ? ChevronLeft : orientation === "right" ? ChevronRight : ChevronDown;
          return <Icon className={cn("size-4", chevronClass)} {...chevronProps} />;
        },
        ...components,
      }}
      {...props}
    />
  );
}

export { Calendar };
