import * as React from "react";
import { Check } from "lucide-react";

import { cn } from "@/lib/utils";

function Checkbox({
  className,
  ...props
}: Omit<React.ComponentProps<"input">, "type">) {
  return (
    <span className="relative inline-flex h-4 w-4 shrink-0 items-center justify-center">
      <input
        type="checkbox"
        className={cn(
          "peer h-4 w-4 shrink-0 appearance-none rounded-[4px] border border-input bg-background shadow-xs transition-[background-color,border-color,box-shadow] checked:border-primary checked:bg-primary hover:border-ring focus-visible:ring-[3px] focus-visible:ring-ring/30 focus-visible:outline-none disabled:cursor-not-allowed disabled:opacity-50",
          className
        )}
        {...props}
      />
      <Check className="pointer-events-none absolute h-3 w-3 text-primary-foreground opacity-0 transition-opacity peer-checked:opacity-100" />
    </span>
  );
}

export { Checkbox };
