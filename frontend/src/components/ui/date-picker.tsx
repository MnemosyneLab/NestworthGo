import { useMemo, useState } from "react";
import { format, isValid } from "date-fns";
import { enUS, zhCN, zhTW, type Locale } from "date-fns/locale";
import { CalendarIcon } from "lucide-react";
import { useTranslation } from "react-i18next";
import { buttonVariants } from "@/components/ui/button";
import { Calendar } from "@/components/ui/calendar";
import { Popover, PopoverContent, PopoverTrigger } from "@/components/ui/popover";
import { useSettings } from "@/queries/settings";
import { cn } from "@/lib/utils";

function pickerLocale(language: string): Locale {
  if (language === "zh-TW") {
    return zhTW;
  }
  if (language.startsWith("zh")) {
    return zhCN;
  }
  return enUS;
}

function parseYmd(value: string | undefined): Date | undefined {
  if (!value) {
    return undefined;
  }
  const match = /^(\d{4})-(\d{2})-(\d{2})$/.exec(value.trim());
  if (!match) {
    return undefined;
  }
  const date = new Date(Number(match[1]), Number(match[2]) - 1, Number(match[3]));
  return isValid(date) ? date : undefined;
}

function formatYmd(date: Date): string {
  return format(date, "yyyy-MM-dd");
}

function displayDate(date: Date, dateFormat: string | undefined, language: string): string {
  switch (dateFormat) {
    case "day-first":
      return format(date, "dd/MM/yyyy");
    case "month-first":
      return format(date, "MM/dd/yyyy");
    case "localized":
      return new Intl.DateTimeFormat(language, { dateStyle: "medium" }).format(date);
    default:
      return format(date, "yyyy-MM-dd");
  }
}

export function DatePicker({
  id,
  value,
  onChange,
  min,
  max,
  disabled = false,
  placeholder,
}: {
  id?: string;
  value: string;
  onChange: (value: string) => void;
  min?: string;
  max?: string;
  disabled?: boolean;
  placeholder?: string;
}) {
  const { t, i18n } = useTranslation();
  const settings = useSettings();
  const [open, setOpen] = useState(false);
  const selected = parseYmd(value);
  const weekStartsOn = settings.data?.weekStart === "sunday" ? (0 as const) : (1 as const);
  const startMonth = useMemo(
    () => parseYmd(min) ?? new Date(new Date().getFullYear() - 50, 0),
    [min],
  );
  const endMonth = useMemo(() => parseYmd(max) ?? new Date(), [max]);
  const disabledMatcher = useMemo(() => {
    const before = min ? parseYmd(min) : undefined;
    const after = max ? parseYmd(max) : undefined;
    if (before && after) {
      return { before, after };
    }
    if (before) {
      return { before };
    }
    if (after) {
      return { after };
    }
    return undefined;
  }, [max, min]);

  return (
    <Popover open={open} onOpenChange={setOpen}>
      <PopoverTrigger
        id={id}
        type="button"
        disabled={disabled}
        className={cn(
          buttonVariants({ variant: "outline" }),
          "w-full justify-start font-normal",
          !selected && "text-muted-foreground",
        )}
      >
        <CalendarIcon className="size-4" aria-hidden="true" />
        {selected ? displayDate(selected, settings.data?.dateFormat, i18n.language) : (placeholder ?? t("history.selectEmpty"))}
      </PopoverTrigger>
      <PopoverContent className="w-auto p-0" align="start">
        <Calendar
          mode="single"
          captionLayout="dropdown"
          locale={pickerLocale(i18n.language)}
          formatters={{
            formatMonthDropdown: (date) => date.toLocaleString(i18n.language, { month: "short" }),
          }}
          weekStartsOn={weekStartsOn}
          selected={selected}
          defaultMonth={selected}
          onSelect={(date) => {
            if (!date) {
              return;
            }
            onChange(formatYmd(date));
            setOpen(false);
          }}
          disabled={disabledMatcher}
          startMonth={startMonth}
          endMonth={endMonth}
        />
      </PopoverContent>
    </Popover>
  );
}
