import React from 'react';

interface IconProps { size?: number; className?: string }

export const RouterIcon: React.FC<IconProps> = ({ size = 48, className }) => (
  <img src="/icons/router.svg" width={size} height={size} alt="Router" className={className} draggable={false} />
);
export const SwitchIcon: React.FC<IconProps> = ({ size = 48, className }) => (
  <img src="/icons/switch.svg" width={size} height={size} alt="Switch" className={className} draggable={false} />
);
export const PcIcon: React.FC<IconProps> = ({ size = 48, className }) => (
  <img src="/icons/pc.svg" width={size} height={size} alt="PC" className={className} draggable={false} />
);
export const ServerIcon: React.FC<IconProps> = ({ size = 48, className }) => (
  <img src="/icons/server.svg" width={size} height={size} alt="Server" className={className} draggable={false} />
);
export const AsaIcon: React.FC<IconProps> = ({ size = 48, className }) => (
  <img src="/icons/asa.svg" width={size} height={size} alt="ASA" className={className} draggable={false} />
);
export const ApIcon: React.FC<IconProps> = ({ size = 48, className }) => (
  <img src="/icons/ap.svg" width={size} height={size} alt="AP" className={className} draggable={false} />
);
export const CloudIcon: React.FC<IconProps> = ({ size = 48, className }) => (
  <img src="/icons/cloud.svg" width={size} height={size} alt="Cloud" className={className} draggable={false} />
);
export const PhoneIcon: React.FC<IconProps> = ({ size = 48, className }) => (
  <img src="/icons/phone.svg" width={size} height={size} alt="Phone" className={className} draggable={false} />
);
export const CellularGwIcon: React.FC<IconProps> = ({ size = 48, className }) => (
  <img src="/icons/cellular.svg" width={size} height={size} alt="Cellular GW" className={className} draggable={false} />
);
export const MobileUEIcon: React.FC<IconProps> = ({ size = 48, className }) => (
  <img src="/icons/mobile.svg" width={size} height={size} alt="Mobile UE" className={className} draggable={false} />
);
export const CallManagerIcon: React.FC<IconProps> = ({ size = 48, className }) => (
  <img src="/icons/cucm.svg" width={size} height={size} alt="Call Manager" className={className} draggable={false} />
);

import type { DeviceModel } from './deviceCatalog';

export function DeviceIcon({ model, size = 48 }: { model: DeviceModel; size?: number }) {
  switch (model) {
    case 'ROUTER': return <RouterIcon size={size} />;
    case 'SWITCH': return <SwitchIcon size={size} />;
    case 'PC': return <PcIcon size={size} />;
    case 'SERVER': return <ServerIcon size={size} />;
    case 'ASA': return <AsaIcon size={size} />;
    case 'AP': return <ApIcon size={size} />;
    case 'CLOUD': return <CloudIcon size={size} />;
    case 'PHONE': return <PhoneIcon size={size} />;
    case 'CELLULAR_GW': return <CellularGwIcon size={size} />;
    case 'MOBILE_UE': return <MobileUEIcon size={size} />;
    case 'CALL_MANAGER': return <CallManagerIcon size={size} />;
    default: return <RouterIcon size={size} />;
  }
}
