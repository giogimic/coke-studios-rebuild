var Navigation = {
    init: function() {
    	$('.nav-name, .nav-go').on('click', function(e) {
    		e.preventDefault();
	    	var listItem = $(this).closest('li');
			var studioId = $(listItem).data('room-id') || 2;

			var fallbackMap = {
				1: { room_type: 'studio_model_a', room_owner: 'Coke Studios', room_name: 'Studio #1' },
				2: { room_type: 'studio_model_b', room_owner: 'Coke Studios', room_name: 'Studio #2' },
				3: { room_type: 'studio_model_c', room_owner: 'Coke Studios', room_name: 'Studio #3' },
				4: { room_type: 'studio_model_d', room_owner: 'Coke Studios', room_name: 'Studio #4' },
				5: { room_type: 'studio_model_rooftop', room_owner: 'Coke Studios', room_name: 'Studio Rooftop' },
				6: { room_type: 'studio_wayne', room_owner: 'Wayne Enterprise', room_name: 'Wayne Ent.' }
			};

			function applyRoom(data) {
				$('#entry').hide();
				$('.bottom-bar').show();
				$('#navigation-ui').hide();
				$('#navigation').removeClass('active');

				if (ige.client && typeof ige.client.loadRoom === 'function') {
					ige.client.loadRoom(data.room_type, data.room_owner);
				}
			}

			$.ajax({
				dataType: "json",
				method: "POST",
				url: "/jsonreply/room.php",
				data: { id: studioId },
				timeout: 2000
			})
			.done(function(data) {
				if (data && data.room_type) {
					applyRoom(data);
				} else {
					applyRoom(fallbackMap[studioId] || fallbackMap[2]);
				}
			})
			.fail(function() {
				applyRoom(fallbackMap[studioId] || fallbackMap[2]);
			});
    	});

    	return this;
    },

    showMainMenu: function() {
    	this.closeAllUI();
    	if (ige.room && ige.room._gameScene) {
    		try { ige.room._gameScene.hide(); } catch (_) {}
    	}
		ige.removeGraph();
		$('#entry').show();
		toggle_popup('#navigation-ui');
		return this;
    },

    closeAllUI: function() {
    	$('.bottom-bar').hide();
    	$('#navigation_close').hide();
    	if (typeof close_inventory_ui === 'function') {
    		close_inventory_ui();
    	}
    }
};

if (typeof(module) !== 'undefined' && typeof(module.exports) !== 'undefined') { module.exports = Navigation; }
