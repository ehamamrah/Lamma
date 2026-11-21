import type { ComponentProps } from 'react';
import { cn } from '@/shared/lib/utils';

export function Input({ className, ...props }: ComponentProps<'input'>) {
  return (
    <input
      className={cn(
        'h-10 w-full rounded-lg border border-border bg-background px-3 text-sm placeholder:text-muted-foreground disabled:opacity-50',
        className,
      )}
      {...props}
    />
  );
}
