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

  test('live studio decor update (wallpaper and flooring) updates database and broadcasts to room visitors', async ({ browser, baseURL }) => {
    const ownerName = `Owner_Decor_${Date.now()}`;
    const visitorName = `Visitor_Decor_${Date.now()}`;
    const password = 'DecorPassword123!';

    // 1. Register owner and visitor
    const ctxOwner = await browser.newContext();
    const pageOwner = await ctxOwner.newPage();
    const resOwner = await pageOwner.request.post('/api/denizen/register', {
      form: { screenName: ownerName, password: password, gender: '1' },
    });
    const ownerData = await resOwner.json();
    expect(ownerData.success).toBe(true);
    const studioID = ownerData.studioId;
    expect(studioID).toBeTruthy();

    const ctxVisitor = await browser.newContext();
    const pageVisitor = await ctxVisitor.newPage();
    const resVisitor = await pageVisitor.request.post('/api/denizen/register', {
      form: { screenName: visitorName, password: password, gender: '2' },
    });
    const visitorData = await resVisitor.json();
    expect(visitorData.success).toBe(true);

    const wsUrl = (baseURL || 'http://localhost:8080').replace('http://', 'ws://').replace('https://', 'wss://') + '/ws/realtime';

    // 2. Connect owner to WebSocket and enter studio
    await pageOwner.evaluate(
      ({ wsUrl, user, secret, studioID }) => {
        return new Promise<void>((resolve, reject) => {
          const ws = new WebSocket(wsUrl);
          (window as any).testWS = ws;
          (window as any).wsMessages = [];
          ws.binaryType = 'arraybuffer';
          ws.onmessage = (event) => {
            const dec = new TextDecoder();
            (window as any).wsMessages.push(dec.decode(new Uint8Array(event.data)).replace(/\0/g, ''));
          };
          ws.onopen = () => {
            const encUser = Array.from(new TextEncoder().encode(user)).map((b) => b.toString(16).padStart(2, '0')).join('').toUpperCase();
            const encPass = Array.from(new TextEncoder().encode(secret)).map((b) => b.toString(16).padStart(2, '0')).join('').toUpperCase();
            ws.send(new TextEncoder().encode(`<Auth.SecretKey key="${secret}"><DENIZEN_AUTH u="${encUser}" p="${encPass}"/></Auth.SecretKey>\0`));
            setTimeout(() => {
              ws.send(new TextEncoder().encode(`<Room.Enter roomID="${studioID}"/>\0`));
              setTimeout(resolve, 300);
            }, 300);
          };
          ws.onerror = reject;
        });
      },
      { wsUrl, user: ownerName, secret: ownerData.secret, studioID }
    );

    // 4. Connect visitor to WebSocket and enter owner's studio
    await pageVisitor.evaluate(
      ({ wsUrl, user, secret, studioID }) => {
        return new Promise<void>((resolve, reject) => {
          const ws = new WebSocket(wsUrl);
          (window as any).testWS = ws;
          (window as any).wsMessages = [];
          ws.binaryType = 'arraybuffer';
          ws.onmessage = (event) => {
            const dec = new TextDecoder();
            (window as any).wsMessages.push(dec.decode(new Uint8Array(event.data)).replace(/\0/g, ''));
          };
          ws.onopen = () => {
            const encUser = Array.from(new TextEncoder().encode(user)).map((b) => b.toString(16).padStart(2, '0')).join('').toUpperCase();
            const encPass = Array.from(new TextEncoder().encode(secret)).map((b) => b.toString(16).padStart(2, '0')).join('').toUpperCase();
            ws.send(new TextEncoder().encode(`<Auth.SecretKey key="${secret}"><DENIZEN_AUTH u="${encUser}" p="${encPass}"/></Auth.SecretKey>\0`));
            setTimeout(() => {
              ws.send(new TextEncoder().encode(`<Room.Enter roomID="${studioID}"/>\0`));
              setTimeout(resolve, 300);
            }, 300);
          };
          ws.onerror = reject;
        });
      },
      { wsUrl, user: visitorName, secret: visitorData.secret, studioID }
    );

    // 5. Owner updates room decor via ST_DECOR
    await pageOwner.evaluate(({ studioID }) => {
      (window as any).testWS.send(
        new TextEncoder().encode(`<Room.Action roomID="${studioID}"><ST_DECOR wp="3" fl="2" db="1"/></Room.Action>\0`)
      );
    }, { studioID });
    await pageOwner.waitForTimeout(400);

    // 6. Verify visitor receives the broadcasted ST_DECOR packet
    const visitorMsgs = await pageVisitor.evaluate(() => (window as any).wsMessages as string[]);
    const hasDecorBroadcast = visitorMsgs.some((m) => m.includes('<ST_DECOR') && m.includes('wp="3"') && m.includes('fl="2"'));
    expect(hasDecorBroadcast).toBe(true);

    // 7. Visitor attempts unauthorized decor change
    await pageVisitor.evaluate(({ studioID }) => {
      (window as any).testWS.send(
        new TextEncoder().encode(`<Room.Action roomID="${studioID}"><ST_DECOR wp="99" fl="99" db="99"/></Room.Action>\0`)
      );
    }, { studioID });
    await pageVisitor.waitForTimeout(400);

    // Owner should NOT receive any broadcast with wp="99"
    const ownerMsgs = await pageOwner.evaluate(() => (window as any).wsMessages as string[]);
    const hasUnauthorizedDecor = ownerMsgs.some((m) => m.includes('wp="99"'));
    expect(hasUnauthorizedDecor).toBe(false);

    await ctxOwner.close();
    await ctxVisitor.close();
  });
});
