export interface CryptoPlatform { id: string; name: string; kind: 'EXCHANGE' | 'WALLET'; icon?: string; aliases?:string }
export const cryptoPlatforms: CryptoPlatform[] = [
 { id:'binance', name:'币安 Binance', kind:'EXCHANGE' },
 { id:'okx', name:'欧易 OKX', kind:'EXCHANGE' },
 { id:'coinbase', name:'Coinbase', kind:'EXCHANGE' },
 { id:'kraken', name:'Kraken', kind:'EXCHANGE' },
 { id:'bybit', name:'Bybit', kind:'EXCHANGE' },
 { id:'bitget', name:'Bitget', kind:'EXCHANGE' },
 { id:'gate', name:'Gate', kind:'EXCHANGE', icon:'gate.png', aliases:'芝麻开门' },
 { id:'kucoin', name:'KuCoin', kind:'EXCHANGE', aliases:'库币' },
 { id:'htx', name:'HTX 火币', kind:'EXCHANGE', icon:'htx.png' },
 { id:'mexc', name:'MEXC', kind:'EXCHANGE', icon:'mexc.webp', aliases:'抹茶' },
 { id:'crypto-com', name:'Crypto.com', kind:'EXCHANGE' },
 { id:'gemini', name:'Gemini', kind:'EXCHANGE' },
 { id:'other-exchange', name:'自定义交易所', kind:'EXCHANGE', icon:'' },
 { id:'okx-wallet', name:'OKX Wallet 欧易钱包', kind:'WALLET' },
 { id:'imtoken', name:'imToken', kind:'WALLET' },
 { id:'tokenpocket', name:'TokenPocket', kind:'WALLET', aliases:'TP钱包' },
 { id:'onekey', name:'OneKey', kind:'WALLET', icon:'onekey.png' },
 { id:'exodus', name:'Exodus', kind:'WALLET' },
 { id:'safe', name:'Safe', kind:'WALLET' },
 { id:'bitget-wallet', name:'Bitget Wallet', kind:'WALLET', icon:'bitget-wallet.png' },
 { id:'metamask', name:'MetaMask', kind:'WALLET', aliases:'小狐狸' },
 { id:'trust', name:'Trust Wallet', kind:'WALLET' },
 { id:'phantom', name:'Phantom', kind:'WALLET' },
 { id:'rabby', name:'Rabby', kind:'WALLET' },
 { id:'ledger', name:'Ledger', kind:'WALLET' },
 { id:'trezor', name:'Trezor', kind:'WALLET' }
];
export const isCryptoAccount = (kind?: string): boolean => kind === 'EXCHANGE' || kind === 'WALLET';
export function platformIcon(id?: string, kind?: string): string | undefined {
 const platform=cryptoPlatforms.find(p=>p.id===id);
 if(!platform)return kind==='WALLET'?'img/crypto-platforms/wallet.svg':undefined;
 if(platform.icon==='')return undefined;
 const file=platform.icon||`${platform.id}.svg`;
 return `img/crypto-platforms/${file}`;
}
export function platformName(id?: string): string { return cryptoPlatforms.find(p=>p.id===id)?.name || ''; }
