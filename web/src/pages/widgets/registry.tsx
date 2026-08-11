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
}

export const WIDGETS: Record<string, WidgetDef> = {
  cpu: { title: "CPU", component: CpuWidget, headerExtra: CpuHeader },
  memory: { title: "Memory", component: MemoryWidget, headerExtra: MemoryHeader },
  network: { title: "Network", component: NetworkWidget, headerExtra: NetworkHeader },
  diskio: { title: "Disk I/O", component: DiskIOWidget, headerExtra: DiskIOHeader },
  system: { title: "System", component: SystemWidget },
  temps: { title: "Temperatures", component: TempsWidget },
  storage: { title: "Storage", component: StorageWidget },
  minecraft: { title: "Minecraft", component: MinecraftWidget },
  services: { title: "Services", component: ServicesWidget },
  plex: { title: "Plex", component: PlexWidget },
};
