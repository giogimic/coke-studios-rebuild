import { test, expect } from '@playwright/test';

test.describe('Task 3: Real Accounts, Sessions, and Per-Player Identity', () => {
  const timestamp = Date.now();
  const aliceName = `Alice_${timestamp}`;
  const bobName = `Bob_${timestamp}`;
  const password = 'SecretPassword123!';

  test('register new denizen issues authoritative session and launches Go world', async ({ page }) => {
    await page.goto('/');

    // Submit registration via API
    const regRes = await page.request.post('/api/denizen/register', {
      form: {
        screenName: aliceName,
        password: password,
        gender: '2',
        avatarDef: 'hr=001/2/sh=002/1',
        mission: 'Testing Coke Studios Parity',
      },
    });

    expect(regRes.status()).toBe(200);
    const regJson = await regRes.json();
    expect(regJson.success).toBe(true);
    expect(regJson.screenName).toBe(aliceName);
    expect(regJson.launchUrl).toContain('/game/');
    expect(regJson.sw1).toBeTruthy();
    expect(regJson.sw2).toBeTruthy();

    // Verify cookies include coke_session
    const cookies = await page.context().cookies();
    const sessionCookie = cookies.find((c) => c.name === 'coke_session');
    expect(sessionCookie).toBeTruthy();
    expect(sessionCookie?.value).toBe(regJson.sw2);
  });

  test('login rejects invalid credentials and accepts valid credentials', async ({ page }) => {
    // 1. Wrong password
    const failRes = await page.request.get(`/sf/ssldocs/loginDenizen.jsp?username=${encodeURIComponent(aliceName)}&password=WrongPassword`);
    const failText = await failRes.text();
    expect(failText).toContain('<ErrorCode>1</ErrorCode>');

    // 2. Correct password
    const okRes = await page.request.get(`/sf/ssldocs/loginDenizen.jsp?username=${encodeURIComponent(aliceName)}&password=${encodeURIComponent(password)}`);
    const okText = await okRes.text();
    expect(okText).toContain('<ErrorCode>0</ErrorCode>');
    expect(okText).toContain(`<ScreenName>${aliceName}</ScreenName>`);
    expect(okText).toContain('<Secret>');
  });

  test('two distinct accounts maintain completely isolated state and sessions', async ({ browser }) => {
    // Context 1: Alice
    const ctxAlice = await browser.newContext();
    const pageAlice = await ctxAlice.newPage();
    const regAlice = await pageAlice.request.post('/api/denizen/register', {
      form: { screenName: `A_${timestamp}`, password: password, gender: '1' },
    });
    const jsonAlice = await regAlice.json();

    // Context 2: Bob
    const ctxBob = await browser.newContext();
    const pageBob = await ctxBob.newPage();
    const regBob = await pageBob.request.post('/api/denizen/register', {
      form: { screenName: `B_${timestamp}`, password: password, gender: '2' },
    });
    const jsonBob = await regBob.json();

    // Verify session tokens are distinct
    expect(jsonAlice.sw2).not.toBe(jsonBob.sw2);

    // Alice cannot update Bob's profile
    const spoofRes = await pageAlice.request.post(`/api/denizen/profile?user=${jsonBob.screenName}`, {
      form: { mission: 'Hacked By Alice' },
    });
    expect(spoofRes.status()).toBe(403);

    await ctxAlice.close();
    await ctxBob.close();
  });

  test('site presentation gates game access when unauthenticated and connects game presentation upon sign-in', async ({ page }) => {
    const gateUser = `GateUser_${Date.now()}`;
    const gatePass = 'SecretPassword123!';

    // Ensure account exists beforehand
    const regRes = await page.request.post('/api/denizen/register', {
      form: { screenName: gateUser, password: gatePass, gender: '1' },
    });
    expect(regRes.status()).toBe(200);

    // 1. Visit portal as a guest / fresh visitor
    await page.goto('/');

    // Check that game access gate overlay is visible and quick travel / viewport headers are hidden
    const gate = page.locator('#gameAccessGate');
    await expect(gate).toBeVisible();

    const viewportHeader = page.locator('#viewportHeader');
    await expect(viewportHeader).toBeHidden();

    const quickTravel = page.locator('#quickTravelStrip');
    await expect(quickTravel).toBeHidden();

    // Check guest balances are marked as '--'
    const balanceDB = page.locator('#sidebarBalanceDB');
    await expect(balanceDB).toHaveText('--');

    // Check nav and badge prompt sign in
    const navGuestBtn = page.locator('#navGuestBtn');
    await expect(navGuestBtn).toBeVisible();
    const btnBadgeAuth = page.locator('#btnBadgeAuth');
    await expect(btnBadgeAuth).toContainText('Sign In / Register');

    // Game frame is loaded without user credentials initially
    const gameFrame = page.locator('#gameFrame');
    const initialSrc = await gameFrame.getAttribute('src');
    expect(initialSrc).toContain('/game/');
    expect(initialSrc).not.toContain('user=');

    // 2. Open login modal via the gate button
    const btnGateSignIn = page.locator('#btnGateSignIn');
    await btnGateSignIn.click();

    const modal = page.locator('#registerModal');
    await expect(modal).toBeVisible();

    // Fill registered credentials
    await page.locator('#loginScreenName').fill(gateUser);
    await page.locator('#loginPassword').fill(gatePass);
    await page.locator('#btnSubmitLogin').click();

    // 3. Verify successful authentication flow:
    // Modal closes and user is placed into Play tab
    await expect(modal).toBeHidden({ timeout: 5000 });

    // Gate is now hidden, revealing the game client
    await expect(gate).toBeHidden();

    // Viewport header and quick-travel lounges are visible with user's screen name
    await expect(viewportHeader).toBeVisible();
    const viewportUserTag = page.locator('#viewportUserTag');
    await expect(viewportUserTag).toContainText(gateUser);
    await expect(quickTravel).toBeVisible();

    // Nav bar and badge show logged-in DJ name
    const navUserText = page.locator('#navUserScreenName');
    await expect(navUserText).toContainText(gateUser);
    await expect(btnBadgeAuth).toContainText('Enter Studio');

    // Authoritative session tokens and credentials connected to game iframe src
    await page.waitForFunction(() => {
      const frame = document.getElementById('gameFrame') as HTMLIFrameElement;
      return frame && frame.src.includes('user=');
    });
    const authFrameSrc = await gameFrame.getAttribute('src');
    expect(authFrameSrc).toContain(`user=${encodeURIComponent(gateUser)}`);
    expect(authFrameSrc).toContain('sw1=');
    expect(authFrameSrc).toContain('sw2=');

    // 4. Sign Out resets site presentation to gated guest state
    const btnSignOut = page.locator('#btnBadgeSignOut');
    await expect(btnSignOut).toBeVisible();
    await btnSignOut.click();

    // User is returned to Home tab; guest nav is restored and balances reset
    await expect(navGuestBtn).toBeVisible();
    await expect(balanceDB).toHaveText('--');

    // Switch back to Play tab: game access gate is active and viewport headers hidden
    await page.locator('[data-tab="play"]').click();
    await expect(gate).toBeVisible();
    await expect(viewportHeader).toBeHidden();
    await expect(quickTravel).toBeHidden();
  });
});
