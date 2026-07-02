import React from 'react';
import { DEVICE_CATALOG, DEVICE_CATEGORIES, DeviceCategory, DeviceModel } from '../assets/deviceCatalog';

interface DevicePaletteProps {
  category: DeviceCategory;
  onCategoryChange: (c: DeviceCategory) => void;
  onDevicePick: (model: DeviceModel, label: string) => void;
  linkMode: boolean;
}

export const DevicePalette: React.FC<DevicePaletteProps> = ({
  category, onCategoryChange, onDevicePick, linkMode,
}) => {
  const devices = DEVICE_CATALOG[category];

  return (
    <div className="pt-palette">
      <div className="pt-categories">
        {DEVICE_CATEGORIES.map((cat) => (
          <button
            key={cat.id}
            type="button"
            className={`pt-category-btn ${category === cat.id ? 'active' : ''}`}
            onClick={() => onCategoryChange(cat.id)}
          >
            {cat.label}
          </button>
        ))}
      </div>
      <div className="pt-devices">
        {linkMode && category !== 'connections' && (
          <div style={{ width: '100%', color: '#c62828', fontSize: 11, padding: '0 4px' }}>
            Cable mode active — pick a connection type or click devices on workspace
          </div>
        )}
        {devices.map((d) => (
          <button
            key={`${d.model}-${d.label}`}
            type="button"
            className="pt-device-item"
            title={d.description}
            onClick={() => onDevicePick(d.model, d.label)}
          >
            <img src={d.icon} width={48} height={48} alt={d.label} draggable={false} />
            <span>{d.label}</span>
          </button>
        ))}
      </div>
    </div>
  );
};
