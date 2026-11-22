import { useEffect, useRef, useState } from 'react';
import { cn } from '@/shared/lib/utils';
import { useCollabStore } from '@/stores/collab';
import { drawStrokes, fitBoard } from './draw';
import { useBoardDrawing } from './useBoardDrawing';

interface Props {
  canDraw: boolean;
  color: string;
  width: number;
}

/**
 * A 16:9 white canvas that fills the available space at device resolution.
 * Strokes are drawn outside React: store changes schedule at most one redraw
 * per animation frame, and the backing store is resized only on resize.
 */
export function BoardCanvas({ canDraw, color, width }: Props) {
  const box = useRef<HTMLDivElement>(null);
  const canvas = useRef<HTMLCanvasElement>(null);
  const [size, setSize] = useState({ w: 0, h: 0 });
  const handlers = useBoardDrawing(canDraw, { color, width });

  useEffect(() => {
    const el = box.current;
    if (!el) return;
    const ro = new ResizeObserver(([entry]) => {
      if (entry) setSize(fitBoard(entry.contentRect.width, entry.contentRect.height));
    });
    ro.observe(el);
    return () => ro.disconnect();
  }, []);

  useEffect(() => {
    const el = canvas.current;
    const ctx = el?.getContext('2d');
    if (!el || !ctx || size.w === 0) return;
    const dpr = window.devicePixelRatio || 1;
    el.width = size.w * dpr;
    el.height = size.h * dpr;
    ctx.setTransform(dpr, 0, 0, dpr, 0, 0);
    return redrawOnChange(ctx, size.w, size.h);
  }, [size]);

  return (
    <div ref={box} className="flex min-h-0 flex-1 items-center justify-center">
      <canvas
        ref={canvas}
        role="img"
        aria-label="Shared whiteboard"
        style={{ width: size.w, height: size.h }}
        className={cn('touch-none rounded-lg bg-white shadow', canDraw && 'cursor-crosshair')}
        {...handlers}
      />
    </div>
  );
}

/** Draws now, then once per frame after any stroke change. */
function redrawOnChange(ctx: CanvasRenderingContext2D, w: number, h: number) {
  let frame = 0;
  const draw = () => {
    frame = 0;
    drawStrokes(ctx, useCollabStore.getState().strokes, w, h);
  };
  draw();
  const unsubscribe = useCollabStore.subscribe((s, prev) => {
    if (s.strokes !== prev.strokes && !frame) frame = requestAnimationFrame(draw);
  });
  return () => {
    unsubscribe();
    cancelAnimationFrame(frame);
  };
}
