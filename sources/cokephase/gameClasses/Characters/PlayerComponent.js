/**
 * Adds mouse control to the entity this component is added to.
 * @type {IgeClass}
 */
var PlayerComponent = IgeClass.extend({
	classId: 'PlayerComponent',
	componentId: 'player',
	targetPos: { x: null, y: null },
	currentPos: { x: null, y: null },
	init: function (entity, options) {
		var self = this;

		window.player = this;

		// Store the entity that this component has been added to
		this._entity = entity;

		// Set starting coordinates
		this.currentPos = this._entity.currentPos;

		// Store options
		this._options = options;

		// Listen for mouse click
		this._mouseDownEvent = ige.input.on('mouseDown', function () { self._mouseDown(); });

		// Listen for key event
		this._keyUpEvent = ige.input.on('keyUp', function (event, keyCode) { self._keyUp(event, keyCode); });

		// Listen for path events
		entity.path.on('pointComplete', function (sender, x, y) { self._pointReached(sender, x, y); });
		entity.path.on('pathComplete', function () { self._pathComplete(); });
		entity.path.on('started', function () { self._pathStarted(); });
		entity.path.on('dynamicFail', function() { self._pathHalt(); });
	},

	_mouseDown: function () {
		if (ige.movingItem === true) {
			return false;
		}

		if (this._entity._alive === false) {
			ige.input.off('mouseDown', this._mouseDownEvent);
		}

		// Get tile coordinates under mouse
		var endTile = ige.room.tileMap().mouseToTile(),
			overTiles;

		// Check bounds
		if (ige.client.withinBounds(endTile.x, endTile.y) === false) {
			if (endTile.x !== ige.room.playerStartCords().x && endTile.y !== ige.room.playerStartCords().y) {
				return false;
			}
		}

		overTiles = this._entity.overTiles()[0];

		// If already headed here, do not re-trigger
		if (this.targetPos.x === endTile.x && this.targetPos.y === endTile.y) {
			return;
		}

		this.targetPos.x = endTile.x;
		this.targetPos.y = endTile.y;

		this._entity.path
			.set(overTiles.x, overTiles.y, 0, this.targetPos.x, this.targetPos.y, 0)
			.speed(1.75)
			.start();

		// Notify realtime WebSocket server of movement
		if (ige.client && ige.client.ws && ige.client.ws.readyState === WebSocket.OPEN) {
			var mvXML = '<AV_MV av="' + (ige.client.currentUserName || 'Adam') + '" x="' + endTile.x + '" z="' + endTile.y + '"/>';
			ige.client.ws.send(mvXML);
		}
	},

	_keyUp: function (event, keyCode) {
		if (typeof this._entity === 'undefined') {
			return false;
		}
	},

	_pointReached: function(sender, x, y) {
		if (typeof this._entity === 'undefined') {
			return false;
		}

		var direction = this._entity.path.getDirection();
		if (direction !== '' && direction !== this._entity._currentDirection) {
			this._entity.changeDirection(direction);
			this._entity.changeAnimation('walk');
		}
	},

	_pathComplete: function() {
		if (typeof this._entity === 'undefined') {
			return false;
		}

		// Check if it's a seat
		if (ige.room.tileMap().isTileOccupied(this.targetPos.x, this.targetPos.y)) {
			var occupying = ige.client.itemAt(this.targetPos.x, this.targetPos.y, true);

			if (occupying && occupying.data('seat') === true) {
				occupying.beingUsed(true, this._entity);
				this._entity.sit(occupying);
			}
		} else {
			this._entity.rest();
		}

		// Check if leaving
		if (this.targetPos.x === ige.room.playerStartCords().x && this.targetPos.y === ige.room.playerStartCords().y) {
			ige.navigation.showMainMenu();
			return;
		}

		this.currentPos = { x: this.targetPos.x, y: this.targetPos.y };
		this._entity.currentPos = this.currentPos;
	},

	_pathStarted: function() {
		if (typeof this._entity === 'undefined') {
			return false;
		}

		this._entity.layer(0);
		
		var direction = this._entity.path.getDirection();
		if (direction !== '') {
			this._entity.changeDirection(direction);
			this._entity.changeAnimation('walk');

			if (!ige.overItem) $('#infostand').hide();
		}
	},

	_pathHalt: function() {
		if (typeof this._entity === 'undefined') {
			return false;
		}

		this._entity.animation.stop();
		this._entity.rest();
	}
});

if (typeof(module) !== 'undefined' && typeof(module.exports) !== 'undefined') { module.exports = PlayerComponent; }
