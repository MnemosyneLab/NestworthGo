import * as React from "react";
import { DayPicker } from "react-day-picker";
import { ChevronDown, ChevronLeft, ChevronRight } from "lucide-react";
import { cn } from "@/lib/utils";
import { buttonVariants } from "@/components/ui/button";

export type CalendarProps = React.ComponentProps<typeof DayPicker>;

function Calendar({
  className,
  classNames,
  showOutsideDays = true,
  captionLayout = "label",
  ...props
}: CalendarProps) {
  return (
    <DayPicker
      showOutsideDays={showOutsideDays}
      captionLayout={captionLayout}
      className={cn("p-2", className)}
      classNames={{
        months: "relative flex flex-col gap-4",
        month: "flex flex-col gap-4",
        month_caption: "relative flex h-8 items-center justify-center",
        caption_label: "text-sm font-medium",
        dropdowns: "flex items-center justify-center gap-2 text-sm",
        dropdown: "rounded-md border border-border bg-card px-2 py-1 text-sm",
        nav: "absolute inset-x-0 top-0 flex items-center justify-between px-1",
        button_previous: cn(buttonVariants({ variant: "ghost", size: "icon" }), "size-8"),
        button_next: cn(buttonVariants({ variant: "ghost", size: "icon" }), "size-8"),
        month_grid: "w-full border-collapse",
        weekdays: "flex",
        weekday: "w-8 text-[0.8rem] font-normal text-muted-foreground",
        week: "mt-1 flex w-full",
        day: "group size-8 p-0 text-center text-sm",
        day_button: cn(
          buttonVariants({ variant: "ghost" }),
          "size-8 p-0 font-normal aria-selected:opacity-100",
        ),
        selected: "rounded-md bg-primary text-primary-foreground hover:bg-primary hover:text-primary-foreground",
        today: "rounded-md bg-muted text-foreground",
        outside: "text-muted-foreground opacity-50",
        disabled: "text-muted-foreground opacity-50",
        hidden: "invisible",
        ...classNames,
      }}
      components={{
        Chevron: ({ orientation, className: chevronClass, ...chevronProps }) => {
          const Icon = orientation === "left" ? ChevronLeft : orientation === "right" ? ChevronRight : ChevronDown;
          return <Icon className={cn("size-4", chevronClass)} {...chevronProps} />;
        },
      }}
      {...props}
    />
  );
}

export { Calendar };
