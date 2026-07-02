export type DeviceCategory =
  | 'routers'
  | 'switches'
  | 'end_devices'
  | 'servers'
  | 'wireless'
  | 'security'
  | 'wan'
  | 'connections';

export type DeviceModel =
  | 'ROUTER'
  | 'SWITCH'
  | 'PC'
  | 'SERVER'
  | 'ASA'
  | 'AP'
  | 'PHONE'
  | 'CLOUD'
  | 'COPPER'
  | 'FIBER';

export interface DeviceCatalogItem {
  model: DeviceModel;
  label: string;
  icon: string;
  description: string;
}

export const DEVICE_CATEGORIES: { id: DeviceCategory; label: string }[] = [
  { id: 'routers', label: 'Routers' },
  { id: 'switches', label: 'Switches' },
  { id: 'end_devices', label: 'End Devices' },
  { id: 'servers', label: 'Servers' },
  { id: 'wireless', label: 'Wireless' },
  { id: 'security', label: 'Security' },
  { id: 'wan', label: 'WAN Emulation' },
  { id: 'connections', label: 'Connections' },
];

export const DEVICE_CATALOG: Record<DeviceCategory, DeviceCatalogItem[]> = {
  routers: [
    { model: 'ROUTER', label: '2911', icon: '/icons/router.svg', description: 'ISR router' },
    { model: 'ROUTER', label: '1941', icon: '/icons/router.svg', description: 'Branch router' },
  ],
  switches: [
    { model: 'SWITCH', label: '2960', icon: '/icons/switch.svg', description: 'Layer 2 switch' },
    { model: 'SWITCH', label: '3650', icon: '/icons/switch.svg', description: 'Multilayer switch' },
  ],
  end_devices: [
    { model: 'PC', label: 'PC-PT', icon: '/icons/pc.svg', description: 'Desktop host' },
    { model: 'PHONE', label: 'IP Phone', icon: '/icons/phone.svg', description: 'VoIP phone' },
  ],
  servers: [
    { model: 'SERVER', label: 'Server-PT', icon: '/icons/server.svg', description: 'Generic server' },
  ],
  wireless: [
    { model: 'AP', label: 'AccessPoint', icon: '/icons/ap.svg', description: 'Wireless AP' },
  ],
  security: [
    { model: 'ASA', label: 'ASA 5505', icon: '/icons/asa.svg', description: 'Firewall' },
  ],
  wan: [
    { model: 'CLOUD', label: 'Cloud', icon: '/icons/cloud.svg', description: 'Internet / cloud' },
    { model: 'ROUTER', label: 'VPN Peer', icon: '/icons/router.svg', description: 'Site-to-site VPN' },
  ],
  connections: [
    { model: 'COPPER', label: 'Copper Straight', icon: '/icons/copper.svg', description: 'Copper cable' },
    { model: 'FIBER', label: 'Fiber', icon: '/icons/fiber.svg', description: 'Fiber optic' },
  ],
};

export function iconForModel(model: DeviceModel): string {
  const map: Record<DeviceModel, string> = {
    ROUTER: '/icons/router.svg',
    SWITCH: '/icons/switch.svg',
    PC: '/icons/pc.svg',
    SERVER: '/icons/server.svg',
    ASA: '/icons/asa.svg',
    AP: '/icons/ap.svg',
    PHONE: '/icons/phone.svg',
    CLOUD: '/icons/cloud.svg',
    COPPER: '/icons/copper.svg',
    FIBER: '/icons/fiber.svg',
  };
  return map[model];
}
