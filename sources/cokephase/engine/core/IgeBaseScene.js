/**
 * When loaded into memory using ige.addGraph('IgeBaseScene') will create
 * the scene "baseScene" and the viewport "vp1" that are used in almost all
 * examples and can be used as the base for your scenegraph as well.
 */
var IgeBaseScene = IgeSceneGraph.extend({
	classId: 'IgeBaseScene',
	
	init: function () {},

	/**
	 * Called when loading the graph data via ige.addGraph().
	 * @param options
	 */
	addGraph: function (options) {
		// Clear existing graph data
		if (ige.$('baseScene')) {
			this.removeGraph();
		}
		
		// Create the scene
		var scene1 = new IgeScene2d()
			.id('scene1')
			.ignoreCamera(true);

		// Create the scene
		var baseScene = new IgeScene2d()
			.id('baseScene');

		// Create the main viewport to look at "baseScene"
		new IgeViewport()
			.id('vp1')
			.autoSize(true)
			.scene(baseScene)
			.drawBounds(false)
			.mount(ige);
	},

	/**
	 * The method called when the graph items are to be removed from the
	 * active graph.
	 */
	removeGraph: function () {
		try { ige.input.clearEvent('mouseDown'); } catch (_) {}
		try { ige.input.clearEvent('mouseUp'); } catch (_) {}
		try { ige.input.clearEvent('keyUp'); } catch (_) {}
		var tm = ige.$('tileMap1');
		if (tm && tm._inventoryListener && typeof tm._inventoryListener.off === 'function') {
			try { tm._inventoryListener.off(); } catch (_) {}
		}

		if (ige.player) {
			try { ige.player.removeComponent('player'); } catch (_) {}
			try { ige.player.removeComponent('animation'); } catch (_) {}
			try { ige.player.removeComponent('path'); } catch (_) {}
		}

		var ids = ['background', 'textureMap', 'tileMap1', 'objectScene', 'gameScene', 'scene1', 'vp1', 'baseScene'];
		for (var i = 0; i < ids.length; i++) {
			var ent = ige.$(ids[i]);
			if (ent && typeof ent.destroy === 'function') {
				try { ent.destroy(); } catch (_) {}
			}
		}
	}
});

if (typeof(module) !== 'undefined' && typeof(module.exports) !== 'undefined') { module.exports = IgeBaseScene; }
