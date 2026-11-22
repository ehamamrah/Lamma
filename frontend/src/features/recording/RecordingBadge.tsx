import { useCollabStore } from '@/stores/collab';

/** Shown to everyone while the host is recording. */
export function RecordingBadge() {
  const recording = useCollabStore((s) => s.host.recording);
  if (!recording) return null;
  return (
    <div
      role="status"
      className="absolute left-3 top-3 z-10 flex items-center gap-2 rounded-full bg-black/70 px-3 py-1 text-xs font-medium text-white"
    >
      <span className="size-2.5 animate-pulse rounded-full bg-red-500" aria-hidden />
      Recording
    </div>
  );
}
