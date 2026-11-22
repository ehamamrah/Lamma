import { type Browser, type BrowserContextOptions, type Page, expect } from '@playwright/test';

export async function newPage(browser: Browser, opts: BrowserContextOptions = {}): Promise<Page> {
  const ctx = await browser.newContext({
    permissions: ['camera', 'microphone', 'clipboard-read', 'clipboard-write'],
    ...opts,
  });
  return ctx.newPage();
}

/** Host creates a meeting and enters it; returns the meeting URL. */
export async function hostMeeting(host: Page, name = 'Hosting Hana'): Promise<string> {
  await host.goto('/');
  await host.getByLabel('Your name').fill(name);
  await host.getByRole('button', { name: 'New meeting' }).click();
  await expect(host).toHaveURL(/\/m\/[0-9a-z]{4}-[0-9a-z]{4}-[0-9a-z]{4}$/);
  await host.getByRole('button', { name: 'Join now' }).click();
  await expect(host.getByRole('button', { name: /leave/i })).toBeVisible();
  return host.url();
}

export async function joinAsGuest(guest: Page, url: string, name: string) {
  await guest.goto(url);
  await guest.getByLabel('Your name').fill(name);
  await guest.getByRole('button', { name: 'Join' }).click();
  await expect(guest.getByRole('button', { name: /leave/i })).toBeVisible();
}
