import type { LucideIcon } from "lucide-react";
import { cn } from "@/lib/utils";
import { TONE_CLASSES, type Tone } from "@/lib/tone";

const sizeClasses = {
  sm: "size-8 rounded-lg [&_svg]:size-4",
  md: "size-10 rounded-xl [&_svg]:size-5",
  lg: "size-12 rounded-2xl [&_svg]:size-6",
} as const;

/** Rounded colored tile that holds a decorative icon (account type, activity kind, nav item). */
export function IconTile({
  icon: Icon,
  tone,
  size = "md",
  className,
}: {
  icon: LucideIcon;
  tone: Tone;
  size?: keyof typeof sizeClasses;
  className?: string;
}) {
  return (
    <span
      aria-hidden="true"
      className={cn("inline-flex shrink-0 items-center justify-center", TONE_CLASSES[tone].soft, TONE_CLASSES[tone].text, sizeClasses[size], className)}
    >
      <Icon />
    </span>
  );
}
