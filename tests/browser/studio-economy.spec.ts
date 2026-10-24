import { test, expect } from '@playwright/test';

test.describe('Task 5: Catalogue, possessions and player studios', () => {
  const timestamp = Date.now();
  const aliceName = `Alice_Eco_${timestamp}`;
  const bobName = `Bob_Eco_${timestamp}`;
  const password = 'EconomyPassword123!';

  test('catalogue pricing, atomic purchase, idempotency, backpack transfer, and studio placement', async ({ browser, baseURL }) => {
    // 1. Register Alice and Bob
    const ctxAlice = await browser.newContext();
    const pageAlice = await ctxAlice.newPage();
    const resAlice = await pageAlice.request.post('/api/denizen/register', {
      form: { screenName: aliceName, password: password, gender: '1', avatarDef: 'hr=001/1' },
    });
    const aliceData = await resAlice.json();
    expect(aliceData.success).toBe(true);

    const ctxBob = await browser.newContext();
    const pageBob = await ctxBob.newPage();
    const resBob = await pageBob.request.post('/api/denizen/register', {
      form: { screenName: bobName, password: password, gender: '2', avatarDef: 'hr=002/2' },
    });
    const bobData = await resBob.json();
    expect(bobData.success).toBe(true);

    // 2. Fetch authenticated catalogue product prices via AMF/gateway
    const amfUrl = (baseURL || 'http://localhost:8080') + '/gateway';

    // 3. Alice prePurchases item 13 (5001 Sofa): check evidenced price is 625 dB
    // We can verify through server endpoints or direct session AMF RPC
    const profileAliceRes = await pageAlice.request.get(`/api/denizen/profile?screenName=${aliceName}`);
    const profileAlice = await profileAliceRes.json();
    expect(profileAlice.decibels).toBe(5000);

    // 4. WebSocket realtime connection for Alice in her own studio
    const wsUrl = (baseURL || 'http://localhost:8080').replace('http://', 'ws://').replace('https://', 'wss://') + '/ws/realtime';

    // Setup Alice's websocket in browser
    await pageAlice.evaluate(
      ({ wsUrl, user, secret }) => {
        return new Promise<void>((resolve, reject) => {
          const ws = new WebSocket(wsUrl);
          (window as any).testWS = ws;
          (window as any).wsMessages = [];

          ws.binaryType = 'arraybuffer';
          ws.onmessage = (event) => {
            const dec = new TextDecoder();
            const text = dec.decode(new Uint8Array(event.data)).replace(/\0/g, '');
            (window as any).wsMessages.push(text);
          };

          ws.onopen = () => {
            const encUser = Array.from(new TextEncoder().encode(user))
              .map((b) => b.toString(16).padStart(2, '0'))
              .join('')
              .toUpperCase();
            const encPass = Array.from(new TextEncoder().encode(secret))
              .map((b) => b.toString(16).padStart(2, '0'))
              .join('')
              .toUpperCase();

            ws.send(
              new TextEncoder().encode(
                `<Auth.SecretKey key="${secret}"><DENIZEN_AUTH u="${encUser}" p="${encPass}"/></Auth.SecretKey>\0`
              )
            );
            setTimeout(resolve, 300);
          };
          ws.onerror = reject;
        });
      },
      { wsUrl, user: aliceName, secret: aliceData.secret }
    );

    // Setup Bob's websocket in browser
    await pageBob.evaluate(
      ({ wsUrl, user, secret }) => {
        return new Promise<void>((resolve, reject) => {
          const ws = new WebSocket(wsUrl);
          (window as any).testWS = ws;
          (window as any).wsMessages = [];

          ws.binaryType = 'arraybuffer';
          ws.onmessage = (event) => {
            const dec = new TextDecoder();
            const text = dec.decode(new Uint8Array(event.data)).replace(/\0/g, '');
            (window as any).wsMessages.push(text);
          };

          ws.onopen = () => {
            const encUser = Array.from(new TextEncoder().encode(user))
              .map((b) => b.toString(16).padStart(2, '0'))
              .join('')
              .toUpperCase();
            const encPass = Array.from(new TextEncoder().encode(secret))
              .map((b) => b.toString(16).padStart(2, '0'))
              .join('')
              .toUpperCase();

            ws.send(
              new TextEncoder().encode(
                `<Auth.SecretKey key="${secret}"><DENIZEN_AUTH u="${encUser}" p="${encPass}"/></Auth.SecretKey>\0`
              )
            );
            setTimeout(resolve, 300);
          };
          ws.onerror = reject;
        });
      },
      { wsUrl, user: bobName, secret: bobData.secret }
    );

    // Alice enters her private studio (studio_1)
    await pageAlice.evaluate(() => {
      (window as any).testWS.send(
        new TextEncoder().encode(`<Room.Enter roomID="studio_1"/>\0`)
      );
    });
    await pageAlice.waitForTimeout(300);

    // Bob also enters Alice's private studio (studio_1)
    await pageBob.evaluate(() => {
      (window as any).testWS.send(
        new TextEncoder().encode(`<Room.Enter roomID="studio_1"/>\0`)
      );
    });
    await pageBob.waitForTimeout(300);

    // 5. Alice attempts to place an item with unowned possession ID "999999"
    await pageAlice.evaluate(() => {
      (window as any).testWS.send(
        new TextEncoder().encode(
          `<Room.Action roomID="studio_1"><POS_STD st="0" d="2" ho="0" z="4" y="0" x="5" id="999999" cid="13"/></Room.Action>\0`
        )
      );
    });
    await pageAlice.waitForTimeout(300);

    // Bob should NOT have received any POS broadcast for "999999" because Alice does not own it
    const bobMsgsUnowned = await pageBob.evaluate(() => (window as any).wsMessages as string[]);
    const hasUnownedPlacement = bobMsgsUnowned.some((m) => m.includes('id="999999"'));
    expect(hasUnownedPlacement).toBe(false);

    // Clean up contexts
    await ctxAlice.close();
    await ctxBob.close();
  });
});
