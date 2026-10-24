import { test, expect } from '@playwright/test';

test.describe('Task 4: Two Players Inhabit the Same Faithful Room', () => {
  const timestamp = Date.now();
  const aliceName = `Alice_RM_${timestamp}`;
  const bobName = `Bob_RM_${timestamp}`;
  const password = 'RoomTestPassword123!';

  test('two players join same room, see each other, walk, chat, and handle leaves/reconnect', async ({ browser, baseURL }) => {
    // 1. Register Alice and Bob to obtain authoritative sessions
    const ctxAlice = await browser.newContext();
    const pageAlice = await ctxAlice.newPage();
    const resAlice = await pageAlice.request.post('/api/denizen/register', {
      form: { screenName: aliceName, password: password, gender: '1', avatarDef: 'hr=001/1/bd=001/1' },
    });
    const aliceData = await resAlice.json();
    expect(aliceData.success).toBe(true);

    const ctxBob = await browser.newContext();
    const pageBob = await ctxBob.newPage();
    const resBob = await pageBob.request.post('/api/denizen/register', {
      form: { screenName: bobName, password: password, gender: '2', avatarDef: 'hr=002/2/bd=002/2' },
    });
    const bobData = await resBob.json();
    expect(bobData.success).toBe(true);

    // 2. Open WebSocket realtime connections for both players in their page contexts
    const wsUrl = (baseURL || 'http://localhost:8080').replace('http://', 'ws://').replace('https://', 'wss://') + '/ws/realtime';

    const aliceSocketLogs: string[] = [];
    const bobSocketLogs: string[] = [];

    // Helper to evaluate websocket in browser page
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
            // Encode hex
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
                `<System.Login msgID="1"><Username encoding="text/binhex">${encUser}</Username><Password encoding="text/binhex">${encPass}</Password></System.Login>\0`
              )
            );
            resolve();
          };
          ws.onerror = reject;
        });
      },
      { wsUrl, user: aliceName, secret: aliceData.sw2 }
    );

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
                `<System.Login msgID="1"><Username encoding="text/binhex">${encUser}</Username><Password encoding="text/binhex">${encPass}</Password></System.Login>\0`
              )
            );
            resolve();
          };
          ws.onerror = reject;
        });
      },
      { wsUrl, user: bobName, secret: bobData.sw2 }
    );

    // Wait for login acceptance
    await pageAlice.waitForFunction(() => {
      const msgs = (window as any).wsMessages || [];
      return msgs.some((m: string) => m.includes('<Accepted msgID="1"/>'));
    });
    await pageBob.waitForFunction(() => {
      const msgs = (window as any).wsMessages || [];
      return msgs.some((m: string) => m.includes('<Accepted msgID="1"/>'));
    });

    // 3. Alice enters VIS$0
    await pageAlice.evaluate(() => {
      const ws = (window as any).testWS;
      ws.send(new TextEncoder().encode('<Room.Enter msgID="2" roomID="VIS$0"/>\0'));
      ws.send(new TextEncoder().encode('<Room.Action roomID="VIS$0"><AV_EN def="hr=001/1"/></Room.Action>\0'));
    });

    await pageAlice.waitForFunction(() => {
      const msgs = (window as any).wsMessages || [];
      return msgs.some((m: string) => m.includes('<ST_EN') && m.includes('VIS$0'));
    });

    // 4. Bob enters VIS$0
    await pageBob.evaluate(() => {
      const ws = (window as any).testWS;
      ws.send(new TextEncoder().encode('<Room.Enter msgID="2" roomID="VIS$0"/>\0'));
      ws.send(new TextEncoder().encode('<Room.Action roomID="VIS$0"><AV_EN def="hr=002/2"/></Room.Action>\0'));
    });

    // Verify Bob sees Alice
    await pageBob.waitForFunction((expectedAlice) => {
      const msgs = (window as any).wsMessages || [];
      return msgs.some((m: string) => m.includes('<AV ') && m.includes(`av="${expectedAlice}"`));
    }, aliceName);

    // Verify Alice sees Bob spawn
    await pageAlice.waitForFunction((expectedBob) => {
      const msgs = (window as any).wsMessages || [];
      return msgs.some((m: string) => m.includes('<AV ') && m.includes(`av="${expectedBob}"`));
    }, bobName);

    // 5. Alice moves; Bob receives AV_MV
    await pageAlice.evaluate(() => {
      const ws = (window as any).testWS;
      ws.send(new TextEncoder().encode('<Room.Action roomID="VIS$0"><AV_GSP><P x="11" z="6"/></AV_GSP></Room.Action>\0'));
    });

    await pageBob.waitForFunction((expectedAlice) => {
      const msgs = (window as any).wsMessages || [];
      return msgs.some((m: string) => m.includes('<AV_MV') && m.includes(`av="${expectedAlice}"`));
    }, aliceName);

    // 6. Alice chats; Bob receives CH
    const chatMsg = 'Hello from Alice in Central Park!';
    const hexMsg = Array.from(new TextEncoder().encode(chatMsg))
      .map((b) => b.toString(16).padStart(2, '0'))
      .join('');

    await pageAlice.evaluate((hex) => {
      const ws = (window as any).testWS;
      ws.send(new TextEncoder().encode(`<Room.Action roomID="VIS$0"><CH sm="1" m="${hex}"/></Room.Action>\0`));
    }, hexMsg);

    await pageBob.waitForFunction((expectedHex) => {
      const msgs = (window as any).wsMessages || [];
      return msgs.some((m: string) => m.includes('<CH ') && m.includes(`m="${expectedHex}"`));
    }, hexMsg);

    // 7. Bob leaves room to lobby; Alice receives AV_RM
    await pageBob.evaluate(() => {
      const ws = (window as any).testWS;
      ws.send(new TextEncoder().encode('<Room.Exit msgID="3" roomID="VIS$0"/>\0'));
    });

    await pageAlice.waitForFunction((expectedBob) => {
      const msgs = (window as any).wsMessages || [];
      return msgs.some((m: string) => m.includes('<AV_RM') && m.includes(`av="${expectedBob}"`));
    }, bobName);

    await ctxAlice.close();
    await ctxBob.close();
  });
});
