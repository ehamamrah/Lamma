/**
 * Local meeting recording (host browser): the meeting tab's video plus a mix
 * of every participant's audio, encoded with MediaRecorder and downloaded as a
 * file. Nothing is uploaded. Audio is mixed from the LiveKit tracks rather
 * than captured from the tab, so it works whichever surface the host picks.
 */
export interface Recording {
  addAudio: (track: MediaStreamTrack) => void;
  stop: () => void;
  done: Promise<Blob>;
}

const MIME_TYPES = [
  'video/webm;codecs=vp9,opus',
  'video/webm;codecs=vp8,opus',
  'video/webm',
  'video/mp4',
];

/** Screen capture and MediaRecorder are missing on most mobile browsers. */
export function canRecord(): boolean {
  return (
    typeof MediaRecorder !== 'undefined' &&
    typeof navigator !== 'undefined' &&
    typeof navigator.mediaDevices?.getDisplayMedia === 'function'
  );
}

export async function startRecording(): Promise<Recording> {
  const display = await navigator.mediaDevices.getDisplayMedia({
    video: { frameRate: 30 },
    audio: false,
    // Chromium hints: offer the current tab first.
    preferCurrentTab: true,
    selfBrowserSurface: 'include',
  } as DisplayMediaStreamOptions);
  try {
    return record(display);
  } catch (err) {
    display.getTracks().forEach((t) => t.stop()); // don't leave the capture running
    throw err;
  }
}

/** Above this the tab risks running out of memory; stop and save instead. */
const MAX_BYTES = 1_500_000_000;
const VIDEO_BPS = 2_500_000; // about 1.1 GB per hour

function record(display: MediaStream): Recording {
  const audio = new AudioContext();
  const mix = audio.createMediaStreamDestination();
  const added = new Set<string>();
  const addAudio = (track: MediaStreamTrack) => {
    if (added.has(track.id)) return;
    added.add(track.id);
    audio.createMediaStreamSource(new MediaStream([track])).connect(mix);
  };
  const stream = new MediaStream([...display.getVideoTracks(), ...mix.stream.getAudioTracks()]);
  let recorder: MediaRecorder;
  try {
    recorder = new MediaRecorder(stream, {
      mimeType: MIME_TYPES.find((m) => MediaRecorder.isTypeSupported(m)),
      videoBitsPerSecond: VIDEO_BPS,
    });
  } catch (err) {
    void audio.close();
    throw err;
  }
  const stop = () => recorder.state !== 'inactive' && recorder.stop();
  const done = collect(recorder, stop, () => {
    display.getTracks().forEach((t) => t.stop());
    void audio.close();
  });
  // "Stop sharing" in the browser UI ends the recording too.
  display.getVideoTracks()[0]?.addEventListener('ended', stop);
  recorder.start(1_000);
  return { addAudio, stop, done };
}

function collect(recorder: MediaRecorder, stop: () => void, cleanup: () => void): Promise<Blob> {
  const chunks: Blob[] = [];
  let bytes = 0;
  recorder.ondataavailable = (e) => {
    if (e.data.size === 0) return;
    chunks.push(e.data);
    bytes += e.data.size;
    if (bytes > MAX_BYTES) stop();
  };
  return new Promise((resolve) => {
    recorder.onstop = () => {
      cleanup();
      resolve(new Blob(chunks, { type: recorder.mimeType }));
    };
  });
}

export function downloadRecording(blob: Blob, code: string) {
  const ext = blob.type.includes('mp4') ? 'mp4' : 'webm';
  const stamp = new Date().toISOString().slice(0, 16).replace(/[:T]/g, '-');
  const a = document.createElement('a');
  a.href = URL.createObjectURL(blob);
  a.download = `lamma-${code}-${stamp}.${ext}`;
  a.click();
  setTimeout(() => URL.revokeObjectURL(a.href), 60_000);
}
