import { Loader2 } from 'lucide-react';
import { useCallback } from 'react';
import type { Status } from '@/shared/api/types';
import { Card, CardDescription, CardTitle } from '@/shared/ui/card';
import { applyAdmission } from '@/features/meeting/session';
import { DevicePreview } from '@/features/prejoin/DevicePreview';
import { useAdmission } from './useAdmission';

export function WaitingRoom({ code }: { code: string }) {
  const onDecision = useCallback((status: Status | 'gone') => applyAdmission(code, status), [code]);
  useAdmission(code, onDecision);

  return (
    <main className="mx-auto grid min-h-full max-w-5xl items-center gap-6 p-6 md:grid-cols-[3fr_2fr]">
      <DevicePreview />
      <Card className="flex flex-col items-center gap-3 text-center" aria-live="polite">
        <Loader2 className="size-8 animate-spin text-primary" aria-hidden />
        <CardTitle>Waiting for the host</CardTitle>
        <CardDescription>
          You'll join automatically as soon as the host lets you in.
        </CardDescription>
      </Card>
    </main>
  );
}
