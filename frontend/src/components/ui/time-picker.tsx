import { useState } from "react";
import { Clock } from "lucide-react";
import { useTranslation } from "react-i18next";
import { Button, buttonVariants } from "@/components/ui/button";
import { Popover, PopoverContent, PopoverTrigger } from "@/components/ui/popover";
import { cn } from "@/lib/utils";

const HOURS = Array.from({ length: 24 }, (_, index) => String(index).padStart(2, "0"));
const MINUTES = Array.from({ length: 60 }, (_, index) => String(index).padStart(2, "0"));

function splitTime(value: string): { hour: string; minute: string } {
  const match = /^(\d{2}):(\d{2})$/.exec(value.trim());
  return { hour: match?.[1] ?? "", minute: match?.[2] ?? "" };
}

export function TimePicker({
  id,
  value,
  onChange,
  disabled = false,
}: {
  id?: string;
  value: string;
  onChange: (value: string) => void;
  disabled?: boolean;
}) {
  const { t } = useTranslation();
  const [open, setOpen] = useState(false);
  const { hour, minute } = splitTime(value);

  const pick = (nextHour: string, nextMinute: string) => {
    if (!nextHour || !nextMinute) {
      return;
    }
    onChange(`${nextHour}:${nextMinute}`);
  };

  return (
    <Popover open={open} onOpenChange={setOpen}>
      <PopoverTrigger
        id={id}
        type="button"
        disabled={disabled}
        className={cn(buttonVariants({ variant: "outline" }), "w-full justify-start font-normal", !value && "text-muted-foreground")}
      >
        <Clock className="size-4" aria-hidden="true" />
        {value || t("history.selectEmpty")}
      </PopoverTrigger>
      <PopoverContent className="w-auto p-2">
        <div className="flex gap-2">
          <ul className="flex max-h-56 flex-col gap-0.5 overflow-y-auto" aria-label={t("history.effectiveTime")}>
            {HOURS.map((option) => (
              <li key={option}>
                <Button
                  type="button"
                  variant={option === hour ? "default" : "ghost"}
                  size="sm"
                  className="w-12"
                  onClick={() => pick(option, minute || "00")}
                >
                  {option}
                </Button>
              </li>
            ))}
          </ul>
          <ul className="flex max-h-56 flex-col gap-0.5 overflow-y-auto" aria-label={t("history.effectiveTime")}>
            {MINUTES.map((option) => (
              <li key={option}>
                <Button
                  type="button"
                  variant={option === minute ? "default" : "ghost"}
                  size="sm"
                  className="w-12"
                  onClick={() => pick(hour || "00", option)}
                >
                  {option}
                </Button>
              </li>
            ))}
          </ul>
        </div>
      </PopoverContent>
    </Popover>
  );
}
