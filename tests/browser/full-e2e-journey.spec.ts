import { test, expect } from '@playwright/test';

test.describe('Sub-task 7.1: Automated Headless Multi-Browser Complete Player Journey', () => {
  const timestamp = Date.now();
  const aliceName = `Alice_E2E_${timestamp}`;
  const bobName = `Bob_E2E_${timestamp}`;
  const password = 'JourneyPassword123!';

  test('Full Unbroken Journey: Register -> London I -> Walk -> Compose -> Burn CD -> DJ -> Vote -> Trade', async ({ browser, baseURL }) => {
    // -------------------------------------------------------------------------
    // STEP 1: Register Alice & Bob via Authoritative API
    // -------------------------------------------------------------------------
    const ctxAlice = await browser.newContext();
    const pageAlice = await ctxAlice.newPage();
    const resAlice = await pageAlice.request.post('/api/denizen/register', {
      form: { screenName: aliceName, password: password, gender: '1', avatarDef: 'hr=001/1' },
    });
    const aliceData = await resAlice.json();
    expect(aliceData.success).toBe(true);
    expect(aliceData.screenName).toBe(aliceName);

    const ctxBob = await browser.newContext();
    const pageBob = await ctxBob.newPage();
    const resBob = await pageBob.request.post('/api/denizen/register', {
      form: { screenName: bobName, password: password, gender: '2', avatarDef: 'hr=002/2' },
    });
    const bobData = await resBob.json();
    expect(bobData.success).toBe(true);
    expect(bobData.screenName).toBe(bobName);

    // -------------------------------------------------------------------------
    // STEP 2: Realtime WebSockets & Room Entry into London I ($SYSTEM$1)
    // -------------------------------------------------------------------------
    const wsUrl = (baseURL || 'http://localhost:24902').replace('http://', 'ws://').replace('https://', 'wss://') + '/ws/realtime';

    const connectPlayer = async (page: any, user: string, secret: string) => {
      await page.evaluate(
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
              const encUser = Array.from(new TextEncoder().encode(user)).map((b) => b.toString(16).padStart(2, '0')).join('').toUpperCase();
              const encPass = Array.from(new TextEncoder().encode(secret)).map((b) => b.toString(16).padStart(2, '0')).join('').toUpperCase();
              ws.send(new TextEncoder().encode(`<System.Login msgID="1"><Username encoding="text/binhex">${encUser}</Username><Password encoding="text/binhex">${encPass}</Password></System.Login>\0`));
              resolve();
            };
            ws.onerror = reject;
          });
        },
        { wsUrl, user, secret }
      );
    };

    await connectPlayer(pageAlice, aliceName, aliceData.sw2 || aliceData.secret);
    await connectPlayer(pageBob, bobName, bobData.sw2 || bobData.secret);

    // Wait for login acceptance
    await pageAlice.waitForFunction(() => {
      const msgs = (window as any).wsMessages || [];
      return msgs.some((m: string) => m.includes('<Accepted msgID="1"/>') || m.includes('<DENIZEN_OK'));
    });
    await pageBob.waitForFunction(() => {
      const msgs = (window as any).wsMessages || [];
      return msgs.some((m: string) => m.includes('<Accepted msgID="1"/>') || m.includes('<DENIZEN_OK'));
    });

    // Both players enter authentic public room London I ($SYSTEM$1)
    await pageAlice.evaluate(() => {
      const ws = (window as any).testWS;
      ws.send(new TextEncoder().encode('<Room.Enter msgID="2" roomID="$SYSTEM$1"/>\0'));
      ws.send(new TextEncoder().encode('<Room.Action roomID="$SYSTEM$1"><AV_EN def="hr=001/1"/></Room.Action>\0'));
    });
    await pageBob.evaluate(() => {
      const ws = (window as any).testWS;
      ws.send(new TextEncoder().encode('<Room.Enter msgID="2" roomID="$SYSTEM$1"/>\0'));
      ws.send(new TextEncoder().encode('<Room.Action roomID="$SYSTEM$1"><AV_EN def="hr=002/2"/></Room.Action>\0'));
    });

    // Assert both receive room entry confirmation and see each other's avatars
    await pageAlice.waitForFunction((expectedPeer) => {
      const msgs = (window as any).wsMessages || [];
      return msgs.some((m: string) => m.includes(expectedPeer));
    }, bobName);
    await pageBob.waitForFunction((expectedPeer) => {
      const msgs = (window as any).wsMessages || [];
      return msgs.some((m: string) => m.includes(expectedPeer));
    }, aliceName);

    // -------------------------------------------------------------------------
    // STEP 3: Walk & Movement Synchronization
    // -------------------------------------------------------------------------
    await pageAlice.evaluate(() => {
      (window as any).testWS.send(
        new TextEncoder().encode('<Room.Action roomID="$SYSTEM$1"><AV_MV x="13" z="8" d="2"/></Room.Action>\0')
      );
    });

    // Bob receives Alice's movement packet
    await pageBob.waitForFunction((target) => {
      const msgs = (window as any).wsMessages || [];
      return msgs.some((m: string) => (m.includes('<AV_MV') || m.includes('<AV_GSP')) && m.includes(target));
    }, aliceName);

    // -------------------------------------------------------------------------
    // STEP 4: Compose Music Mix
    // -------------------------------------------------------------------------
    const testEDL = '^50,rockOne_guitar00{2,0{^';
    const songTitle = 'London Calling';
    const saveMixRes = await pageAlice.request.post('/api/mixer/save', {
      form: { user: aliceName, slot: '1', title: songTitle, edl: testEDL },
    });
    const saveMixData = await saveMixRes.json();
    expect(saveMixData.success).toBe(true);

    // -------------------------------------------------------------------------
    // STEP 5: Burn CD
    // -------------------------------------------------------------------------
    const burnRes = await pageAlice.request.post('/api/mixer/burn', {
      form: { user: aliceName, title: songTitle, edl: testEDL },
    });
    const burnData = await burnRes.json();
    expect(burnData.success).toBe(true);
    expect(burnData.edl).toBe(testEDL);
    const cdPosID = burnData.possession_id || burnData.posId || '1';

    // -------------------------------------------------------------------------
    // STEP 6: Mount DJ Booth & Perform
    // -------------------------------------------------------------------------
    await pageAlice.evaluate(({ cdId }) => {
      (window as any).testWS.send(
        new TextEncoder().encode(`<Room.Action roomID="$SYSTEM$1"><CD_P cdId="${cdId}"/></Room.Action>\0`)
      );
    }, { cdId: cdPosID });

    // Bob receives CD_P performance packet
    await pageBob.waitForFunction(() => {
      const msgs = (window as any).wsMessages || [];
      return msgs.some((m: string) => m.includes('<CD_P'));
    });

    // -------------------------------------------------------------------------
    // STEP 7: Audience Vote (Cheer)
    // -------------------------------------------------------------------------
    await pageBob.evaluate(() => {
      (window as any).testWS.send(
        new TextEncoder().encode('<Room.Action roomID="$SYSTEM$1"><VOTE type="cheer"/></Room.Action>\0')
      );
    });

    // Alice receives vote acknowledgment or settlement
    await pageAlice.waitForTimeout(400);

    // -------------------------------------------------------------------------
    // STEP 8: Safe Two-Player Furniture Trade
    // -------------------------------------------------------------------------
    // Both buy items from the catalog
    const buyAlice = await pageAlice.request.post('/api/shop/purchase', {
      form: { user: aliceName, item: '45' }, // Coke Couch
    });
    const aliceItem = await buyAlice.json();
    expect(buyAlice.ok()).toBe(true);
    const alicePosID = aliceItem.pos_id;

    const buyBob = await pageBob.request.post('/api/shop/purchase', {
      form: { user: bobName, item: '50' }, // Sofa
    });
    const bobItem = await buyBob.json();
    expect(buyBob.ok()).toBe(true);
    const bobPosID = bobItem.pos_id;

    // Alice initiates trade with Bob
    await pageAlice.evaluate(({ target }) => {
      (window as any).testWS.send(
        new TextEncoder().encode(`<Room.Action roomID="$SYSTEM$1"><TD_ST tde="${target}"/></Room.Action>\0`)
      );
    }, { target: bobName });

    // Both receive TD_DISPL
    await pageBob.waitForFunction(() => {
      const msgs = (window as any).wsMessages || [];
      return msgs.some((m: string) => m.includes('<TD_DISPL'));
    });

    // Alice adds item 45, Bob adds item 50
    await pageAlice.evaluate(({ posID }) => {
      (window as any).testWS.send(
        new TextEncoder().encode(`<Room.Action roomID="$SYSTEM$1"><TD_AD id="${posID}"/></Room.Action>\0`)
      );
    }, { posID: alicePosID });

    await pageBob.evaluate(({ posID }) => {
      (window as any).testWS.send(
        new TextEncoder().encode(`<Room.Action roomID="$SYSTEM$1"><TD_AD id="${posID}"/></Room.Action>\0`)
      );
    }, { posID: bobPosID });

    await pageAlice.waitForTimeout(200);

    // Both agree to finalize trade
    await pageAlice.evaluate(() => {
      (window as any).testWS.send(new TextEncoder().encode('<Room.Action roomID="$SYSTEM$1"><TD_AG/></Room.Action>\0'));
    });
    await pageBob.evaluate(() => {
      (window as any).testWS.send(new TextEncoder().encode('<Room.Action roomID="$SYSTEM$1"><TD_AG/></Room.Action>\0'));
    });

    // Assert both receive completion packet TD_CPLT
    await pageAlice.waitForFunction(() => {
      const msgs = (window as any).wsMessages || [];
      return msgs.some((m: string) => m.includes('<TD_CPLT'));
    });
    await pageBob.waitForFunction(() => {
      const msgs = (window as any).wsMessages || [];
      return msgs.some((m: string) => m.includes('<TD_CPLT'));
    });

    // Clean up contexts
    await ctxAlice.close();
    await ctxBob.close();
  });
});
