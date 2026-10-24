if (!igeRoot) {
	igeRoot = './';
}
window.igeLoader = (function () {
	// Generate cache-busting timestamp tag
	var cacheTag = '_t=' + Date.now();

	// Load the engine stylesheet with cache buster
	var css = document.createElement('link');
	css.rel = 'stylesheet';
	css.type = 'text/css';
	css.media = 'all';
	css.href = igeRoot + 'css/ige.css?' + cacheTag;
	
	document.getElementsByTagName('head')[0].appendChild(css);
	
	var IgeLoader = function () {
		var self = this,
			ccScript;

		this._loadingCount = 0;

		// Load CoreConfig.js with cache buster
		ccScript = document.createElement('script');
		ccScript.src = igeRoot + 'CoreConfig.js?' + cacheTag;
		ccScript.onload = function () {
			self.coreConfigReady();
		};
		ccScript.addEventListener('error', function () {
			throw('ERROR LOADING ' + igeRoot + 'CoreConfig.js' + ' - does it exist?');
		}, true);

		document.getElementsByTagName('head')[0].appendChild(ccScript);
	};

	IgeLoader.prototype.coreConfigReady = function () {
		var self = this;

		if (typeof(igeCoreConfig) !== 'undefined') {
			// Load ClientConfig.js with cache buster
			var ccScript = document.createElement('script');
			ccScript.src = './ClientConfig.js?' + cacheTag;
			ccScript.onload = function () {
				self.clientConfigReady();
			};
			ccScript.addEventListener('error', function () {
				throw('ERROR LOADING ClientConfig.js - does it exist?');
			}, true);

			document.getElementsByTagName('head')[0].appendChild(ccScript);
		} else {
			throw('ERROR READING igeCoreConfig object - was it specified in CoreConfig.js?');
		}
	};

	IgeLoader.prototype.clientConfigReady = function () {
		this._coreList = igeCoreConfig.include;
		this._clientList = igeClientConfig.include;

		this._fileList = [];
		for (var i = 0; i < this._coreList.length; i++) {
			if (this._coreList[i][0].indexOf('c') > -1) {
				this._fileList.push(igeRoot + this._coreList[i][2]);
			}
		}

		for (var j = 0; j < this._clientList.length; j++) {
			this._fileList.push(this._clientList[j]);
		}

		this.loadNext();
	};

	IgeLoader.prototype.loadNext = function () {
		var url = this._fileList.shift(),
			script = document.createElement('script'),
			self = this;

		if (url !== undefined) {
			var sep = (url.indexOf('?') === -1) ? '?' : '&';
			script.src = url + sep + cacheTag;
			script.onload = function () {
				self.loadNext();
			};

			script.addEventListener('error', function () {
				throw('ERROR LOADING ' + url + ' - does it exist?');
			}, true);

			document.getElementsByTagName('head')[0].appendChild(script);
		}
	};

	return new IgeLoader();
}());
