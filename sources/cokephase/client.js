var c = Math.cos(-90),
	s = Math.sin(-90);

var trans = [
	c,  -s,  0,
	s + .225,  c, 0, 
	0,  0,  1
];

var Client = IgeClass.extend({
	classId: 'Client',
	init: function () {
		ige.showStats(0);
		ige.input.debug(false);
		ige.globalSmoothing(true);

		var self = this;
		this.gameItems = [];
		this.remotePlayers = {};
		this.decibels = 100;
		this.currentRoomId = 'studio_model_b';
		this.currentUserName = 'Adam';

		// Parse URL query parameters
		var urlParams = new URLSearchParams(window.location.search);
		if (urlParams.has('user') && urlParams.get('user').trim() !== '') {
			this.currentUserName = urlParams.get('user').trim();
		}
		if (urlParams.has('room') && urlParams.get('room').trim() !== '') {
			var rParam = urlParams.get('room').trim();
			var roomMap = {
				'1': 'studio_model_a',
				'2': 'studio_model_b',
				'3': 'studio_model_c',
				'4': 'studio_model_d',
				'5': 'studio_model_rooftop',
				'6': 'studio_wayne'
			};
			this.currentRoomId = roomMap[rParam] || rParam;
		}

		// Load textures
		ige.gameTexture = {};
		ige.gameTexture.carpetTest = new IgeCellSheet(rootPath + 'assets/textures/tiles/carpet-test-4.png', 1, 1);
		ige.gameTexture.purpleTile = new IgeCellSheet(rootPath + 'assets/textures/tiles/floor-tiles.png', 3, 1);

		ige.gameTexture.studio_model_a = new IgeTexture(rootPath + 'assets/rooms/studio_model_a.png');
		ige.gameTexture.studio_model_b = new IgeTexture(rootPath + 'assets/rooms/studio_model_b_2.png');
		ige.gameTexture.studio_model_c = new IgeTexture(rootPath + 'assets/rooms/studio_model_c_1.png');
		ige.gameTexture.studio_model_d = new IgeTexture(rootPath + 'assets/rooms/studio_model_d.png');
		ige.gameTexture.studio_model_rooftop = new IgeTexture(rootPath + 'assets/rooms/Studio_Rooftop_with_Grid_256.png');
		ige.gameTexture.studio_wayne = new IgeTexture(rootPath + 'assets/rooms/wayne_ent_4.png');
		ige.gameTexture.pokemon = new IgeTexture(rootPath + 'assets/rooms/pokemon.jpg');
		ige.gameTexture.entry_top = new IgeTexture(rootPath + 'assets/rooms/wall_doormask.png');
		ige.gameTexture.entry_side = new IgeTexture(rootPath + 'assets/rooms/wall_doormask_side.png');
		
		ige.gameTexture.leftWall = new IgeTexture(rootPath + 'assets/textures/left_wall_1_a_0_0_0.png');
		ige.gameTexture.rightWall = new IgeTexture(rootPath + 'assets/textures/right_wall_1_a_0_2_0.png');
		ige.gameTexture.pokemon_house = new IgeTexture(rootPath + 'assets/rooms/pokemon_house.png');
		ige.gameTexture.pokemon_house2 = new IgeTexture(rootPath + 'assets/rooms/pokemon_house.png');

		ige.gameTexture.people = new TexturePackerAtlas('PEOPLE', rootPath + 'assets/character/data.png', rootPath + 'assets/character/data.js');
		ige.gameTexture.people.hair = new TexturePackerAtlas('PEOPLE', rootPath + 'assets/character/data.png', rootPath + 'assets/character/data.js');

		// Furniture
		ige.gameTexture.furniture = new IgeSpriteSheet(rootPath + 'assets/furniture.png');

		ige.addComponent(IgeEditorComponent);
		
		// Create the HTML canvas
		ige.createFrontBuffer(true);

		// Wait for textures to load before continuing
		ige.on('texturesLoaded', function () {
			ige.start(function (success) {
				if (success) {
					// Hide loading screen
					$('.igeLoading').hide();

					// Initialize navigation
					ige.navigation = Navigation.init();

					// Load authentic room immediately with player avatar
					self.loadRoom(self.currentRoomId, self.currentUserName);

					// Connect realtime WebSocket
					self.connectRealtime();

					// Hook up toolbar actions
					self.initToolbarEvents();
				}
			});
		});
	},

	loadRoom: function (roomType, owner) {
		var self = this;
		this.currentRoomId = roomType;

		// Clean up existing room and entities
		if (ige.room && ige.room._gameScene) {
			try { ige.room._gameScene.hide(); } catch (_) {}
		}
		this.gameItems = [];
		this.remotePlayers = {};

		ige.removeGraph();
		ige.addGraph('IgeBaseScene');

		// Hide entry splash and show bottom toolbar
		$('#entry').hide();
		$('.bottom-bar').show();
		$('#navigation_close').show();

		// Instantiate and render the isometric studio
		var playerStudio = new PlayerStudio()
			.type(roomType)
			.owner(owner || self.currentUserName)
			.render();
		ige.room = playerStudio;

		// Instantiate and spawn the local player character
		ige.player = new Character()
			.id('player')
			.setStyle('001')
			.setHeadStyle('001')
			.setHairStyle('013')
			.setEyeStyle('001')
			.setMouthStyle('001')
			.setLeftSleveStyle('001')
			.setRightSleveStyle('001')
			.setShirtStyle('001')
			.setPantStyle('001')
			.setShoeStyle('001')
			.startPlayer();

		// Populate authentic room furniture
		self.spawnDefaultFurniture(roomType);

		// Notify realtime server
		if (self.ws && self.ws.readyState === WebSocket.OPEN) {
			var enterXML = '<Room.Enter roomID="' + roomType + '" msgID="' + Date.now() + '"/>';
			self.ws.send(enterXML);
			var spawnXML = '<AV_EN av="' + self.currentUserName + '"/>';
			self.ws.send(spawnXML);
		}

		console.log('[CokePhase] Loaded room: ' + roomType + ' for user: ' + self.currentUserName);
	},

	spawnDefaultFurniture: function (roomType) {
		var self = this;
		try {
			if (roomType === 'studio_model_b') {
				self.gameItems.push(new GameItem('mini_coke_fridge', 'NE', 1, 6));
				self.gameItems.push(new GameItem('pinball', 'NW', 1, 1));
				self.gameItems.push(new GameItem('sofa_coke', 'SW', 6, 1));
				self.gameItems.push(new GameItem('v_coke_table', 'SE', 3, 7));
				self.gameItems.push(new GameItem('n_stool', 'SE', 4, 7));
				self.gameItems.push(new GameItem('standing_lamp', 'SE', 1, 8));
			} else if (roomType === 'studio_model_a') {
				self.gameItems.push(new GameItem('mini_coke_fridge', 'NE', 1, 4));
				self.gameItems.push(new GameItem('sofa_coke', 'SW', 3, 2));
				self.gameItems.push(new GameItem('n_stereo', 'SE', 1, 1));
			} else if (roomType === 'studio_model_c') {
				self.gameItems.push(new GameItem('mini_coke_fridge', 'NE', 1, 3));
				self.gameItems.push(new GameItem('sofa_coke', 'SW', 3, 1));
			} else {
				self.gameItems.push(new GameItem('mini_coke_fridge', 'NE', 1, 4));
				self.gameItems.push(new GameItem('sofa_coke', 'SW', 4, 2));
			}
		} catch (err) {
			console.warn('[CokePhase] Spawn furniture error:', err);
		}
	},

	connectRealtime: function () {
		var self = this;
		var protocol = window.location.protocol === 'https:' ? 'wss:' : 'ws:';
		var wsHost = window.location.host;
		var wsURL = protocol + '//' + wsHost + '/ws/realtime';

		try {
			self.ws = new WebSocket(wsURL);

			self.ws.onopen = function () {
				console.log('[CokePhase WS] Connected to ' + wsURL);
				var loginXML = '<Login username="' + self.currentUserName + '" msgID="1"/>';
				self.ws.send(loginXML);

				var enterXML = '<Room.Enter roomID="' + self.currentRoomId + '" msgID="2"/>';
				self.ws.send(enterXML);

				var spawnXML = '<AV_EN av="' + self.currentUserName + '"/>';
				self.ws.send(spawnXML);
			};

			self.ws.onmessage = function (event) {
				self.handleRealtimeMessage(event.data);
			};

			self.ws.onerror = function (err) {
				console.warn('[CokePhase WS] Error:', err);
			};

			self.ws.onclose = function () {
				console.log('[CokePhase WS] Disconnected. Reconnecting in 3s...');
				setTimeout(function () {
					self.connectRealtime();
				}, 3000);
			};
		} catch (e) {
			console.warn('[CokePhase WS] Connection setup failed:', e);
		}
	},

	handleRealtimeMessage: function (data) {
		var self = this;
		if (typeof data !== 'string') return;

		// Chat message
		if (data.indexOf('<CH ') !== -1) {
			var avMatch = data.match(/av="([^"]+)"/);
			var mMatch = data.match(/m="([^"]+)"/);
			if (avMatch && mMatch) {
				var av = avMatch[1];
				var hexStr = mMatch[1];
				var text = '';
				try {
					for (var i = 0; i < hexStr.length; i += 2) {
						text += String.fromCharCode(parseInt(hexStr.substr(i, 2), 16));
					}
				} catch (_) {
					text = hexStr;
				}
				if (typeof Chat !== 'undefined' && typeof Chat.create === 'function') {
					Chat.create(av, text, 2);
				}
			}
			return;
		}

		// Coke drink dispenser response
		if (data.indexOf('<AV_DC') !== -1 && data.indexOf('act="drk"') !== -1) {
			var drkAvMatch = data.match(/av="([^"]+)"/);
			var drkAv = drkAvMatch ? drkAvMatch[1] : '';
			if (drkAv === self.currentUserName && ige.player) {
				ige.player.drinkStart();
				setTimeout(function () {
					if (ige.player) ige.player.drinkStop();
				}, 3000);
				self.decibels += 10;
				self.showNotification('+10 dB! Refreshing Coca-Cola!');
				self.updateDecibelsDisplay();
			}
			return;
		}

		// Remote avatar movement
		if (data.indexOf('<AV_MV') !== -1) {
			var mvAvMatch = data.match(/av="([^"]+)"/);
			var xMatch = data.match(/x="([^"]+)"/);
			var zMatch = data.match(/z="([^"]+)"/);
			if (mvAvMatch && xMatch && zMatch) {
				var mvAv = mvAvMatch[1];
				var tx = parseInt(xMatch[1], 10);
				var tz = parseInt(zMatch[1], 10);
				if (mvAv !== self.currentUserName && self.remotePlayers[mvAv]) {
					self.remotePlayers[mvAv].walkTo(tx, tz);
				}
			}
			return;
		}

		// Remote avatar spawn
		if (data.indexOf('<AV ') !== -1) {
			var spAvMatch = data.match(/av="([^"]+)"/);
			if (spAvMatch) {
				var spAv = spAvMatch[1];
				if (spAv !== self.currentUserName && !self.remotePlayers[spAv]) {
					try {
						var rem = new Character()
							.id('remote_' + spAv)
							.setStyle('001')
							.setHeadStyle('001')
							.setHairStyle('013')
							.setEyeStyle('001')
							.setMouthStyle('001')
							.setShirtStyle('001')
							.setPantStyle('001')
							.setShoeStyle('001')
							.startPlayer();
						self.remotePlayers[spAv] = rem;
					} catch (_) {}
				}
			}
			return;
		}

		// Performance cheer / boo
		if (data.indexOf('<CD_TU') !== -1) {
			self.showNotification('♫ Cheer! (+5 dB to DJ)');
			return;
		}
		if (data.indexOf('<CD_TD') !== -1) {
			self.showNotification('Boo! Performance voted down');
			return;
		}
	},

	drinkCoke: function () {
		var self = this;
		if (ige.player) {
			ige.player.drinkStart();
			setTimeout(function () {
				if (ige.player) ige.player.drinkStop();
			}, 3000);
		}
		if (self.ws && self.ws.readyState === WebSocket.OPEN) {
			self.ws.send('<DRK_REQ item="coke_can"/>');
		} else {
			self.decibels += 10;
			self.showNotification('+10 dB! Refreshing Coca-Cola!');
			self.updateDecibelsDisplay();
		}
	},

	triggerDJPerformance: function () {
		var self = this;
		if (self.ws && self.ws.readyState === WebSocket.OPEN) {
			self.ws.send('<CD_P title="Coke Studios Live Mix" s="^50,rockOne_guitar00{2,0{4,4{^50,danceOne_beatz00{4,0{^"/>');
		}
		self.showNotification('DJ is performing! Vote in the Infostand!');
		$('#infostand .title').text('Live DJ Performance');
		$('#infostand .description').text('Now playing: Coke Studios Live Mix. Vote Cheers or Boos!');
		$('#infostand').show();
	},

	voteCheer: function () {
		var self = this;
		if (self.ws && self.ws.readyState === WebSocket.OPEN) {
			self.ws.send('<CD_TU/>');
		}
		self.showNotification('Voted Cheers! (+5 dB)');
	},

	voteBoo: function () {
		var self = this;
		if (self.ws && self.ws.readyState === WebSocket.OPEN) {
			self.ws.send('<CD_TD/>');
		}
		self.showNotification('Voted Boo!');
	},

	updateDecibelsDisplay: function () {
		$('#decibals').attr('title', 'Decibels: ' + this.decibels + ' dB');
	},

	showNotification: function (text) {
		var $notif = $('<div class="coke-game-bubble">' + text + '</div>');
		$notif.css({
			position: 'fixed',
			top: '80px',
			left: '50%',
			transform: 'translateX(-50%)',
			background: '#e41e2b',
			color: '#fff',
			padding: '8px 18px',
			borderRadius: '20px',
			border: '2px solid #ffcc00',
			fontWeight: 'bold',
			fontFamily: 'Tahoma, sans-serif',
			fontSize: '14px',
			boxShadow: '0 4px 12px rgba(0,0,0,0.5)',
			zIndex: 99999,
			pointerEvents: 'none'
		});
		$('body').append($notif);
		setTimeout(function () {
			$notif.fadeOut(500, function () { $notif.remove(); });
		}, 2500);
	},

	initToolbarEvents: function () {
		var self = this;

		$('#decibals').off('click').on('click', function (e) {
			e.preventDefault();
			self.showNotification('Decibels Wallet: ' + self.decibels + ' dB');
		});

		$('#music').off('click').on('click', function (e) {
			e.preventDefault();
			self.triggerDJPerformance();
		});

		// Infostand button hooks
		$('#itemRotate').off('click').on('click', function () {
			if (ige.selected && typeof ige.selected.rotate === 'function') {
				ige.selected.rotate();
			}
		});

		$('#itemPickup').off('click').on('click', function () {
			if (ige.selected && typeof ige.selected.pickup === 'function') {
				ige.selected.pickup();
			}
		});
	},

	itemAt: function (tileX, tileY, topMost) {
		if (!ige.room || !ige.room.tileMap()) return undefined;
		var tileData = ige.room.tileMap().map.tileData(tileX, tileY);
		if (typeof tileData !== 'undefined' && tileData.constructor === Array && topMost) {
			return tileData[tileData.length - 1];
		}
		return tileData;
	},

	getTexture: function(tAction, tPart, tStyle, tDirection, tSubsection) {
		return 'h_' + tAction + '_' + tPart + '_' + tStyle + '_' + tDirection + '_' + tSubsection + '.png';
	},

	withinBounds: function(tileX, tileY) {
		if (tileX < 0 || tileY < 0) return false;
		if (!ige.room || !ige.room.object) return false;
		if (tileX >= ige.room.object['width'] || tileY >= ige.room.object['height']) return false;

		var itemAt = this.itemAt(tileX, tileY, true);
		if (typeof itemAt !== 'undefined' && itemAt._classId === 'InvisibleBlock') {
			return false;
		}
		return true;
	}
});

if (typeof(module) !== 'undefined' && typeof(module.exports) !== 'undefined') { module.exports = Client; }
