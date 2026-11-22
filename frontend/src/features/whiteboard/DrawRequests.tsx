import { useParticipants } from '@livekit/components-react';
import { Check, Hand, X } from 'lucide-react';
import { Button } from '@/shared/ui/button';
import { boardHost, useCollabStore } from '@/stores/collab';

/** Host: approve or deny draw requests and revoke drawing rights. */
export function DrawRequests() {
  const { requests, drawers, board } = useCollabStore((s) => s.host);
  const participants = useParticipants();
  const nameOf = (id: string) => participants.find((p) => p.identity === id)?.name || 'Guest';
  if (board !== 'collab' || (requests.length === 0 && drawers.length === 0)) return null;

  return (
    <div className="flex flex-wrap items-center gap-2 text-sm" aria-label="Drawing permissions">
      {requests.map((id) => (
        <span
          key={id}
          className="flex items-center gap-1 rounded-full bg-primary/15 py-0.5 pl-2.5 pr-0.5"
        >
          <Hand className="size-3.5" aria-hidden />
          <span className="max-w-32 truncate">{nameOf(id)}</span>
          <Button
            size="sm"
            variant="ghost"
            className="h-7 px-2"
            aria-label={`Allow ${nameOf(id)} to draw`}
            onClick={() => boardHost.approve(id)}
          >
            <Check />
          </Button>
          <Button
            size="sm"
            variant="ghost"
            className="h-7 px-2"
            aria-label={`Deny ${nameOf(id)}`}
            onClick={() => boardHost.deny(id)}
          >
            <X />
          </Button>
        </span>
      ))}
      {drawers.map((id) => (
        <span
          key={id}
          className="flex items-center gap-1 rounded-full bg-muted py-0.5 pl-2.5 pr-0.5"
        >
          <span className="max-w-32 truncate">{nameOf(id)} can draw</span>
          <Button
            size="sm"
            variant="ghost"
            className="h-7 px-2"
            aria-label={`Stop ${nameOf(id)} drawing`}
            onClick={() => boardHost.revoke(id)}
          >
            <X />
          </Button>
        </span>
      ))}
    </div>
  );
}
