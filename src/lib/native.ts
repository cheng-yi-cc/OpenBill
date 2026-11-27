// Set by the Android host before the Vue bundle runs. Browser deployments keep
// their existing authentication and do not gain an anonymous API session.
export function isNativePersonalMode(): boolean {
    return typeof window !== 'undefined' && (window as Window & { __CYLEDGER_PERSONAL__?: boolean }).__CYLEDGER_PERSONAL__ === true;
}

declare global { interface Window { CYLedgerAppearance?: { setTheme(color:string,dark:boolean):void }; CYLedgerLocal?: { status():string; settings(json:string):void; action(action:string,payload:string):void; preferences(json:string):void; backups():string; backup(settings:string):void; restore():void; restoreLocal?(name:string):void; exportBackup(name:string):void; deleteBackup(name:string):void; appInfo?():string; updateState?():string; checkUpdate?(manual:boolean):void; downloadAndInstallUpdate?():void; cancelUpdateDownload?():void; ignoreUpdate?(tag:string):void; openReleasePage?(url:string):void } } }

function cssColorToHex(raw: string): string {
    const trimmed = raw.trim();
    if (/^#[0-9a-f]{6}$/i.test(trimmed)) return trimmed;
    if (/^#[0-9a-f]{3}$/i.test(trimmed)) return '#' + trimmed.slice(1).split('').map(c => c + c).join('');
    const rgb = trimmed.match(/^rgba?\(\s*(\d+)\s*,\s*(\d+)\s*,\s*(\d+)(?:\s*,\s*([\d.]+))?\s*\)$/i);
    if (rgb && (rgb[4] === undefined || Number(rgb[4]) > 0.05)) {
        return '#' + [rgb[1], rgb[2], rgb[3]].map(v => Math.max(0, Math.min(255, Number(v))).toString(16).padStart(2, '0')).join('');
    }
    return '';
}

export function syncNativeAppearance():void {
    if (!isNativePersonalMode()) return;
    requestAnimationFrame(()=>{
        const page=document.querySelector('.page-current.cy-mobile-surface')||document.querySelector('.page-current')||document.documentElement;
        const dark=document.documentElement.classList.contains('dark');
        const navBg=page.querySelector('.navbar .navbar-bg')||page.querySelector('.stat-top')||page.querySelector('.cy-month-header');
        const fromNav=navBg?cssColorToHex(getComputedStyle(navBg).backgroundColor):'';
        const styles=getComputedStyle(page);
        const fromBg=cssColorToHex(styles.getPropertyValue('--cy-bg'))||cssColorToHex(styles.getPropertyValue('--cy-card'));
        const color=fromNav||fromBg||(dark?'#142127':'#f5f6f8');
        window.CYLedgerAppearance?.setTheme(color,dark);
    });
}

export function localBackupSettings():string { const values:Record<string,string>={};for(const key of Object.keys(localStorage))if(key==='ebk_app_settings'||key.startsWith('cy_ledger_experience_')){const value=localStorage.getItem(key);if(value){try{JSON.parse(value);values[key]=value;}catch{/* Ignore malformed browser preferences. */}}}return JSON.stringify(values); }
export function syncNativeSettings():void { window.CYLedgerLocal?.settings(localBackupSettings()); }
