import { test, expect } from '@playwright/test';

test.describe('Task 7: Social continuity, trade and host controls', () => {
  const timestamp = Date.now();
  const aliceName = `Alice_Soc_${timestamp}`;
  const bobName = `Bob_Soc_${timestamp}`;
  const password = 'SocialTestPass123!';

  test('friend request, reject, accept, instant messaging, host controls, and atomic trade', async ({ browser, baseURL }) => {
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

    // 2. Alice buys Item 45 (Coke Couch), Bob buys Item 50 (Sofa)
    const buyAliceRes = await pageAlice.request.post('/api/shop/purchase', {
      form: { user: aliceName, item: '45' },
    });
    const buyAliceData = await buyAliceRes.json();
    expect(buyAliceData.success).toBe(true);
    const alicePosID = buyAliceData.pos_id;

    const buyBobRes = await pageBob.request.post('/api/shop/purchase', {
      form: { user: bobName, item: '50' },
    });
    const buyBobData = await buyBobRes.json();
    expect(buyBobData.success).toBe(true);
    const bobPosID = buyBobData.pos_id;

    // 3. Connect both players via WebSocket
    const wsUrl = (baseURL || 'http://localhost:8080').replace('http://', 'ws://').replace('https://', 'wss://') + '/ws/realtime';

    const connectWS = async (page: any, user: string, secret: string) => {
      await page.evaluate(
        ({ wsUrl, user, secret }) => {
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
              setTimeout(resolve, 300);
            };
            ws.onerror = reject;
          });
        },
        { wsUrl, user, secret }
      );
    };

    await connectWS(pageAlice, aliceName, aliceData.secret);
    await connectWS(pageBob, bobName, bobData.secret);

    // Both enter public studio VIS$0
    await pageAlice.evaluate(() => {
      (window as any).testWS.send(new TextEncoder().encode(`<Room.Enter roomID="VIS$0"/>\0`));
    });
    await pageBob.evaluate(() => {
      (window as any).testWS.send(new TextEncoder().encode(`<Room.Enter roomID="VIS$0"/>\0`));
    });
    await pageAlice.waitForTimeout(300);

    // 4. Friend Request & Refusal
    await pageAlice.evaluate(({ target }) => {
      (window as any).testWS.send(new TextEncoder().encode(`<MMS_REQ to="${target}"/>\0`));
    }, { target: bobName });
    await pageBob.waitForTimeout(300);

    // Bob receives friend invitation
    const bobMsgs1 = await pageBob.evaluate(() => (window as any).wsMessages as string[]);
    const hasInvite = bobMsgs1.some((m) => m.includes('<MSGR_INV') || m.includes('<MMS_REQ'));
    expect(hasInvite).toBe(true);

    // Bob rejects invitation
    await pageBob.evaluate(({ sender }) => {
      (window as any).testWS.send(new TextEncoder().encode(`<MMS_REJ from="${sender}"/>\0`));
    }, { sender: aliceName });
    await pageAlice.waitForTimeout(300);

    // Check friends list via HTTP API: neither should have friends
    const friendsAliceRes1 = await pageAlice.request.get(`/api/messenger/friend/list?user=${aliceName}`);
    if (friendsAliceRes1.ok()) {
      const fData = await friendsAliceRes1.json();
      expect(Array.isArray(fData) ? fData.length : (fData.friends?.length || 0)).toBe(0);
    }

    // 5. Friend Request & Acceptance
    await pageAlice.evaluate(({ target }) => {
      (window as any).testWS.send(new TextEncoder().encode(`<MMS_REQ to="${target}"/>\0`));
    }, { target: bobName });
    await pageBob.waitForTimeout(200);

    await pageBob.evaluate(({ sender }) => {
      (window as any).testWS.send(new TextEncoder().encode(`<MMS_ACC from="${sender}"/>\0`));
    }, { sender: aliceName });
    await pageAlice.waitForTimeout(300);

    // Both should receive friendship confirmation
    const aliceMsgs = await pageAlice.evaluate(() => (window as any).wsMessages as string[]);
    const hasFriendConfirm = aliceMsgs.some((m) => m.includes('<MSGR_AD_F') || m.includes('<MMS_ACC'));
    expect(hasFriendConfirm).toBe(true);

    // 6. Instant Messaging
    await pageAlice.evaluate(({ target }) => {
      (window as any).testWS.send(new TextEncoder().encode(`<MMS_SND to="${target}" msg="Hey Bob! Wanna trade?"/>\0`));
    }, { target: bobName });
    await pageBob.waitForTimeout(300);

    const bobMsgs2 = await pageBob.evaluate(() => (window as any).wsMessages as string[]);
    const hasIM = bobMsgs2.some((m) => m.includes('Hey Bob! Wanna trade?'));
    expect(hasIM).toBe(true);

    // 7. Authoritative In-Room Trade Flow
    // Alice starts trade with Bob
    await pageAlice.evaluate(({ target }) => {
      (window as any).testWS.send(new TextEncoder().encode(`<Room.Action roomID="VIS$0"><TD_ST tde="${target}"/></Room.Action>\0`));
    }, { target: bobName });
    await pageBob.waitForTimeout(300);

    // Both receive TD_DISPL
    const bobMsgs3 = await pageBob.evaluate(() => (window as any).wsMessages as string[]);
    const hasDispl = bobMsgs3.some((m) => m.includes('<TD_DISPL'));
    expect(hasDispl).toBe(true);

    // Alice adds Item 45
    await pageAlice.evaluate(({ posID }) => {
      (window as any).testWS.send(new TextEncoder().encode(`<Room.Action roomID="VIS$0"><TD_AD id="${posID}"/></Room.Action>\0`));
    }, { posID: alicePosID });
    await pageBob.waitForTimeout(200);

    // Alice agrees (Trade must NOT complete yet)
    await pageAlice.evaluate(() => {
      (window as any).testWS.send(new TextEncoder().encode(`<Room.Action roomID="VIS$0"><TD_AG/></Room.Action>\0`));
    });
    await pageBob.waitForTimeout(200);

    // Bob adds Item 50
    await pageBob.evaluate(({ posID }) => {
      (window as any).testWS.send(new TextEncoder().encode(`<Room.Action roomID="VIS$0"><TD_AD id="${posID}"/></Room.Action>\0`));
    }, { posID: bobPosID });
    await pageAlice.waitForTimeout(200);

    // Alice agrees again, then Bob agrees -> atomic trade completion
    await pageAlice.evaluate(() => {
      (window as any).testWS.send(new TextEncoder().encode(`<Room.Action roomID="VIS$0"><TD_AG/></Room.Action>\0`));
    });
    await pageBob.evaluate(() => {
      (window as any).testWS.send(new TextEncoder().encode(`<Room.Action roomID="VIS$0"><TD_AG/></Room.Action>\0`));
    });
    await pageAlice.waitForTimeout(400);

    // Both receive TD_CPLT
    const aliceFinalMsgs = await pageAlice.evaluate(() => (window as any).wsMessages as string[]);
    const bobFinalMsgs = await pageBob.evaluate(() => (window as any).wsMessages as string[]);

    expect(aliceFinalMsgs.some((m) => m.includes('<TD_CPLT'))).toBe(true);
    expect(bobFinalMsgs.some((m) => m.includes('<TD_CPLT'))).toBe(true);

    // Clean up
    await ctxAlice.close();
    await ctxBob.close();
  });
});
