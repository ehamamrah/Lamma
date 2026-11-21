import { X } from 'lucide-react';
import { Button } from '@/shared/ui/button';

export function PanelHeader({ title, onClose }: { title: string; onClose: () => void }) {
  return (
    <header className="flex items-center justify-between border-b border-border px-4 py-3">
      <h2 className="font-semibold">{title}</h2>
      <Button variant="ghost" size="icon" aria-label={`Close ${title}`} onClick={onClose}>
        <X />
      </Button>
    </header>
  );
}
