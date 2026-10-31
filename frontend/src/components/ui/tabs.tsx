import * as React from "react";
import { Tabs as BaseTabs } from "@base-ui/react/tabs";
import { cn } from "@/lib/utils";

const Tabs = BaseTabs.Root;

/**
 * "pill" is the page-level switcher. "line" is a quieter underline style for
 * tabs nested inside another tab panel, so the two levels never look alike.
 */
type TabsVariant = "pill" | "line";
const TabsVariantContext = React.createContext<TabsVariant>("pill");

function TabsList({ className, variant = "pill", ...props }: React.ComponentProps<typeof BaseTabs.List> & { variant?: TabsVariant }) {
  return (
    <TabsVariantContext.Provider value={variant}>
      <BaseTabs.List
        data-variant={variant}
        className={cn(
          variant === "line"
            ? "flex h-9 items-stretch gap-5 border-b border-border text-muted-foreground"
            : "inline-flex h-9 items-center gap-0.5 rounded-lg bg-muted p-0.5 text-muted-foreground",
          className,
        )}
        {...props}
      />
    </TabsVariantContext.Provider>
  );
}

const TabsTrigger = React.forwardRef<HTMLButtonElement, React.ComponentProps<typeof BaseTabs.Tab>>(function TabsTrigger({ className, ...props }, ref) {
  const variant = React.useContext(TabsVariantContext);
  return (
    <BaseTabs.Tab
      ref={ref}
      className={cn(
        "inline-flex items-center justify-center text-sm font-medium text-muted-foreground hover:text-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-primary/50",
        variant === "line"
          ? "-mb-px rounded-t-md border-b-2 border-transparent px-0.5 transition-[color,border-color] aria-selected:border-primary aria-selected:text-foreground"
          : "rounded-md px-3 py-1 transition-[color,background-color,box-shadow] aria-selected:bg-card aria-selected:text-foreground aria-selected:shadow-xs",
        className,
      )}
      {...props}
    />
  );
});

function TabsContent({ className, ...props }: React.ComponentProps<typeof BaseTabs.Panel>) {
  return <BaseTabs.Panel className={cn("mt-3 rounded-xl focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-primary/50 focus-visible:ring-offset-2 focus-visible:ring-offset-background", className)} {...props} />;
}

export { Tabs, TabsList, TabsTrigger, TabsContent };
