import { expect, type Page, test } from '@playwright/test';
import { hostMeeting, joinAsGuest, newPage } from './helpers';

/** Painted pixels on the whiteboard canvas (the canvas itself is transparent). */
function inkOn(page: Page): Promise<number> {
  return page.getByRole('img', { name: 'Shared whiteboard' }).evaluate((c: HTMLCanvasElement) => {
    const data = c.getContext('2d')!.getImageData(0, 0, c.width, c.height).data;
    let n = 0;
    for (let i = 3; i < data.length; i += 4) if (data[i]! > 0) n++;
    return n;
  });
}

async function drawLine(page: Page, from: [number, number], to: [number, number]) {
  const box = (await page.getByRole('img', { name: 'Shared whiteboard' }).boundingBox())!;
  const at = ([x, y]: [number, number]) => [box.x + box.width * x, box.y + box.height * y] as const;
  await page.mouse.move(...at(from));
  await page.mouse.down();
  for (let i = 1; i <= 10; i++) {
    const p: [number, number] = [
      from[0] + ((to[0] - from[0]) * i) / 10,
      from[1] + ((to[1] - from[1]) * i) / 10,
    ];
    await page.mouse.move(...at(p));
  }
  await page.mouse.up();
}

test('reactions and a collaborative whiteboard with approval', async ({ browser }) => {
  const host = await newPage(browser);
  const url = await hostMeeting(host);
  const guest = await newPage(browser);
  await joinAsGuest(guest, url, 'Guest Gus');
  await expect(host.getByRole('button', { name: 'Record' })).toBeVisible();
  await expect(guest.getByRole('button', { name: 'Record' })).toHaveCount(0);

  // Reactions reach the other side.
  await guest.getByRole('button', { name: 'Reactions' }).click();
  await guest.getByRole('menuitem', { name: 'React 🎉' }).click();
  await expect(host.getByRole('img', { name: '🎉' })).toBeVisible();

  // Solo mode: only the host draws; the guest sees the strokes.
  await host.getByRole('button', { name: 'Open whiteboard' }).click();
  await expect(guest.getByText('Only the host can draw')).toBeVisible();
  await drawLine(host, [0.1, 0.1], [0.5, 0.5]);
  await expect.poll(() => inkOn(guest)).toBeGreaterThan(0);

  // Collaborative mode: the guest must ask and be approved.
  await host.getByRole('radio', { name: 'Collaborative' }).click();
  await guest.getByRole('button', { name: 'Request to draw' }).click();
  await host.getByRole('button', { name: 'Allow Guest Gus to draw' }).click();
  await expect(guest.getByText('You can draw')).toBeVisible();
  const before = await inkOn(host);
  await drawLine(guest, [0.6, 0.2], [0.9, 0.8]);
  await expect.poll(() => inkOn(host)).toBeGreaterThan(before);

  // Revoking stops the guest drawing.
  await host.getByRole('button', { name: 'Stop Guest Gus drawing' }).click();
  await expect(guest.getByRole('button', { name: 'Request to draw' })).toBeVisible();
});

test('late joiners receive the current whiteboard', async ({ browser }) => {
  const host = await newPage(browser);
  const url = await hostMeeting(host);
  await host.getByRole('button', { name: 'Open whiteboard' }).click();
  await drawLine(host, [0.2, 0.2], [0.8, 0.6]);

  const guest = await newPage(browser);
  await joinAsGuest(guest, url, 'Late Lou');
  await expect.poll(() => inkOn(guest)).toBeGreaterThan(0);
});

test('phone layout: compact controls with a More sheet', async ({ browser }) => {
  const host = await newPage(browser);
  const url = await hostMeeting(host);
  const phone = await newPage(browser, {
    viewport: { width: 390, height: 844 },
    isMobile: true,
    hasTouch: true,
  });
  await joinAsGuest(phone, url, 'Phone Pat');

  await expect(phone.getByRole('button', { name: 'People' })).toHaveCount(0);
  await phone.getByRole('button', { name: 'More options' }).click();
  await phone
    .getByRole('dialog', { name: 'More options' })
    .getByRole('button', { name: 'People' })
    .click();
  await expect(phone.getByRole('heading', { name: 'People (2)' })).toBeVisible();

  const overflow = await phone.evaluate(
    () => document.documentElement.scrollWidth - window.innerWidth,
  );
  expect(overflow).toBeLessThanOrEqual(0);
});
