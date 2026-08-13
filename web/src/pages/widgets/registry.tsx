// Widget registry: every dashboard widget with its title and components.

import {
  CpuHeader,
  CpuWidget,
  DiskIOHeader,
  DiskIOWidget,
  MemoryHeader,
  MemoryWidget,
  NetworkHeader,
  NetworkWidget,
} from "./metrics";
import {
  FansWidget,
  MediaAppsWidget,
  MinecraftWidget,
  PlexWidget,
  ServicesWidget,
  StorageWidget,
  SystemWidget,
  TempsWidget,
} from "./apps";

export interface WidgetDef {
  title: string;
  component: () => JSX.Element | null;
  headerExtra?: () => JSX.Element | null;
  /** Smallest useful panel height in px (resize grabber lower bound). */
  minH?: number;
}

export const WIDGETS: Record<string, WidgetDef> = {
  cpu: { title: "CPU", component: CpuWidget, headerExtra: CpuHeader, minH: 170 },
  memory: { title: "Memory", component: MemoryWidget, headerExtra: MemoryHeader, minH: 170 },
  network: { title: "Network", component: NetworkWidget, headerExtra: NetworkHeader, minH: 170 },
  diskio: { title: "Disk I/O", component: DiskIOWidget, headerExtra: DiskIOHeader, minH: 170 },
  system: { title: "System", component: SystemWidget, minH: 140 },
  temps: { title: "Temperatures", component: TempsWidget, minH: 110 },
  fans: { title: "Fans", component: FansWidget, minH: 110 },
  storage: { title: "Storage", component: StorageWidget, minH: 150 },
  minecraft: { title: "Minecraft", component: MinecraftWidget, minH: 100 },
  services: { title: "Services", component: ServicesWidget, minH: 110 },
  plex: { title: "Plex", component: PlexWidget, minH: 100 },
  apps: { title: "Media apps", component: MediaAppsWidget, minH: 100 },
};
