var Chat = {
    init: function() {
        // Start shuffling bubbles
        setInterval(function() { Chat.shuffleBubbles(); }, this.getSettings().pushTime);

        // Start listening for chat input
        $("#chat-input").off('keyup').on('keyup', function(e) {
            if (e.which != 13) return;

            e.preventDefault();

            var message = $("#chat-input").val().trim();
            var user = (ige.client && ige.client.currentUserName) ? ige.client.currentUserName : (new URLSearchParams(window.location.search).get('user') || 'Adam');

            if (message != "" && ! Chat.handleAsLocalCommand(message)) {
                Chat.create(user, message, null);

                // Broadcast to Go realtime server
                if (ige.client && ige.client.ws && ige.client.ws.readyState === WebSocket.OPEN) {
                    var hex = "";
                    for (var i = 0; i < message.length; i++) {
                        var h = message.charCodeAt(i).toString(16);
                        if (h.length < 2) h = "0" + h;
                        hex += h;
                    }
                    var chatXML = '<CH av="' + user + '" sm="1" m="' + hex + '"/>';
                    ige.client.ws.send(chatXML);
                }
            }

            $("#chat-input").val("");
        });
    },
    getSettings: function() {
        return {
            pushTime: 2500,
            changeAmount: 30,
        };
    },
    shuffleBubbles: function() {
        $.each($("body").find("x-bubble"), function(i, msg) {
            var $msg          = $(msg);
            var scrollTop     = $(window).scrollTop(),
                elementOffset = $msg.offset().top,
                distance      = (elementOffset - scrollTop);

            var msgTop = distance - Chat.getSettings().changeAmount;
            
            $msg.css("top", msgTop);

            if (msgTop < -100) $msg.remove();
        });
    },
    create: function(name, message, left) {
        if (left == null || left > 5) left = 2;

        this.shuffleBubbles();
        message = this.escapeHtmlEntities(message);

        var id = name + "-" + Math.floor(Math.random()*10000) + "-" + left;

        $("<x-bubble id='" + id + "'>" +
                "<x-chat-name>" + name + "</x-chat-name>" +
                "<x-chat-msg>" + message + "</x-chat-msg>" +
            "</x-bubble>")
        .appendTo("body")
        .css({
            "top": 200,
            "left": left + "0vw"
        });
    },
    escapeHtmlEntities: function(text) {
        return $("<div>").text(text).html();
    },
    handleAsLocalCommand: function(text) {
        if(text == '' || text.charAt(0) != '.')
            return false;

        var args = text.split(" ");

        switch(args[0]) {
            case '.ige': 
                if (ige.editor) ige.editor.toggle();
                return true;

            case '.hair': 
                if(args[1] && ige.player) {
                    ige.player.setHairStyle(args[1]);
                    ige.player.changeDirection();
                }
                return true;

            case '.shirt': 
                if(args[1] && ige.player) {
                    ige.player.setShirtStyle(args[1]);
                    ige.player.changeDirection();
                }
                return true;

            case '.pant': 
                if(args[1] && ige.player) {
                    ige.player.setPantStyle(args[1]);
                    ige.player.changeDirection();
                }
                return true;

            case '.shoe': 
                if(args[1] && ige.player) {
                    ige.player.setShoeStyle(args[1]);
                    ige.player.changeDirection();
                }
                return true;

            case '.haircolor':
                if(args[1] && ige.player && ige.player.head && ige.player.head.hair) {
                    ige.player.head.hair.setColor(args[1]);
                }
                return true;

            case '.a':
                if(args[1] && ige.player) {
                    ige.player.changeAnimation(args[1]);
                }
                return true;

            case '.carry':
                if (ige.player) ige.player.carryToggle();
                return true;

            case '.drink':
            case '.coke':
                if (ige.client) ige.client.drinkCoke();
                return true;

            case '.dj':
            case '.music':
                if (ige.client) ige.client.triggerDJPerformance();
                return true;

            case '.tu':
            case '.cheer':
                if (ige.client) ige.client.voteCheer();
                return true;

            case '.td':
            case '.boo':
                if (ige.client) ige.client.voteBoo();
                return true;
        }

        return false;
    },
};

Chat.init();
