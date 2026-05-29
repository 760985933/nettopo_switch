package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/gorilla/websocket"
)

const codexDebugPort = 9229

// Injection script — modeled after Codex++ renderer-inject.js.
// Registered via Page.addScriptToEvaluateOnNewDocument so it runs
// BEFORE any Codex app JS, then also executed immediately.
const unlockScript = `
(function(){
	console.log('[Codex++] Plugin unlock starting...');

	// ---- 1. Intercept Object.defineProperty ----
	// React and many frameworks use this to set reactive state.
	// If Codex tries to define plugins=false, we block it.
	var _dp = Object.defineProperty;
	Object.defineProperty = function(obj, prop, desc) {
		var lp = (prop||'').toLowerCase();
		if (lp==='plugins'||lp==='pluginsenabled'||lp==='pluginenabled'||lp==='haspluginaccess') {
			if (desc && desc.value===false) {
				console.log('[Codex++] Blocked Object.defineProperty('+prop+', false)');
				desc.value = true;
			}
		}
		return _dp.call(this, obj, prop, desc);
	};

	// ---- 2. Intercept fetch ----
	// Patch any JSON response that contains feature flags.
	var _fetch = window.fetch;
	window.fetch = function(url, opts) {
		var urlStr = typeof url==='string' ? url : (url&&url.url?url.url:'');
		return _fetch.apply(this, arguments).then(function(resp){
			var ct = resp.headers.get('content-type')||'';
			if (ct.indexOf('json')===-1) return resp;
			return resp.clone().json().then(function(data){
				var c = false;
				(function patch(o){
					if (!o||typeof o!=='object') return;
					if (Array.isArray(o)){o.forEach(patch);return;}
					Object.keys(o).forEach(function(k){
						var lk=k.toLowerCase();
						if (lk==='plugins'||lk==='pluginsenabled'||lk==='pluginenabled'){if(typeof o[k]==='boolean'||o[k]===null){o[k]=true;c=true;}}
						if (lk==='features'||lk==='entitlements'||lk==='featureflags'){if(o[k]&&typeof o[k]==='object'){o[k].plugins=true;o[k].pluginsEnabled=true;c=true;}}
						if (typeof o[k]==='object') patch(o[k]);
					});
				})(data);
				if (c) console.log('[Codex++] Patched fetch response:', urlStr.substring(0,80));
				return new Response(JSON.stringify(data),{status:resp.status,statusText:resp.statusText,headers:resp.headers});
			}).catch(function(){return resp;});
		}).catch(function(e){return Promise.reject(e);});
	};

	// ---- 3. Intercept WebSocket ----
	var _WS = window.WebSocket;
	window.WebSocket = function(url, protocols) {
		var ws = new _WS(url, protocols);
		var _handler = null;
		try {
			Object.defineProperty(ws, 'onmessage', {
				get:function(){return _handler;},
				set:function(fn){
					_handler = function(ev){
						try {
							if (typeof ev.data==='string'){
								var d = JSON.parse(ev.data);
								if (d&&typeof d==='object'){
									if (d.features){d.features.plugins=true;d.features.pluginsEnabled=true;}
									if ('plugins' in d) d.plugins=true;
									if ('pluginsEnabled' in d) d.pluginsEnabled=true;
									ev=new MessageEvent('message',{data:JSON.stringify(d),origin:ev.origin});
								}
							}
						}catch(_){}
						return fn.call(this, ev);
					};
				},
				configurable:true
			});
		}catch(_){}
		return ws;
	};

	// ---- 4. Intercept Storage ----
	var _setItem = Storage.prototype.setItem;
	Storage.prototype.setItem = function(k,v){
		try{var lk=(k||'').toLowerCase();if(lk.indexOf('feature')!==-1||lk.indexOf('plugin')!==-1||lk.indexOf('entitle')!==-1){var o=JSON.parse(v);if(o&&typeof o==='object'){o.plugins=true;o.pluginsEnabled=true;v=JSON.stringify(o);}}}catch(_){}
		return _setItem.call(this,k,v);
	};

	// ---- 5. Intercept Electron IPC (contextBridge) ----
	try {
		Object.keys(window).forEach(function(k){
			try {
				var api=window[k];
				if (api&&typeof api==='object'&&typeof api.invoke==='function'&&!api.nodeType){
					var _invoke=api.invoke.bind(api);
					api.invoke=function(){
						return _invoke.apply(this,arguments).then(function(r){
							if(r&&typeof r==='object'){
								(function patch(o){if(!o||typeof o!=='object')return;if(Array.isArray(o)){o.forEach(patch);return;}Object.keys(o).forEach(function(k){var lk=k.toLowerCase();if(lk==='plugins'||lk==='pluginsenabled')o[k]=true;if(lk==='features'||lk==='entitlements'){if(o[k]&&typeof o[k]==='object'){o[k].plugins=true;o[k].pluginsEnabled=true;}}if(typeof o[k]==='object')patch(o[k]);});})(r);
							}
							return r;
						}).catch(function(e){return Promise.reject(e);});
					};
				}
			}catch(_){}
		});
	}catch(_){}

	// ---- 6. Set globals ----
	Object.defineProperty(window,'__PLUGINS_ENABLED',{value:true,writable:false,configurable:false});
	window.hasPluginAccess = function(){return true;};

	// ---- 7. CSS + DOM + React state patching ----
	function applyCSS(){
		if (document.getElementById('cxpp-css')) return;
		var s=document.createElement('style');
		s.id='cxpp-css';
		s.textContent='[class*="plugin"],[class*="Plugin"]{display:flex!important;visibility:visible!important;opacity:1!important;pointer-events:auto!important;z-index:auto!important;filter:none!important}[class*="plugin"][disabled],[class*="Plugin"][disabled],[class*="plugin"] [disabled],[class*="Plugin"] [disabled]{pointer-events:auto!important;opacity:1!important}[aria-disabled=true]{pointer-events:auto!important}[class*="overlay"]:has([class*="plugin"]){display:none!important}[class*="grey"],[class*="gray"]{filter:none!important;opacity:1!important}';
		document.head.appendChild(s);
	}
	function removeDisabled(){
		document.querySelectorAll('[class*="plugin"][disabled],[class*="Plugin"][disabled],[class*="plugin"] [disabled],[class*="Plugin"] [disabled]').forEach(function(e){e.removeAttribute('disabled');});
		document.querySelectorAll('[class*="plugin"][aria-disabled=true],[class*="Plugin"][aria-disabled=true]').forEach(function(e){e.setAttribute('aria-disabled','false');});
	}

	// ---- 8. Deep React state mutation ----
	// Traverse React fiber tree and force plugin-related state to true
	function patchReactFiberTree(){
		try {
			// Find any element with React fiber
			var rootEl = document.getElementById('root') || document.getElementById('app') || document.body;
			var fiberKey = null;
			// Try body and children
			var allEls = document.querySelectorAll('body, body *, #root, #root *, #app, #app *');
			for (var i = 0; i < Math.min(allEls.length, 200); i++) {
				var keys = Object.keys(allEls[i]);
				for (var j = 0; j < keys.length; j++) {
					if (keys[j].startsWith('__reactFiber') || keys[j].startsWith('__reactInternalInstance')) {
						fiberKey = keys[j];
						rootEl = allEls[i];
						break;
					}
				}
				if (fiberKey) break;
			}

			if (!fiberKey) {
				console.log('[Codex++] No React fiber found');
				return;
			}

			var visited = new Set();
			var patchedCount = 0;
			var maxNodes = 20000;

			function patchValue(v) {
				if (!v || typeof v !== 'object' || visited.has(v)) return;
				if (patchedCount > maxNodes) return;
				visited.add(v);
				patchedCount++;
				if (Array.isArray(v)) { v.forEach(function(item){patchValue(item);}); return; }
				try {
					var keys = Object.keys(v);
					for (var i = 0; i < keys.length; i++) {
						var k = keys[i];
						var lk = k.toLowerCase();
						if (lk === 'plugins' || lk === 'pluginsenabled' || lk === 'pluginenabled' || lk === 'haspluginaccess' || lk === 'ispluginenabled' || lk === 'pluginaccess') {
							if (typeof v[k] === 'boolean' || v[k] === null || v[k] === undefined) {
								v[k] = true;
							}
						}
						if (lk === 'features' || lk === 'featureflags' || lk === 'entitlements' || lk === 'capabilities') {
							if (v[k] && typeof v[k] === 'object' && !Array.isArray(v[k])) {
								v[k].plugins = true;
								v[k].pluginsEnabled = true;
								v[k].pluginEnabled = true;
							}
						}
						if (lk === 'plan' || lk === 'plantype' || lk === 'subscription') {
							if (typeof v[k] === 'string' && v[k] === 'free') {
								v[k] = 'plus';
							}
						}
						if (typeof v[k] === 'object' && v[k] !== null) {
							patchValue(v[k]);
						}
					}
				} catch(e) {}
			}

			var fiber = rootEl[fiberKey];
			// Walk entire fiber tree
			function walk(node) {
				if (!node || visited.has(node) || patchedCount > maxNodes) return;
				visited.add(node);
				try {
					if (node.memoizedState) {
						// Walk hooks linked list
						var hook = node.memoizedState;
						while (hook) {
							if (hook.memoizedState) patchValue(hook.memoizedState);
							if (hook.queue && hook.queue.lastRenderedState) {
								if (typeof hook.queue.lastRenderedState === 'boolean') {
									hook.queue.lastRenderedState = true;
								} else {
									patchValue(hook.queue.lastRenderedState);
								}
							}
							hook = hook.next;
						}
					}
					if (node.memoizedProps) patchValue(node.memoizedProps);
					if (node.pendingProps) patchValue(node.pendingProps);
					if (node.stateNode && node.stateNode.state) patchValue(node.stateNode.state);
				} catch(e) {}
				walk(node.child);
				walk(node.sibling);
				walk(node.return); // also walk up to find context providers
			}
			walk(fiber);
			console.log('[Codex++] React fiber patched (' + patchedCount + ' nodes visited)');
		} catch(e) {
			console.log('[Codex++] React fiber patch error:', e.message);
		}
	}

	// ---- 9. Find and patch global stores (Zustand, Redux, etc.) ----
	function patchGlobalStores(){
		Object.keys(window).forEach(function(k){
			try {
				var v = window[k];
				if (!v || typeof v !== 'object' || v.nodeType) return;
				// Zustand stores have getState/setState
				if (typeof v.getState === 'function' && typeof v.setState === 'function') {
					try {
						var state = v.getState();
						if (state && typeof state === 'object') {
							(function patch(o){
								if (!o||typeof o!=='object') return;
								if (Array.isArray(o)){o.forEach(patch);return;}
								Object.keys(o).forEach(function(kk){
									var lk=kk.toLowerCase();
									if (lk==='plugins'||lk==='pluginsenabled'||lk==='pluginenabled'){if(typeof o[kk]==='boolean'||o[kk]===null){o[kk]=true;}}
									if (lk==='features'||lk==='entitlements'){if(o[kk]&&typeof o[kk]==='object'){o[kk].plugins=true;o[kk].pluginsEnabled=true;}}
									if (typeof o[kk]==='object') patch(o[kk]);
								});
							})(state);
							v.setState(state);
							console.log('[Codex++] Patched Zustand store:', k);
						}
					} catch(e) {}
				}
			} catch(e) {}
		});
	}

	// Run patching on DOM ready
	function runAllPatches(){
		applyCSS();
		removeDisabled();
		patchReactFiberTree();
		patchGlobalStores();
		setTimeout(removeDisabled, 500);
		setTimeout(patchReactFiberTree, 1000);
		setTimeout(patchReactFiberTree, 3000);
		setTimeout(patchReactFiberTree, 8000);
	}
	if (document.readyState==='loading'){
		document.addEventListener('DOMContentLoaded', runAllPatches);
	} else {
		runAllPatches();
	}
	// Observer for dynamically added elements
	new MutationObserver(function(){removeDisabled();}).observe(document.documentElement,{childList:true,subtree:true,attributes:true,attributeFilter:['disabled','aria-disabled']});

	console.log('[Codex++] Plugin unlock registered (with React fiber + store patching)');
})();
`

type cdpTarget struct {
	WebSocketDebuggerURL string `json:"webSocketDebuggerUrl"`
	Title                string `json:"title"`
	URL                  string `json:"url"`
	Type                 string `json:"type"`
}

func getCodexTargets() []cdpTarget {
	resp, err := http.Get(fmt.Sprintf("http://127.0.0.1:%d/json/list", codexDebugPort))
	if err != nil {
		return nil
	}
	defer resp.Body.Close()

	var all []cdpTarget
	if err := json.NewDecoder(resp.Body).Decode(&all); err != nil {
		return nil
	}

	var pages []cdpTarget
	for _, t := range all {
		if t.WebSocketDebuggerURL == "" {
			continue
		}
		if t.Type == "page" || t.Type == "" {
			pages = append(pages, t)
		}
	}

	var codexPages, otherPages []cdpTarget
	for _, p := range pages {
		if strings.Contains(strings.ToLower(p.Title), "codex") || strings.HasPrefix(p.URL, "file://") {
			codexPages = append(codexPages, p)
		} else {
			otherPages = append(otherPages, p)
		}
	}
	return append(codexPages, otherPages...)
}

// injectIntoPage connects to a single CDP page target and:
// 1. Registers the unlock script via Page.addScriptToEvaluateOnNewDocument (runs before page JS)
// 2. Also evaluates it immediately in the current page context
func injectIntoPage(wsURL string) error {
	dialer := websocket.Dialer{HandshakeTimeout: 5 * time.Second}
	conn, _, err := dialer.Dial(wsURL, nil)
	if err != nil {
		return fmt.Errorf("WebSocket 连接失败: %w", err)
	}
	defer conn.Close()

	// Enable Page domain
	conn.WriteJSON(map[string]interface{}{"id": 1, "method": "Page.enable"})
	conn.SetReadDeadline(time.Now().Add(3 * time.Second))
	var res map[string]interface{}
	conn.ReadJSON(&res) // discard response

	// Register for new documents
	conn.WriteJSON(map[string]interface{}{
		"id":     2,
		"method": "Page.addScriptToEvaluateOnNewDocument",
		"params": map[string]interface{}{"source": unlockScript},
	})
	conn.SetReadDeadline(time.Now().Add(5 * time.Second))
	if err := conn.ReadJSON(&res); err != nil {
		return fmt.Errorf("addScriptToEvaluateOnNewDocument 失败: %w", err)
	}
	if errInfo, ok := res["error"]; ok {
		return fmt.Errorf("addScriptToEvaluateOnNewDocument 异常: %v", errInfo)
	}

	// Also evaluate immediately in current page
	conn.WriteJSON(map[string]interface{}{
		"id":     3,
		"method": "Runtime.evaluate",
		"params": map[string]interface{}{
			"expression":    unlockScript,
			"returnByValue": true,
		},
	})
	conn.SetReadDeadline(time.Now().Add(5 * time.Second))
	conn.ReadJSON(&res) // discard response

	return nil
}

// TryPluginUnlock detects a running Codex with CDP enabled and injects
// the unlock script. It does NOT launch or kill Codex — the user must
// start Codex with: --remote-debugging-port=9229 --remote-allow-origins=http://127.0.0.1:9229
func TryPluginUnlock(logFn func(level, source, msg, requestID string)) error {
	targets := getCodexTargets()
	if len(targets) == 0 {
		logFn("warn", "plugin",
			fmt.Sprintf("未检测到 Codex CDP 端口 %d。请用以下参数启动 Codex: --remote-debugging-port=%d --remote-allow-origins=http://127.0.0.1:%d",
				codexDebugPort, codexDebugPort, codexDebugPort), "")
		return fmt.Errorf("Codex CDP 端口 %d 不可用", codexDebugPort)
	}

	logFn("info", "plugin", fmt.Sprintf("检测到 Codex CDP（%d 个页面），注入解锁脚本...", len(targets)), "")

	var lastErr error
	ok := 0
	for _, t := range targets {
		logFn("info", "plugin", fmt.Sprintf("  → %s (%s)", t.Title, t.URL), "")
		if err := injectIntoPage(t.WebSocketDebuggerURL); err != nil {
			lastErr = err
			logFn("warn", "plugin", fmt.Sprintf("  ✗ %s: %v", t.Title, err), "")
		} else {
			ok++
		}
	}

	if ok > 0 {
		logFn("info", "plugin", fmt.Sprintf("插件解锁脚本已注入 %d 个页面（addScriptToEvaluateOnNewDocument + Runtime.evaluate）", ok), "")
		return nil
	}
	return fmt.Errorf("所有目标注入失败: %w", lastErr)
}

// PluginUnlockLogin writes the Codex config in openai-direct format and then
// injects the plugin unlock script via CDP. It requires the proxy to be
// running (internet access needed for upstream API calls).
func (a *App) PluginUnlockLogin() (string, error) {
	path, err := a.WriteCodexConfigToml()
	if err != nil {
		return "", err
	}

	// CDP injection disabled for now
	// go func() {
	// 	if err := TryPluginUnlock(a.appendLog); err != nil {
	// 		a.appendLog("warn", "plugin", "插件解锁失败: "+err.Error(), "")
	// 	}
	// }()

	return path, nil
}
