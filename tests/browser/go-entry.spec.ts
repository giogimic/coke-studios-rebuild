import { test, expect } from '@playwright/test';

test.describe('Task 2: Go Client Sole Launch Path', () => {
  test('portal loads and delegates sole gameplay to Go WebAssembly', async ({ page }) => {
    await page.goto('/');

    // 1. Verify engine selector is absent from the player interface
    const engineSelector = page.locator('#engineSelector');
    await expect(engineSelector).toHaveCount(0);

    // 2. Verify game frame loads /game/ path
    const gameFrame = page.locator('#gameFrame');
    await expect(gameFrame).toBeVisible();
    const frameSrc = await gameFrame.getAttribute('src');
    expect(frameSrc).toContain('/game/');
    expect(frameSrc).not.toContain('/phase/');
  });

  test('/game/ serves WebAssembly shell with canvas and honest retry on missing WASM', async ({ page }) => {
    await page.goto('/game/?user=Tester&sw1=VGVzdGVy');

    // 1. Ensure canvas element is present for Ebitengine
    const canvas = page.locator('#canvas');
    await expect(canvas).toBeVisible();

    // 2. Verify page does not contain Phase / Phaser scripts
    const phaseScripts = page.locator('script[src*="phaser"], script[src*="cokephase"]');
    await expect(phaseScripts).toHaveCount(0);

    // 3. Verify wasm_exec.js is loaded
    const wasmScript = page.locator('script[src*="wasm_exec.js"]');
    await expect(wasmScript).toHaveCount(1);
  });
});
