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

    // Check that unauthenticated visitor sees guest state in avatar badge and login controls
    const badgeName = page.locator('#badgeScreenName');
    await expect(badgeName).toHaveText('GUEST');

    const loggedOutBtns = page.locator('#loggedOutButtons');
    await expect(loggedOutBtns).toBeVisible();

    const loggedInBtns = page.locator('#loggedInButtons');
    await expect(loggedInBtns).toBeHidden();

    // Game frame is loaded without user credentials initially
    const gameFrame = page.locator('#gameFrame');
    const initialSrc = await gameFrame.getAttribute('src');
    expect(initialSrc).toContain('/game/');
    expect(initialSrc).not.toContain('user=');

    // 2. Log in using screenName and password via the badge controls
    await page.locator('#badgeScreenNameInput').fill(gateUser);
    await page.locator('#badgePasswordInput').fill(gatePass);
    await page.locator('#loggedOutButtons button:has-text("Sign In")').click();

    // 3. Verify successful authentication flow:
    // Logged in buttons are now visible and logged out buttons are hidden
    await expect(loggedInBtns).toBeVisible({ timeout: 5000 });
    await expect(loggedOutBtns).toBeHidden();

    // Badge shows logged-in DJ handle
    await expect(badgeName).toHaveText(gateUser);

    // Authoritative session tokens and credentials connected to game iframe src
    await page.waitForFunction(() => {
      const frame = document.getElementById('gameFrame') as HTMLIFrameElement;
      return frame && frame.src.includes('user=');
    });
    const authFrameSrc = await gameFrame.getAttribute('src');
    expect(authFrameSrc).toContain(`user=${encodeURIComponent(gateUser)}`);
    expect(authFrameSrc).toContain('sw1=');
    expect(authFrameSrc).toContain('sw2=');

    // 4. Sign Out resets site presentation to guest state
    const btnSignOut = loggedInBtns.locator('button:has-text("Sign Out")');
    await expect(btnSignOut).toBeVisible();
    await btnSignOut.click();

    // Guest state is restored
    await expect(badgeName).toHaveText('GUEST');
    await expect(loggedOutBtns).toBeVisible();
    await expect(loggedInBtns).toBeHidden();
  });

  test('redemption of promotional Coca-Cola bottle cap PIN credits V-Coins and furniture to backpack', async ({ page }) => {
    const pinUser = `CapUser_${Date.now()}`;
    const pinPass = 'CapPass123!';
    const dynamicPin = `TEST-CAP-${Date.now()}`;

    // Register user
    const regRes = await page.request.post('/api/denizen/register', {
      form: { screenName: pinUser, password: pinPass, gender: '1' },
    });
    expect(regRes.status()).toBe(200);

    // Seed dynamic promotional PIN code for idempotent test runs
    const seedRes = await page.request.post('/api/caps/create', {
      form: { pin: dynamicPin, vcoins: '100', item: '52' },
    });
    expect(seedRes.status()).toBe(200);

    // 1. Invalid PIN code fails
    const failRes = await page.request.post('/sf/ssldocs/redeemCap.jsp', {
      form: { user: pinUser, pin: 'INVALID-CODE-999' },
    });
    expect(failRes.status()).toBe(400);
    const failJson = await failRes.json();
    expect(failJson.success).toBe(false);
    expect(failJson.error).toContain('invalid or non-existent');

    // 2. Valid promotional PIN code succeeds
    const okRes = await page.request.post('/sf/ssldocs/redeemCap.jsp', {
      form: { user: pinUser, pin: dynamicPin },
    });
    expect(okRes.status()).toBe(200);
    const okJson = await okRes.json();
    expect(okJson.success).toBe(true);
    expect(okJson.reward_vcoins).toBe(100);
    expect(okJson.reward_item).toBe(52); // Coke Neon Sign

    // 3. Second redemption of same PIN fails
    const dupRes = await page.request.post('/sf/ssldocs/redeemCap.jsp', {
      form: { user: pinUser, pin: dynamicPin },
    });
    expect(dupRes.status()).toBe(400);
    const dupJson = await dupRes.json();
    expect(dupJson.success).toBe(false);
    expect(dupJson.error).toContain('already been redeemed');
  });
});
