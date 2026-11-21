import * as SwitchPrimitive from '@radix-ui/react-switch';
import type { ComponentProps } from 'react';
import { cn } from '@/shared/lib/utils';

export function Switch({ className, ...props }: ComponentProps<typeof SwitchPrimitive.Root>) {
  return (
    <SwitchPrimitive.Root
      className={cn(
        'inline-flex h-6 w-11 shrink-0 cursor-pointer items-center rounded-full border-2 border-transparent bg-muted transition-colors data-[state=checked]:bg-primary disabled:opacity-50',
        className,
      )}
      {...props}
    >
      <SwitchPrimitive.Thumb className="block size-5 rounded-full bg-foreground shadow transition-transform data-[state=checked]:translate-x-5" />
    </SwitchPrimitive.Root>
  );
}
