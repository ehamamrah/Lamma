// Frame clock for camera effects. Timers in a worker keep running when the
// meeting tab is in the background, unlike requestAnimationFrame, so others
// keep seeing our video while we look at another tab.
let timer: ReturnType<typeof setInterval> | undefined;

self.onmessage = (e: MessageEvent<number>) => {
  clearInterval(timer);
  if (e.data > 0) timer = setInterval(() => self.postMessage(0), e.data);
};
