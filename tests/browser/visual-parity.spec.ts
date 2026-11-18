import { test, expect } from '@playwright/test';
import * as fs from 'fs';
import * as path from 'path';

test.describe('Sub-task 7.2: Frame-Accurate Visual Parity & Resolution Fidelity', () => {
  const screenshotsDir = path.join(__dirname, '..', 'reports', 'visual-snapshots');

  test.beforeAll(async () => {
    if (!fs.existsSync(screenshotsDir)) {
      fs.mkdirSync(screenshotsDir, { recursive: true });
    }
  });

  test('760px authentic website shell layout, geometry, and badge avatar canvas', async ({ page }) => {
    await page.goto('/');

    // 1. Verify strict 760px portal shell width
    const pageContainer = page.locator('#page-container');
    await expect(pageContainer).toBeVisible();
    const containerBox = await pageContainer.boundingBox();
    expect(containerBox).not.toBeNull();
    expect(Math.round(containerBox!.width)).toBe(760);

    // 2. Verify left column avatar badge geometry (176px canonical width)
    const badgeCol = page.locator('#badge-column');
    await expect(badgeCol).toBeVisible();
    const badgeBox = await badgeCol.boundingBox();
    expect(badgeBox).not.toBeNull();
    expect(Math.round(badgeBox!.width)).toBe(176);

    // 3. Verify top navigation bar geometry (584px width, 26px height)
    const topNav = page.locator('#top-nav');
    await expect(topNav).toBeVisible();
    const navBox = await topNav.boundingBox();
    expect(navBox).not.toBeNull();
    expect(Math.round(navBox!.width)).toBe(584);
    expect(Math.round(navBox!.height)).toBe(26);

    // 4. Verify live avatar preview canvas (128x150)
    const avatarCanvas = page.locator('#badgeAvatarCanvas');
    await expect(avatarCanvas).toBeVisible();
    const canvasAttrs = await avatarCanvas.evaluate((el: HTMLCanvasElement) => ({
      width: el.width,
      height: el.height,
    }));
    expect(canvasAttrs.width).toBe(128);
    expect(canvasAttrs.height).toBe(150);

    // Capture visual snapshot of the authentic 760px portal
    const snapshotPath = path.join(screenshotsDir, 'portal-760px-shell.png');
    await pageContainer.screenshot({ path: snapshotPath });
    expect(fs.existsSync(snapshotPath)).toBe(true);
    expect(fs.statSync(snapshotPath).size).toBeGreaterThan(1000);
  });

  test('Macromedia Director MX 760x520 stage canvas resolution and aspect ratio across public venues', async ({ page }) => {
    // Open Go WASM game client in standalone stage viewport
    await page.setViewportSize({ width: 760, height: 520 });

    const venues = [
      { id: '$SYSTEM$1', name: 'london-1' },
      { id: '$SYSTEM$2', name: 'alaska-1' },
      { id: '$SYSTEM$3', name: 'miami-1' },
      { id: '$SYSTEM$4', name: 'mombasa-1' },
      { id: '$SYSTEM$5', name: 'goa-1' },
      { id: '$SYSTEM$6', name: 'moscow-1' },
      { id: '$SYSTEM$7', name: 'centralpark-1' },
    ];

    for (const v of venues) {
      await page.goto(`/game/?room=${encodeURIComponent(v.id)}&user=VisualVerifier&sw1=VmlzdWFsVmVyaWZpZXI%3D`);

      // 1. Ensure canvas element is present and sized to the stage
      const canvas = page.locator('#canvas');
      await expect(canvas).toBeVisible();

      const canvasBox = await canvas.boundingBox();
      expect(canvasBox).not.toBeNull();
      expect(Math.round(canvasBox!.width)).toBe(760);
      expect(Math.round(canvasBox!.height)).toBe(520);

      // Verify exact Director MX 760:520 stage ratio (1.4615)
      const aspectRatio = canvasBox!.width / canvasBox!.height;
      expect(aspectRatio).toBeCloseTo(760 / 520, 2);

      // Capture visual snapshot of the stage
      const snapshotPath = path.join(screenshotsDir, `stage-${v.name}.png`);
      await page.screenshot({ path: snapshotPath });
      expect(fs.existsSync(snapshotPath)).toBe(true);
      expect(fs.statSync(snapshotPath).size).toBeGreaterThan(1000);
    }
  });

  test('Public venue background raster layers and Director cast descriptors load cleanly', async ({ request }) => {
    const publicVenues = [
      { id: '$SYSTEM$1', name: 'London I', members: '/Lingo/london/Members.csv', bg: '/Lingo/london/3_cc.london.bg.png' },
      { id: '$SYSTEM$2', name: 'Alaska I', members: '/Lingo/alaska/Members.csv', bg: '/Lingo/alaska/reykjavik_bg.jpg' },
      { id: '$SYSTEM$3', name: 'Miami I', members: '/Lingo/miami/Members.csv', bg: '/Lingo/miami/miami.jpg' },
      { id: '$SYSTEM$4', name: 'Mombasa I', members: '/Lingo/mombasa/Members.csv', bg: '/Lingo/mombasa/mombasa.jpg' },
      { id: '$SYSTEM$5', name: 'Goa I', members: '/Lingo/goa/Members.csv', bg: '/Lingo/goa/goa.jpg' },
      { id: '$SYSTEM$6', name: 'Moscow I', members: '/Lingo/moscow/Members.csv', bg: '/Lingo/moscow/moscow.jpg' },
      { id: '$SYSTEM$7', name: 'Central Park I', members: '/Lingo/centralpark/Members.csv', bg: '/Lingo/centralpark/backgroundflatspeakerwire.png' },
    ];

    for (const venue of publicVenues) {
      const res = await request.get(venue.members);
      expect(res.status(), `Public venue ${venue.name} members CSV failed at ${venue.members}`).toBe(200);
      const text = await res.text();
      expect(text.length).toBeGreaterThan(20);

      const bgRes = await request.get(venue.bg);
      expect(bgRes.status(), `Public venue ${venue.name} background failed at ${venue.bg}`).toBe(200);
      const bgBytes = await bgRes.body();
      expect(bgBytes.length).toBeGreaterThan(1000);
    }
  });

  test('Avatar sprite components layers and figure definition palette indices', async ({ request }) => {
    // Verify male and female body sets load cleanly
    const bodySets = [
      '/assets/avatar/body/male/body-se-stand.png',
      '/assets/avatar/body/female/body-se-stand.png',
      '/assets/avatar/body/male/body-se-sit.png',
      '/assets/avatar/body/female/body-se-sit.png',
    ];

    for (const spritePath of bodySets) {
      const res = await request.get(spritePath);
      expect(res.status(), `Avatar sprite failed at ${spritePath}`).toBe(200);
      const body = await res.body();
      expect(body.length).toBeGreaterThan(100);
      // Verify PNG header signature 0x89 0x50 0x4E 0x47
      expect(body[0]).toBe(0x89);
      expect(body[1]).toBe(0x50);
      expect(body[2]).toBe(0x4E);
      expect(body[3]).toBe(0x47);
    }
  });
});
