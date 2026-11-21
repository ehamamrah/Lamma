import {
  CarouselLayout,
  FocusLayout,
  FocusLayoutContainer,
  GridLayout,
  ParticipantTile,
  useTracks,
} from '@livekit/components-react';
import { Track } from 'livekit-client';

/** Grid of cameras; a screen share takes focus with cameras in a carousel. */
export function Stage() {
  const tracks = useTracks(
    [
      { source: Track.Source.Camera, withPlaceholder: true },
      { source: Track.Source.ScreenShare, withPlaceholder: false },
    ],
    { onlySubscribed: false },
  );
  const screen = tracks.find((t) => t.source === Track.Source.ScreenShare);

  if (!screen) {
    return (
      <GridLayout tracks={tracks} className="h-full">
        <ParticipantTile />
      </GridLayout>
    );
  }
  const others = tracks.filter((t) => t !== screen);
  return (
    <FocusLayoutContainer className="h-full">
      <CarouselLayout tracks={others}>
        <ParticipantTile />
      </CarouselLayout>
      <FocusLayout trackRef={screen} />
    </FocusLayoutContainer>
  );
}
