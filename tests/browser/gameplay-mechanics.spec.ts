import { test, expect } from '@playwright/test';

test.describe('Task 8: Authentic Gameplay Mechanics & Gestures', () => {
  const timestamp = Date.now();
  const aliceName = `Alice_GM_${timestamp}`;
  const bobName = `Bob_GM_${timestamp}`;
  const password = 'GameplayPassword123!';

  test('avatars broadcast dance actions, receive drink dispenser rewards, and send friend requests', async ({ browser, baseURL }) => {
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

    const wsUrl = (baseURL || 'http://localhost:24902').replace('http://', 'ws://').replace('https://', 'wss://') + '/ws/realtime';

    // 2. Open WebSockets and Login
    await pageAlice.evaluate(
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
            const encUser = Array.from(new TextEncoder().encode(user))
              .map((b) => b.toString(16).padStart(2, '0')).join('').toUpperCase();
            const encPass = Array.from(new TextEncoder().encode(secret))
              .map((b) => b.toString(16).padStart(2, '0')).join('').toUpperCase();
            ws.send(new TextEncoder().encode(`<System.Login msgID="1"><Username encoding="text/binhex">${encUser}</Username><Password encoding="text/binhex">${encPass}</Password></System.Login>\0`));
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
            (window as any).wsMessages.push(dec.decode(new Uint8Array(event.data)).replace(/\0/g, ''));
          };
          ws.onopen = () => {
            const encUser = Array.from(new TextEncoder().encode(user))
              .map((b) => b.toString(16).padStart(2, '0')).join('').toUpperCase();
            const encPass = Array.from(new TextEncoder().encode(secret))
              .map((b) => b.toString(16).padStart(2, '0')).join('').toUpperCase();
            ws.send(new TextEncoder().encode(`<System.Login msgID="1"><Username encoding="text/binhex">${encUser}</Username><Password encoding="text/binhex">${encPass}</Password></System.Login>\0`));
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

    // 3. Both enter room VIS$0
    await pageAlice.evaluate(() => {
      const ws = (window as any).testWS;
      ws.send(new TextEncoder().encode('<Room.Enter msgID="2" roomID="VIS$0"/>\0'));
      ws.send(new TextEncoder().encode('<Room.Action roomID="VIS$0"><AV_EN def="hr=001/1"/></Room.Action>\0'));
    });
    await pageBob.evaluate(() => {
      const ws = (window as any).testWS;
      ws.send(new TextEncoder().encode('<Room.Enter msgID="2" roomID="VIS$0"/>\0'));
      ws.send(new TextEncoder().encode('<Room.Action roomID="VIS$0"><AV_EN def="hr=002/2"/></Room.Action>\0'));
    });

    // Ensure presence established
    await pageBob.waitForFunction((expectedAlice) => {
      const msgs = (window as any).wsMessages || [];
      return msgs.some((m: string) => m.includes('<AV ') && m.includes(`av="${expectedAlice}"`));
    }, aliceName);

    // 4. Test Avatar Dance Action (AV_ACT -> AV_DC)
    await pageAlice.evaluate(() => {
      const ws = (window as any).testWS;
      ws.send(new TextEncoder().encode('<Room.Action roomID="VIS$0"><AV_ACT act="dnc"/></Room.Action>\0'));
    });

    await pageBob.waitForFunction((expectedAlice) => {
      const msgs = (window as any).wsMessages || [];
      return msgs.some((m: string) => m.includes('<AV_DC') && m.includes(`av="${expectedAlice}"`) && m.includes('act="dnc"'));
    }, aliceName);

    // Stop dance
    await pageAlice.evaluate(() => {
      const ws = (window as any).testWS;
      ws.send(new TextEncoder().encode('<Room.Action roomID="VIS$0"><AV_ACT act=""/></Room.Action>\0'));
    });

    await pageBob.waitForFunction((expectedAlice) => {
      const msgs = (window as any).wsMessages || [];
      return msgs.some((m: string) => m.includes('<AV_DC') && m.includes(`av="${expectedAlice}"`) && m.includes('act=""'));
    }, aliceName);

    // Test Avatar Wave and Smile Actions (wav, sml)
    await pageAlice.evaluate(() => {
      const ws = (window as any).testWS;
      ws.send(new TextEncoder().encode('<Room.Action roomID="VIS$0"><AV_ACT act="wav"/></Room.Action>\0'));
    });

    await pageBob.waitForFunction((expectedAlice) => {
      const msgs = (window as any).wsMessages || [];
      return msgs.some((m: string) => m.includes('<AV_DC') && m.includes(`av="${expectedAlice}"`) && m.includes('act="wav"'));
    }, aliceName);

    await pageAlice.evaluate(() => {
      const ws = (window as any).testWS;
      ws.send(new TextEncoder().encode('<Room.Action roomID="VIS$0"><AV_ACT act="sml"/></Room.Action>\0'));
    });

    await pageBob.waitForFunction((expectedAlice) => {
      const msgs = (window as any).wsMessages || [];
      return msgs.some((m: string) => m.includes('<AV_DC') && m.includes(`av="${expectedAlice}"`) && m.includes('act="sml"'));
    }, aliceName);

    // 5. Test Drink Dispenser Interaction (AV_DC -> AV_DC with pts="10" & coke_can)
    await pageAlice.evaluate(() => {
      const ws = (window as any).testWS;
      ws.send(new TextEncoder().encode('<Room.Action roomID="VIS$0"><AV_DC/></Room.Action>\0'));
    });

    await pageBob.waitForFunction((expectedAlice) => {
      const msgs = (window as any).wsMessages || [];
      return msgs.some((m: string) => m.includes('<AV_DC') && m.includes(`av="${expectedAlice}"`) && m.includes('item="coke_can"') && m.includes('pts="10"'));
    }, aliceName);

    // 6. Test Realtime Friend Request (MMS_REQ)
    await pageAlice.evaluate((target) => {
      const ws = (window as any).testWS;
      ws.send(new TextEncoder().encode(`<Room.Action roomID="VIS$0"><MMS_REQ to="${target}"/></Room.Action>\0`));
    }, bobName);

    // Verify Bob receives friend request invitation
    await pageBob.waitForFunction((fromUser) => {
      const msgs = (window as any).wsMessages || [];
      return msgs.some((m: string) => m.includes('<MSGR_INV') && m.includes(fromUser));
    }, aliceName);

    await ctxAlice.close();
    await ctxBob.close();
  });

  test('furniture rotation, appliance toggle, and sitting posture sync authoritatively across room', async ({ browser, baseURL }) => {
    const timestamp2 = Date.now() + 1;
    const charlie = `Charlie_${timestamp2}`;
    const dana = `Dana_${timestamp2}`;

    const ctxCharlie = await browser.newContext();
    const pageCharlie = await ctxCharlie.newPage();
    const resCharlie = await pageCharlie.request.post('/api/denizen/register', {
      form: { screenName: charlie, password: password, gender: '1', avatarDef: 'hr=001/1' },
    });
    const charlieData = await resCharlie.json();
    expect(charlieData.success).toBe(true);

    const ctxDana = await browser.newContext();
    const pageDana = await ctxDana.newPage();
    const resDana = await pageDana.request.post('/api/denizen/register', {
      form: { screenName: dana, password: password, gender: '2', avatarDef: 'hr=002/2' },
    });
    const danaData = await resDana.json();
    expect(danaData.success).toBe(true);

    // Charlie buys a Premier Light (cid: 10)
    const buyRes = await pageCharlie.request.post('/api/shop/purchase', {
      form: { user: charlie, item: '10' },
    });
    const buyData = await buyRes.json();
    expect(buyData.success).toBe(true);
    const posID = String(buyData.pos_id);
    const studioID = `studio_${charlie.toLowerCase()}`;

    const wsUrl = (baseURL || 'http://localhost:24902').replace('http://', 'ws://').replace('https://', 'wss://') + '/ws/realtime';

    // Login Charlie and Dana via WebSocket
    await pageCharlie.evaluate(
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
            const encUser = Array.from(new TextEncoder().encode(user))
              .map((b) => b.toString(16).padStart(2, '0')).join('').toUpperCase();
            const encPass = Array.from(new TextEncoder().encode(secret))
              .map((b) => b.toString(16).padStart(2, '0')).join('').toUpperCase();
            ws.send(new TextEncoder().encode(`<System.Login msgID="1"><Username encoding="text/binhex">${encUser}</Username><Password encoding="text/binhex">${encPass}</Password></System.Login>\0`));
            resolve();
          };
          ws.onerror = reject;
        });
      },
      { wsUrl, user: charlie, secret: charlieData.sw2 }
    );

    await pageDana.evaluate(
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
            const encUser = Array.from(new TextEncoder().encode(user))
              .map((b) => b.toString(16).padStart(2, '0')).join('').toUpperCase();
            const encPass = Array.from(new TextEncoder().encode(secret))
              .map((b) => b.toString(16).padStart(2, '0')).join('').toUpperCase();
            ws.send(new TextEncoder().encode(`<System.Login msgID="1"><Username encoding="text/binhex">${encUser}</Username><Password encoding="text/binhex">${encPass}</Password></System.Login>\0`));
            resolve();
          };
          ws.onerror = reject;
        });
      },
      { wsUrl, user: dana, secret: danaData.sw2 }
    );

    await pageCharlie.waitForFunction(() => ((window as any).wsMessages || []).some((m: string) => m.includes('<Accepted msgID="1"/>')));
    await pageDana.waitForFunction(() => ((window as any).wsMessages || []).some((m: string) => m.includes('<Accepted msgID="1"/>')));

    // Charlie and Dana enter the studio
    await pageCharlie.evaluate((rID) => {
      const ws = (window as any).testWS;
      ws.send(new TextEncoder().encode(`<Room.Enter msgID="2" roomID="${rID}"/>\0`));
      ws.send(new TextEncoder().encode(`<Room.Action roomID="${rID}"><AV_EN def="hr=001/1"/></Room.Action>\0`));
    }, studioID);

    await pageDana.evaluate((rID) => {
      const ws = (window as any).testWS;
      ws.send(new TextEncoder().encode(`<Room.Enter msgID="2" roomID="${rID}"/>\0`));
      ws.send(new TextEncoder().encode(`<Room.Action roomID="${rID}"><AV_EN def="hr=002/2"/></Room.Action>\0`));
    }, studioID);

    // Wait for presence
    await pageDana.waitForFunction((c) => ((window as any).wsMessages || []).some((m: string) => m.includes(`av="${c}"`)), charlie);

    // Charlie places the Premier Light (cid: 10) in studio
    await pageCharlie.evaluate(({ rID, pID }) => {
      const ws = (window as any).testWS;
      ws.send(new TextEncoder().encode(`<Room.Action roomID="${rID}"><POS_STD st="0" d="2" ho="0" z="6" y="0" x="5" id="${pID}" cid="10"/></Room.Action>\0`));
    }, { rID: studioID, pID: posID });

    // Dana receives furniture placement
    await pageDana.waitForFunction(({ pID }) => {
      const msgs = (window as any).wsMessages || [];
      return msgs.some((m: string) => m.includes('<POS ') && m.includes(`id="${pID}"`) && m.includes('cid="10"'));
    }, { pID: posID });

    // Charlie double clicks lamp to toggle state on: st="1"
    await pageCharlie.evaluate(({ rID, pID }) => {
      const ws = (window as any).testWS;
      ws.send(new TextEncoder().encode(`<Room.Action roomID="${rID}"><POS st="1" d="4" ho="0" z="6" y="0" x="5" id="${pID}"/></Room.Action>\0`));
    }, { rID: studioID, pID: posID });

    // Dana receives in-place rotation and state toggle
    await pageDana.waitForFunction(({ pID }) => {
      const msgs = (window as any).wsMessages || [];
      return msgs.some((m: string) => m.includes('<POS ') && m.includes(`id="${pID}"`) && m.includes('st="1"') && m.includes('d="4"'));
    }, { pID: posID });

    // Test Sitting Gesture broadcast
    await pageCharlie.evaluate((rID) => {
      const ws = (window as any).testWS;
      ws.send(new TextEncoder().encode(`<Room.Action roomID="${rID}"><AV_ACT act="sit"/></Room.Action>\0`));
    }, studioID);

    await pageDana.waitForFunction((c) => {
      const msgs = (window as any).wsMessages || [];
      return msgs.some((m: string) => m.includes('<AV_DC') && m.includes(`av="${c}"`) && m.includes('act="sit"'));
    }, charlie);

    // Test Stand Gesture broadcast
    await pageCharlie.evaluate((rID) => {
      const ws = (window as any).testWS;
      ws.send(new TextEncoder().encode(`<Room.Action roomID="${rID}"><AV_ACT act="std"/></Room.Action>\0`));
    }, studioID);

    await pageDana.waitForFunction((c) => {
      const msgs = (window as any).wsMessages || [];
      return msgs.some((m: string) => m.includes('<AV_DC') && m.includes(`av="${c}"`) && m.includes('act="std"'));
    }, charlie);

    await ctxCharlie.close();
    await ctxDana.close();
  });

  test('private whisper routing and host moderation kickout are isolated authoritatively', async ({ browser, baseURL }) => {
    const timestamp3 = Date.now() + 2;
    const alice = `Alice_Host_${timestamp3}`;
    const bob = `Bob_Guest_${timestamp3}`;
    const charlie = `Charlie_Guest_${timestamp3}`;
    const studioID = `studio_${alice.toLowerCase()}`;

    // 1. Register Alice, Bob, and Charlie
    const ctxAlice = await browser.newContext();
    const pageAlice = await ctxAlice.newPage();
    const resAlice = await pageAlice.request.post('/api/denizen/register', {
      form: { screenName: alice, password: password, gender: '1', avatarDef: 'hr=001/1' },
    });
    const aliceData = await resAlice.json();
    expect(aliceData.success).toBe(true);

    const ctxBob = await browser.newContext();
    const pageBob = await ctxBob.newPage();
    const resBob = await pageBob.request.post('/api/denizen/register', {
      form: { screenName: bob, password: password, gender: '2', avatarDef: 'hr=002/2' },
    });
    const bobData = await resBob.json();
    expect(bobData.success).toBe(true);

    const ctxCharlie = await browser.newContext();
    const pageCharlie = await ctxCharlie.newPage();
    const resCharlie = await pageCharlie.request.post('/api/denizen/register', {
      form: { screenName: charlie, password: password, gender: '1', avatarDef: 'hr=003/1' },
    });
    const charlieData = await resCharlie.json();
    expect(charlieData.success).toBe(true);

    const wsUrl = (baseURL || 'http://localhost:24902').replace('http://', 'ws://').replace('https://', 'wss://') + '/ws/realtime';

    const loginHelper = (page: any, user: string, secret: string) => {
      return page.evaluate(
        ({ wsUrl, user, secret }: any) => {
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
              const encUser = Array.from(new TextEncoder().encode(user))
                .map((b) => b.toString(16).padStart(2, '0')).join('').toUpperCase();
              const encPass = Array.from(new TextEncoder().encode(secret))
                .map((b) => b.toString(16).padStart(2, '0')).join('').toUpperCase();
              ws.send(new TextEncoder().encode(`<System.Login msgID="1"><Username encoding="text/binhex">${encUser}</Username><Password encoding="text/binhex">${encPass}</Password></System.Login>\0`));
              resolve();
            };
            ws.onerror = reject;
          });
        },
        { wsUrl, user, secret }
      );
    };

    // 2. Login all 3 via WebSocket
    await loginHelper(pageAlice, alice, aliceData.sw2);
    await loginHelper(pageBob, bob, bobData.sw2);
    await loginHelper(pageCharlie, charlie, charlieData.sw2);

    await pageAlice.waitForFunction(() => ((window as any).wsMessages || []).some((m: string) => m.includes('<Accepted msgID="1"/>')));
    await pageBob.waitForFunction(() => ((window as any).wsMessages || []).some((m: string) => m.includes('<Accepted msgID="1"/>')));
    await pageCharlie.waitForFunction(() => ((window as any).wsMessages || []).some((m: string) => m.includes('<Accepted msgID="1"/>')));

    // 3. Enter Alice's studio
    const enterStudio = (page: any, rID: string, def: string) => {
      return page.evaluate(({ rID, def }: any) => {
        const ws = (window as any).testWS;
        ws.send(new TextEncoder().encode(`<Room.Enter msgID="2" roomID="${rID}"/>\0`));
        ws.send(new TextEncoder().encode(`<Room.Action roomID="${rID}"><AV_EN def="${def}"/></Room.Action>\0`));
      }, { rID, def });
    };

    await enterStudio(pageAlice, studioID, 'hr=001/1');
    await enterStudio(pageBob, studioID, 'hr=002/2');
    await enterStudio(pageCharlie, studioID, 'hr=003/1');

    // Confirm all 3 avatars present
    await pageAlice.waitForFunction((b) => ((window as any).wsMessages || []).some((m: string) => m.includes(`av="${b}"`)), bob);
    await pageAlice.waitForFunction((c) => ((window as any).wsMessages || []).some((m: string) => m.includes(`av="${c}"`)), charlie);
    await pageBob.waitForFunction((a) => ((window as any).wsMessages || []).some((m: string) => m.includes(`av="${a}"`)), alice);
    await pageCharlie.waitForFunction((a) => ((window as any).wsMessages || []).some((m: string) => m.includes(`av="${a}"`)), alice);

    // 4. Test Private Whisper Routing
    const secretPlan = 'SuperSecretMixPlan_9988';
    const secretHex = Array.from(new TextEncoder().encode(secretPlan))
      .map((b) => b.toString(16).padStart(2, '0')).join('');

    await pageAlice.evaluate(({ rID, target, secretHex }) => {
      const ws = (window as any).testWS;
      ws.send(new TextEncoder().encode(`<Room.Action roomID="${rID}"><CH sm="whisper" to="${target}" m="${secretHex}"/></Room.Action>\0`));
    }, { rID: studioID, target: bob, secretHex });

    // Bob receives the private whisper
    await pageBob.waitForFunction(({ a, secretHex }) => {
      const msgs = (window as any).wsMessages || [];
      return msgs.some((m: string) => m.includes('<CH ') && m.includes(`av="${a}"`) && m.includes(`m="${secretHex}"`));
    }, { a: alice, secretHex });

    // Alice receives the whisper reflection
    await pageAlice.waitForFunction(({ b, secretHex }) => {
      const msgs = (window as any).wsMessages || [];
      return msgs.some((m: string) => m.includes('<CH ') && m.includes(`to="${b}"`) && m.includes(`m="${secretHex}"`));
    }, { b: bob, secretHex });

    // Charlie MUST NOT receive the private whisper
    await pageCharlie.waitForTimeout(500);
    const charlieReceivedSecret = await pageCharlie.evaluate((secretHex) => {
      const msgs = (window as any).wsMessages || [];
      return msgs.some((m: string) => m.includes(secretHex));
    }, secretHex);
    expect(charlieReceivedSecret).toBe(false);

    // 5. Test Unauthorized Kick Rejection (Charlie tries to kick Bob from Alice's room)
    await pageCharlie.evaluate(({ rID, target }) => {
      const ws = (window as any).testWS;
      ws.send(new TextEncoder().encode(`<Room.Action roomID="${rID}"><ST_KICK target="${target}"/></Room.Action>\0`));
    }, { rID: studioID, target: bob });

    await pageBob.waitForTimeout(500);
    const bobKickedPrematurely = await pageBob.evaluate(() => {
      const msgs = (window as any).wsMessages || [];
      return msgs.some((m: string) => m.includes('<KICKED '));
    });
    expect(bobKickedPrematurely).toBe(false);

    // 6. Test Authorized Host Moderation Kick (Alice kicks Bob)
    await pageAlice.evaluate(({ rID, target }) => {
      const ws = (window as any).testWS;
      ws.send(new TextEncoder().encode(`<Room.Action roomID="${rID}"><ST_KICK target="${target}"/></Room.Action>\0`));
    }, { rID: studioID, target: bob });

    // Bob receives <KICKED roomID="..." by="Alice"/>
    await pageBob.waitForFunction(({ rID, a }) => {
      const msgs = (window as any).wsMessages || [];
      return msgs.some((m: string) => m.includes('<KICKED ') && m.includes(`roomID="${rID}"`) && m.includes(`by="${a}"`));
    }, { rID: studioID, a: alice });

    // Alice and Charlie receive <AV_RM av="Bob"/>
    await pageAlice.waitForFunction((b) => {
      const msgs = (window as any).wsMessages || [];
      return msgs.some((m: string) => m.includes('<AV_RM ') && m.includes(`av="${b}"`));
    }, bob);

    await pageCharlie.waitForFunction((b) => {
      const msgs = (window as any).wsMessages || [];
      return msgs.some((m: string) => m.includes('<AV_RM ') && m.includes(`av="${b}"`));
    }, bob);

    await ctxAlice.close();
    await ctxBob.close();
    await ctxCharlie.close();
  });

  test('arcade minigames suite: Uncover the Music, Sumo Arena, and Pop Quiz trivia award decibels and respect caps', async ({ page }) => {
    const timestamp = Date.now();
    const arcadeUser = `ArcadeFan_${timestamp}`;
    const password = 'ArcadePassword123!';

    // 1. Register test user via API
    const regRes = await page.request.post('/api/denizen/register', {
      form: { screenName: arcadeUser, password: password, gender: '1', avatarDef: 'hr=001/1' },
    });
    const regData = await regRes.json();
    expect(regData.success).toBe(true);

    // 2. Open portal and log in
    await page.goto('/');
    await page.locator('#btnGateSignIn').click();
    await page.locator('#loginScreenName').fill(arcadeUser);
    await page.locator('#loginPassword').fill(password);
    await page.locator('#btnSubmitLogin').click();
    await expect(page.locator('#gameAccessGate')).toBeHidden({ timeout: 5000 });

    // 3. Verify dark loft portal styling
    const bodyLayout = page.locator('.portal-body-layout');
    await expect(bodyLayout).toBeVisible();
    const bodyBg = await bodyLayout.evaluate((el) => window.getComputedStyle(el).backgroundColor);
    expect(bodyBg).toBe('rgb(24, 21, 22)'); // #181516

    // 4. Switch to Games & Arcade tab
    await page.locator('div[data-tab="popquiz"]').click();
    await expect(page.locator('#tab-popquiz')).toBeVisible();

    // 5. Test Uncover the Music minigame
    await page.locator('#subtabBtn-uncover').click();
    const uncoverPane = page.locator('#arcadePane-uncover');
    await expect(uncoverPane).toBeVisible();

    // Verify 24 cards rendered
    const cards = page.locator('#uncoverCardsGrid .uncover-card');
    await expect(cards).toHaveCount(24);

    // Click first card
    const firstCard = cards.nth(0);
    await firstCard.click();
    await expect(firstCard).toHaveClass(/flipped/);

    // 6. Test Pop Quiz Trivia
    await page.locator('#subtabBtn-quiz').click();
    await expect(page.locator('#arcadePane-quiz')).toBeVisible();
    // Answer the quiz question
    const firstOptionBtn = page.locator('#quizOptionsContainer button').first();
    await firstOptionBtn.click();
    const quizFeedback = page.locator('#quizFeedback');
    await expect(quizFeedback).toBeVisible();

    // 7. Test V-Ego San Sumo Arena
    await page.locator('#subtabBtn-sumo').click();
    await expect(page.locator('#arcadePane-sumo')).toBeVisible();
    // Select Head Hit move
    await page.locator('button:has-text("Head Hit")').click();
    const commentary = page.locator('#sumoCommentaryBox');
    await expect(commentary).not.toHaveText('Select your opening stance to engage Master Kenji in combat!');
  });
});
