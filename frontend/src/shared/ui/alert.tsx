import type { ComponentProps } from 'react';
import { cn } from '@/shared/lib/utils';

export function Alert({ className, ...props }: ComponentProps<'div'>) {
  return (
    <div
      role="alert"
      className={cn(
        'rounded-lg border border-destructive/50 bg-destructive/10 px-3 py-2 text-sm',
        className,
      )}
      {...props}
    />
  );
}
