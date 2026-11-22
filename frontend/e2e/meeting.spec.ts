import { expect, type Page, test } from '@playwright/test';
import { newPage } from './helpers';

/** Number of <video> elements actually rendering remote frames. */
async function playingRemoteVideos(page: Page): Promise<number> {
  return page.evaluate(
    () =>
      [...document.querySelectorAll<HTMLVideoElement>('.lk-participant-tile video')].filter(
        (v) =>
          v.readyState >= 2 && v.videoWidth > 0 && !v.closest('[data-lk-local-participant="true"]'),
      ).length,
  );
}

test('two participants meet, chat and the host moderates', async ({ browser }) => {
  const host = await newPage(browser);
  await host.goto('/');
  await host.getByLabel('Your name').fill('Hosting Hana');
  await host.getByRole('button', { name: 'New meeting' }).click();
  await expect(host).toHaveURL(/\/m\/[0-9a-z]{4}-[0-9a-z]{4}-[0-9a-z]{4}$/);
  const meetingUrl = host.url();
  await host.getByRole('button', { name: 'Join now' }).click();
  await expect(host.getByRole('button', { name: /leave/i })).toBeVisible();

  const guest = await newPage(browser);
  await guest.goto(meetingUrl);
  await guest.getByLabel('Your name').fill('Guest Gus');
  await guest.getByRole('button', { name: 'Join' }).click();
  await expect(guest.getByRole('button', { name: /leave/i })).toBeVisible();

  // Real media flows both ways through LiveKit.
  await expect.poll(() => playingRemoteVideos(host), { timeout: 30_000 }).toBeGreaterThan(0);
  await expect.poll(() => playingRemoteVideos(guest), { timeout: 30_000 }).toBeGreaterThan(0);

  // Chat goes through the API and arrives via LiveKit data.
  await guest.getByRole('button', { name: 'Chat' }).click();
  await guest.getByLabel('Message').fill('hello from gus');
  await guest.getByRole('button', { name: 'Send' }).click();
  await host.getByRole('button', { name: 'Chat' }).click();
  await expect(host.getByText('hello from gus')).toBeVisible();

  // Guests never see host controls; the host can remove the guest.
  await expect(guest.getByRole('button', { name: 'Host controls' })).toHaveCount(0);
  await host.getByRole('button', { name: 'People' }).click();
  await expect(host.getByRole('heading', { name: 'People (2)' })).toBeVisible();
  host.once('dialog', (d) => void d.accept());
  await host.getByRole('button', { name: 'Remove participant' }).click();
  await expect(guest.getByText('You were removed')).toBeVisible();

  // Ending the meeting disconnects everyone and invalidates the link.
  await host.getByRole('button', { name: 'Host controls' }).click();
  host.once('dialog', (d) => void d.accept());
  await host.getByRole('button', { name: 'End meeting for all' }).click();
  await expect(host.getByText('Meeting ended')).toBeVisible();
  await guest.goto(meetingUrl);
  await expect(guest.getByText('Meeting not found')).toBeVisible();
});

test('waiting room: host admits a guest', async ({ browser }) => {
  const host = await newPage(browser);
  await host.goto('/');
  await host.getByLabel('Your name').fill('Host');
  await host.getByLabel('Waiting room').click();
  await host.getByRole('button', { name: 'New meeting' }).click();
  await host.getByRole('button', { name: 'Join now' }).click();
  await host.getByRole('button', { name: 'Host controls' }).click();

  const guest = await newPage(browser);
  await guest.addInitScript(() => {
    const md = navigator.mediaDevices;
    const original = md.getUserMedia.bind(md);
    (window as unknown as { gumCalls: number }).gumCalls = 0;
    md.getUserMedia = (c) => {
      (window as unknown as { gumCalls: number }).gumCalls++;
      return original(c);
    };
  });
  await guest.goto(host.url());
  await guest.getByLabel('Your name').fill('Waiting Wes');
  await guest.getByRole('button', { name: 'Ask to join' }).click();
  await expect(guest.getByText('Waiting for the host')).toBeVisible();

  // Regression: the device preview must not re-acquire the camera in a loop
  // (which made the waiting room flicker).
  const calls = () => guest.evaluate(() => (window as unknown as { gumCalls: number }).gumCalls);
  const settled = await calls();
  await guest.waitForTimeout(2_000);
  expect(await calls()).toBe(settled);

  await host.getByRole('button', { name: 'Admit Waiting Wes' }).click();
  await expect(guest.getByRole('button', { name: /leave/i })).toBeVisible();
});

test('unknown and malformed meeting links', async ({ page }) => {
  await page.goto('/m/zzzz-zzzz-zzzz');
  await expect(page.getByText('Meeting not found')).toBeVisible();
  await page.goto('/m/ZZZZZZZZZZZZ');
  await expect(page).toHaveURL(/\/m\/zzzz-zzzz-zzzz$/);
});
