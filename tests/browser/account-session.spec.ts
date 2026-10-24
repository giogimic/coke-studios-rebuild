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
});
