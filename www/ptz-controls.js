const velocities = {
    up: [0, 0.5, 0], down: [0, -0.5, 0],
    left: [-0.5, 0, 0], right: [0.5, 0, 0],
    'zoom-in': [0, 0, 0.5], 'zoom-out': [0, 0, -0.5],
};

class PTZControls extends HTMLElement {
    constructor() {
        super();
        this.attachShadow({mode: 'open'});
        this.active = null;
        this.pending = null;
        this.sending = false;
        this.refreshTimer = null;
        this.hideTimer = null;
        this.events = null;
        this.presetsLoadId = 0;
    }

    connectedCallback() {
        this.shadowRoot.innerHTML = `
        <style>
            :host { position: absolute; bottom: 50px; right: 15px; z-index: 10; }
            button {
                border: 0; color: #333; cursor: pointer; font-size: 20px;
                touch-action: none; user-select: none; -webkit-user-select: none;
                border-radius: 50%; overflow: hidden; appearance: none;
                -webkit-tap-highlight-color: transparent;
            }
            button:focus-visible { outline: 2px solid white; outline-offset: -2px; }
            .toggle { width: 30px; height: 30px; border-radius: 50%; background: #0009; color: white; }
            .panel { position: absolute; bottom: 35px; right: 5px; display: flex; gap: 10px; align-items: center; }
            .panel[hidden] { display: none; }
            .directions { position: relative; width: 100px; height: 100px; border-radius: 50%; overflow: hidden; background: #fffb; }
            .directions button { position: absolute; width: 30px; height: 30px; background: transparent; padding: 0; }
            [data-dir="up"] { top: 0; left: 35px; }
            [data-dir="down"] { bottom: 0; left: 35px; }
            [data-dir="left"] { top: 35px; left: 0; }
            [data-dir="right"] { top: 35px; right: 0; }
            .directions .stop { top: 35px; left: 35px; background: #e33; border-radius: 50%; }
            .zoom { display: flex; flex-direction: column; gap: 10px; }
            .zoom button { width: 40px; height: 40px; border-radius: 50%; background: #fffb; }
            button.active { background: #999 !important; }
            .directions button:hover { background: #ccc; }
            .presets-toggle { position: absolute; top: -14px; right: -12px; width: 28px; height: 28px;
                z-index: 2; background: #333d; color: white; font-size: 16px; }
            .presets-menu { position: absolute; bottom: calc(100% + 10px); right: -12px;
                z-index: 3; width: 185px; max-height: min(240px, 45vh); overflow-y: auto;
                box-sizing: border-box; padding: 5px; background: #222e; border-radius: 8px; }
            .presets-menu[hidden] { display: none; }
            .presets-menu button { display: block; width: 100%; border-radius: 5px; padding: 9px;
                background: transparent; color: white; font-size: 13px; text-align: left;
                white-space: nowrap; text-overflow: ellipsis; }
            .presets-menu button:hover { background: #666; }
            .presets-menu .message { color: white; font-size: 12px; padding: 9px; }
            .status { position: absolute; bottom: 145px; right: 0; width: 220px; color: white; background: #0009; font-size: 12px; }
            .status:empty { display: none; }
        </style>
        <button class="toggle" aria-label="PTZ controls" aria-expanded="false">✥</button>
        <div class="panel" hidden>
            <button class="presets-toggle" type="button" aria-label="Presets" aria-expanded="false" title="Presets">☷</button>
            <div class="presets-menu" hidden></div>
            <div class="zoom">
                <button data-dir="zoom-in" aria-label="Zoom in">+</button>
                <button data-dir="zoom-out" aria-label="Zoom out">−</button>
            </div>
            <div class="directions">
                <button data-dir="up" aria-label="Tilt up">▲</button>
                <button data-dir="right" aria-label="Pan right">▶</button>
                <button data-dir="down" aria-label="Tilt down">▼</button>
                <button data-dir="left" aria-label="Pan left">◀</button>
                <button class="stop" aria-label="Stop movement"></button>
            </div>
        </div>
        <div class="status" role="status"></div>`;

        this.events = new AbortController();
        const options = {signal: this.events.signal};
        this.shadowRoot.querySelector('.toggle').addEventListener('click', () => this.showControls(this.panel.hidden), options);
        this.shadowRoot.querySelector('.stop').addEventListener('click', () => this.stop(true), options);
        this.shadowRoot.querySelector('.presets-toggle').addEventListener('click', () => this.togglePresets(), options);
        this.panel.addEventListener('pointermove', () => this.resetHideTimer(), options);

        for (const button of this.shadowRoot.querySelectorAll('[data-dir]')) {
            button.addEventListener('pointerdown', event => {
                if (event.button !== 0 || this.active) return;
                event.preventDefault();
                button.setPointerCapture(event.pointerId);
                this.start(button, event.pointerId);
            }, options);
            const release = event => {
                if (this.active && this.active.pointerId === event.pointerId) this.stop();
            };
            button.addEventListener('pointerup', release, options);
            for (const type of ['pointercancel', 'lostpointercapture']) {
                button.addEventListener(type, event => {
                    if (this.active && this.active.pointerId === event.pointerId) this.stop();
                }, options);
            }
            button.addEventListener('keydown', event => {
                if (event.key !== ' ' && event.key !== 'Enter') return;
                event.preventDefault();
                if (!event.repeat && !this.active) this.start(button, null);
            }, options);
            button.addEventListener('keyup', event => {
                if (event.key === ' ' || event.key === 'Enter') {
                    event.preventDefault();
                    this.stop();
                }
            }, options);
            button.addEventListener('blur', () => this.stop(), options);
        }
        window.addEventListener('blur', () => this.stop(), options);
        window.addEventListener('pagehide', () => this.stop(), options);
        document.addEventListener('visibilitychange', () => {
            if (document.hidden) this.stop();
        }, options);
    }

    disconnectedCallback() {
        this.stop();
        clearTimeout(this.hideTimer);
        this.events.abort();
    }

    get panel() {
        return this.shadowRoot.querySelector('.panel');
    }

    showControls(visible) {
        this.panel.hidden = !visible;
        this.shadowRoot.querySelector('.toggle').setAttribute('aria-expanded', String(visible));
        if (visible) this.resetHideTimer();
        else {
            this.closePresets();
            this.stop();
            clearTimeout(this.hideTimer);
        }
    }

    resetHideTimer() {
        clearTimeout(this.hideTimer);
        // Never hide a held button: doing so can swallow its release event.
        const menu = this.shadowRoot.querySelector('.presets-menu');
        if (!this.active && menu.hidden) this.hideTimer = setTimeout(() => this.showControls(false), 8000);
    }

    closePresets() {
        const menu = this.shadowRoot.querySelector('.presets-menu');
        menu.hidden = true;
        this.shadowRoot.querySelector('.presets-toggle').setAttribute('aria-expanded', 'false');
        this.presetsLoadId++; // Ignore results from a menu that has been closed.
        this.resetHideTimer();
    }

    async togglePresets() {
        const menu = this.shadowRoot.querySelector('.presets-menu');
        if (!menu.hidden) {
            this.closePresets();
            return;
        }
        menu.hidden = false;
        this.shadowRoot.querySelector('.presets-toggle').setAttribute('aria-expanded', 'true');
        clearTimeout(this.hideTimer);
        const loadId = ++this.presetsLoadId;
        const message = document.createElement('div');
        message.className = 'message';
        message.textContent = 'Loading presets…';
        menu.replaceChildren(message);

        try {
            const response = await fetch(new URL('api/ptz', location.href), {
                method: 'POST',
                headers: {'Content-Type': 'application/json'},
                body: JSON.stringify({src: this.getAttribute('source'), action: 'presets'}),
            });
            if (!response.ok) throw new Error(await response.text());
            const presets = await response.json();
            if (!Array.isArray(presets)) throw new Error('Invalid presets response');
            if (menu.hidden || loadId !== this.presetsLoadId) return;
            menu.replaceChildren();
            for (const preset of presets) {
                if (typeof preset.token !== 'string' || !preset.token) continue;
                const option = document.createElement('button');
                option.type = 'button';
                option.textContent = preset.name || preset.token;
                option.title = option.textContent;
                option.addEventListener('click', () => {
                    // Reuse the existing movement queue; server stops before GotoPreset.
                    if (this.active) this.stop();
                    this.sendCommand({action: 'goto-preset', preset: preset.token});
                    this.closePresets();
                });
                menu.appendChild(option);
            }
            if (!menu.children.length) {
                message.textContent = 'No presets available';
                menu.appendChild(message);
            }
        } catch (error) {
            if (menu.hidden || loadId !== this.presetsLoadId) return;
            message.textContent = error.message || 'Failed to load presets';
            menu.replaceChildren(message);
        }
    }

    start(button, pointerId) {
        this.active = {button, pointerId};
        button.classList.add('active');
        clearTimeout(this.hideTimer);
        const [pan, tilt, zoom] = velocities[button.dataset.dir];
        const move = () => this.sendCommand({action: 'move', pan, tilt, zoom});
        // Renew the camera's 1s timeout while the button remains held.
        this.refreshTimer = setInterval(move, 300);
        move();
    }

    stop(force = false) {
        if (!this.active && !force) return;
        if (this.active) this.active.button.classList.remove('active');
        this.active = null;
        clearInterval(this.refreshTimer);
        this.refreshTimer = null;
        this.sendCommand({action: 'stop'});
        this.resetHideTimer();
    }

    sendCommand(command) {
        // Replace obsolete renewals; releasing replaces every unsent move with a stop.
        this.pending = {src: this.getAttribute('source'), ...command};
        if (!this.sending) this.flushCommands();
    }

    async flushCommands() {
        this.sending = true;
        while (this.pending) {
            const command = this.pending;
            this.pending = null;
            try {
                const response = await fetch(new URL('api/ptz', location.href), {
                    method: 'POST',
                    headers: {'Content-Type': 'application/json'},
                    body: JSON.stringify(command),
                    keepalive: true,
                });
                if (!response.ok) throw new Error(await response.text());
                this.shadowRoot.querySelector('.status').textContent = '';
            } catch (error) {
                this.shadowRoot.querySelector('.status').textContent = error.message || 'PTZ command failed';
                console.error('PTZ command failed:', error);
                if (command.action === 'move') this.stop();
            }
        }
        this.sending = false;
    }
}

customElements.define('ptz-controls', PTZControls);
