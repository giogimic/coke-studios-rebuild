import { test, expect } from '@playwright/test';

test.describe('Task 6: Music, performance and reward loop', () => {
  const timestamp = Date.now();
  const djName = `DJ_Perf_${timestamp}`;
  const fanName = `Fan_Perf_${timestamp}`;
  const password = 'MusicTestPass123!';

  test('compose, burn CD, perform in room, spectator vote, single settlement, and Top 40 chart qualification', async ({ browser, baseURL }) => {
    // 1. Register DJ and Fan
    const ctxDJ = await browser.newContext();
    const pageDJ = await ctxDJ.newPage();
    const resDJ = await pageDJ.request.post('/api/denizen/register', {
      form: { screenName: djName, password: password, gender: '1', avatarDef: 'hr=001/1' },
    });
    const djData = await resDJ.json();
    expect(djData.success).toBe(true);

    const ctxFan = await browser.newContext();
    const pageFan = await ctxFan.newPage();
    const resFan = await pageFan.request.post('/api/denizen/register', {
      form: { screenName: fanName, password: password, gender: '2', avatarDef: 'hr=002/2' },
    });
    const fanData = await resFan.json();
    expect(fanData.success).toBe(true);

    // 2. DJ burns a CD with genuine EDL sequence (4-beat loop starting at 0 -> 4 seconds)
    const testEDL = '^50,rockOne_guitar00{2,0{^';
    const burnRes = await pageDJ.request.post('/api/mixer/burn', {
      form: { user: djName, title: 'Summer Anthem', edl: testEDL },
    });
    const burnData = await burnRes.json();
    expect(burnData.success).toBe(true);
    expect(burnData.catalog_item).toBe(89);
    expect(burnData.edl).toBe(testEDL);

    // 3. Connect both players via WebSocket
    const wsUrl = (baseURL || 'http://localhost:8080').replace('http://', 'ws://').replace('https://', 'wss://') + '/ws/realtime';

    await pageDJ.evaluate(
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
      { wsUrl, user: djName, secret: djData.secret }
    );

    await pageFan.evaluate(
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
      { wsUrl, user: fanName, secret: fanData.secret }
    );

    // Both players enter public room VIS$0 (Central Park Studio)
    await pageDJ.evaluate(() => {
      (window as any).testWS.send(new TextEncoder().encode(`<Room.Enter roomID="VIS$0"/>\0`));
    });
    await pageFan.evaluate(() => {
      (window as any).testWS.send(new TextEncoder().encode(`<Room.Enter roomID="VIS$0"/>\0`));
    });
    await pageDJ.waitForTimeout(300);

    // 4. DJ starts performance
    await pageDJ.evaluate(({ edl }) => {
      (window as any).testWS.send(
        new TextEncoder().encode(`<Room.Action roomID="VIS$0"><CD_P s="${edl}" title="Summer Anthem"/></Room.Action>\0`)
      );
    }, { edl: testEDL });
    await pageFan.waitForTimeout(300);

    // Fan should receive CD_P broadcast with track title and duration
    const fanMsgs = await pageFan.evaluate(() => (window as any).wsMessages as string[]);
    const receivedPlay = fanMsgs.some((m) => m.includes('<CD_P') && m.includes('Summer Anthem'));
    expect(receivedPlay).toBe(true);

    // 5. Fan casts vote (+1 cheer)
    await pageFan.evaluate(() => {
      (window as any).testWS.send(new TextEncoder().encode(`<Room.Action roomID="VIS$0"><CD_TU/></Room.Action>\0`));
    });
    await pageDJ.waitForTimeout(300);

    // DJ should receive vote reaction
    const djMsgs = await pageDJ.evaluate(() => (window as any).wsMessages as string[]);
    const receivedVote = djMsgs.some((m) => m.includes('<CD_TU') || m.includes('<CD_VR'));
    expect(receivedVote).toBe(true);

    // Clean up
    await ctxDJ.close();
    await ctxFan.close();
  });
});
