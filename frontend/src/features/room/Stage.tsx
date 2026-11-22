import {
  CarouselLayout,
  FocusLayout,
  GridLayout,
  ParticipantTile,
  type TrackReferenceOrPlaceholder,
  useTracks,
} from '@livekit/components-react';
import { Track } from 'livekit-client';
import type { ReactNode } from 'react';
import { useIsMobile } from '@/shared/lib/useMediaQuery';
import { useCollabStore } from '@/stores/collab';
import { Whiteboard } from '@/features/whiteboard/Whiteboard';

/**
 * Grid of cameras. The whiteboard or a screen share takes focus, with the
 * cameras in a strip: beside it on desktop, below it on phones.
 */
export function Stage() {
  const tracks = useTracks(
    [
      { source: Track.Source.Camera, withPlaceholder: true },
      { source: Track.Source.ScreenShare, withPlaceholder: false },
    ],
    { onlySubscribed: false },
  );
  const board = useCollabStore((s) => s.host.board !== 'off');
  const screen = tracks.find((t) => t.source === Track.Source.ScreenShare);

  if (board)
    return (
      <Focused strip={tracks}>
        <Whiteboard />
      </Focused>
    );
  if (screen) {
    return (
      <Focused strip={tracks.filter((t) => t !== screen)}>
        <FocusLayout trackRef={screen} className="h-full" />
      </Focused>
    );
  }
  return (
    <GridLayout tracks={tracks} className="h-full">
      <ParticipantTile />
    </GridLayout>
  );
}

function Focused({
  strip,
  children,
}: {
  strip: TrackReferenceOrPlaceholder[];
  children: ReactNode;
}) {
  const isMobile = useIsMobile();
  return (
    <div className="flex h-full gap-2 overflow-hidden max-md:flex-col">
      <div className="min-h-0 min-w-0 flex-1">{children}</div>
      {strip.length > 0 && (
        // Fixed-size, clipped box: the carousel sizes its tiles from it.
        <div className="shrink-0 overflow-hidden max-md:h-28 md:w-52">
          <CarouselLayout
            tracks={strip}
            orientation={isMobile ? 'horizontal' : 'vertical'}
            className="lk-carousel size-full"
          >
            <ParticipantTile />
          </CarouselLayout>
        </div>
      )}
    </div>
  );
}
