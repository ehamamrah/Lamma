import { expect, type Locator, type Page, test } from '@playwright/test';
import { newPage } from './helpers';

/**
 * True when the top-left corner of the video is the painted ocean sky
 * (#89d4f5). The fake camera shows no person, so once segmentation runs the
 * whole frame is background instead of Chrome's green test pattern.
 */
function showsOcean(video: Locator): Promise<boolean> {
  return video.evaluate((v: HTMLVideoElement) => {
    if (!v.videoWidth) return false;
    const c = document.createElement('canvas');
    c.width = v.videoWidth;
    c.height = v.videoHeight;
    const ctx = c.getContext('2d')!;
    ctx.drawImage(v, 0, 0);
    const d = ctx.getImageData(0, 0, 20, 20).data;
    const sum = [0, 0, 0];
    for (let i = 0; i < d.length; i += 4) for (let k = 0; k < 3; k++) sum[k]! += d[i + k]!;
    const [r, g, b] = sum.map((x) => x / 400) as [number, number, number];
    return b > 200 && r < 170 && g > 180;
  });
}

const expectOcean = (video: Locator) =>
  expect.poll(() => showsOcean(video), { timeout: 40_000 }).toBe(true);

const preview = (page: Page) => page.locator('section[aria-label="Device preview"] video');
const myTile = (page: Page) => page.locator('[data-lk-local-participant="true"] video').first();

async function startWithOcean(page: Page) {
  await page.goto('/');
  await page.getByLabel('Your name').fill('Effects Ed');
  await page.getByRole('button', { name: 'New meeting' }).click();
  await page.getByRole('button', { name: 'Effects' }).click();
  await page.getByRole('button', { name: 'Ocean' }).click();
  await expect(page.getByText('Loading effects…')).toHaveCount(0, { timeout: 60_000 });
  await expect(page.getByText('could not load')).toHaveCount(0);
  await expectOcean(preview(page));
}

test('virtual background and mask, kept across camera off and on', async ({ browser }) => {
  const page = await newPage(browser);
  const errors: string[] = [];
  page.on('pageerror', (e) => errors.push(e.message));
  page.on('console', (m) => {
    if (m.type() === 'error' && /Content Security|Refused/.test(m.text())) errors.push(m.text());
  });
  await startWithOcean(page);
  await page.getByRole('button', { name: 'Crown' }).click(); // loads the face model too
  await expect(page.getByText('Loading effects…')).toHaveCount(0, { timeout: 40_000 });
  await expect(page.getByText('could not load')).toHaveCount(0);

  // The meeting starts with the effect applied, and the choice is remembered.
  await page.getByRole('button', { name: 'Join now' }).click();
  await expectOcean(myTile(page));
  await page.getByRole('button', { name: 'Effects' }).click();
  await expect(page.getByRole('button', { name: 'Ocean' })).toHaveAttribute('aria-pressed', 'true');
  await expect(page.getByRole('button', { name: 'Crown' })).toHaveAttribute('aria-pressed', 'true');

  // Turning the camera off and on keeps it.
  const camera = page.getByRole('button', { name: 'Camera', exact: true });
  await camera.click();
  await expect(camera).toHaveAttribute('aria-pressed', 'false');
  await camera.click();
  await expect(camera).toHaveAttribute('aria-pressed', 'true');
  await expectOcean(myTile(page));
  expect(errors).toEqual([]);
});

test('a camera first turned on mid-meeting is processed', async ({ browser }) => {
  const page = await newPage(browser);
  await startWithOcean(page);
  await page.getByRole('button', { name: 'Turn camera off' }).click();
  await page.getByRole('button', { name: 'Join now' }).click();
  await page.getByRole('button', { name: 'Camera', exact: true }).click();
  await expectOcean(myTile(page));
});
