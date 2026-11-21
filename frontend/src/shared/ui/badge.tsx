import type { ComponentProps } from 'react';
import { cn } from '@/shared/lib/utils';

export function Badge({ className, ...props }: ComponentProps<'span'>) {
  return (
    <span
      className={cn('rounded-full bg-primary/20 px-2 py-0.5 text-xs font-medium text-primary', className)}
      {...props}
    />
  );
}
