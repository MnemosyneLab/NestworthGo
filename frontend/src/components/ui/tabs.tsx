import * as React from "react";
import { Tabs as BaseTabs } from "@base-ui/react/tabs";
import { cn } from "@/lib/utils";

const Tabs = BaseTabs.Root;

function TabsList({ className, ...props }: React.ComponentProps<typeof BaseTabs.List>) {
  return (
    <BaseTabs.List
      className={cn("inline-flex h-9 items-center gap-1 rounded-md bg-muted p-1 text-muted-foreground", className)}
      {...props}
    />
  );
}

function TabsTrigger({ className, ...props }: React.ComponentProps<typeof BaseTabs.Tab>) {
  return (
    <BaseTabs.Tab
      className={cn(
        "inline-flex items-center justify-center rounded-sm px-3 py-1 text-sm font-medium transition-colors data-[selected]:bg-card data-[selected]:text-foreground data-[selected]:shadow-sm",
        className,
      )}
      {...props}
    />
  );
}

function TabsContent({ className, ...props }: React.ComponentProps<typeof BaseTabs.Panel>) {
  return <BaseTabs.Panel className={cn("mt-3 focus-visible:outline-none", className)} {...props} />;
}

export { Tabs, TabsList, TabsTrigger, TabsContent };
