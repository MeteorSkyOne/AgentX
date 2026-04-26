import * as React from "react";
import { ChevronDown } from "lucide-react";

import { cn } from "@/lib/utils";

interface SelectProps extends React.ComponentProps<"select"> {
  selectClassName?: string;
}

function Select({ className, selectClassName, children, ...props }: SelectProps) {
  return (
    <div className={cn("relative w-full", className)}>
      <select
        className={cn(
          "h-9 w-full appearance-none rounded-md border border-input bg-background px-3 pr-9 text-sm shadow-xs transition-[background-color,border-color,box-shadow] hover:border-ring focus:border-ring focus:ring-[3px] focus:ring-ring/20 focus:outline-none disabled:cursor-not-allowed disabled:opacity-50",
          selectClassName
        )}
        {...props}
      >
        {children}
      </select>
      <ChevronDown className="pointer-events-none absolute right-3 top-1/2 h-4 w-4 -translate-y-1/2 text-muted-foreground" />
    </div>
  );
}

export { Select };
