// Mirrors of the Go API JSON types.

export interface SystemInfo {
  hostname: string;
  os: string;
  kernel: string;
  arch: string;
  cpuModel: string;
  cpuCores: number;
  memTotal: number;
  bootTime: number;
  version: string;
  mock: boolean;
}

export interface NetRate {
  name: string;
  rxBps: number;
  txBps: number;
  rxTotal: number;
  txTotal: number;
}

export interface DiskRate {
  name: string;
  readBps: number;
  writeBps: number;
  utilPct: number;
}

export interface Temp {
  label: string;
  c: number;
}

export interface Snapshot {
  ts: number;
  cpu: number;
  perCore: number[];
  load: [number, number, number];
  memUsed: number;
  memTotal: number;
  memCached: number;
  swapUsed: number;
  swapTotal: number;
  net: NetRate[];
  disk: DiskRate[];
  temps: Temp[];
}

// Storage

export interface Branch {
  path: string;
  device: string;
  total: number;
  used: number;
}

export interface Pool {
  name: string;
  mount: string;
  fsType: string;
  total: number;
  used: number;
  branches: Branch[];
}

export interface MountInfo {
  mount: string;
  device: string;
  fsType: string;
  total: number;
  used: number;
}

export interface Smart {
  available: boolean;
  healthy?: boolean;
  powerOnHours?: number;
  reallocated?: number;
  pendingSectors?: number;
  crcErrors?: number;
  percentUsed?: number;
  mediaErrors?: number;
}

export interface Disk {
  name: string;
  device: string;
  model: string;
  serial?: string;
  sizeBytes: number;
  rotational: boolean;
  tempC?: number;
  smart: Smart;
}

export interface StorageOverview {
  pools: Pool[] | null;
  disks: Disk[] | null;
  mounts: MountInfo[] | null;
}

export interface SnapraidInfo {
  configured: boolean;
  installed: boolean;
  configPath?: string;
  parity?: string[];
  content?: string[];
  dataDisks?: string[];
}

// Services

export interface Service {
  unit: string;
  description: string;
  loadState: string;
  activeState: string;
  subState: string;
  since?: number;
  pid?: number;
  memBytes?: number;
  enabled?: string;
}

export interface ServicesResponse {
  services: Service[] | null;
  allowActions: boolean;
}

// Plex

export interface PlexSession {
  user: string;
  title: string;
  grandparent?: string;
  type: string;
  player: string;
  product: string;
  state: string;
  progressMs: number;
  durationMs: number;
  decision: string;
  bitrateKbps?: number;
}

export interface PlexLibrary {
  title: string;
  type: string;
  count: number;
}

export interface PlexStatus {
  configured: boolean;
  reachable: boolean;
  error?: string;
  version?: string;
  sessions: PlexSession[];
  libraries: PlexLibrary[];
}

// Minecraft

export type MCState = "stopped" | "starting" | "running" | "stopping" | "crashed";

export interface MCServer {
  id: string;
  name: string;
  dir: string;
  state: MCState;
  version?: string;
  software?: string;
  port?: number;
  onlinePlayers: string[] | null;
  maxPlayers?: number;
  startedAt?: number;
  pid?: number;
  cpuPct: number;
  memBytes: number;
  memMax?: number;
  mem?: string;
  java?: string;
  jar?: string;
  jvmArgs?: string[];
  aikar: boolean;
  autoStart: boolean;
  autoRestart: boolean;
  eulaAccepted: boolean;
  rconEnabled: boolean;
  lastExit?: string;
}

export interface LogLine {
  ts: number;
  level: string;
  text: string;
}

export interface PropEntry {
  key: string;
  value: string;
}

export interface BackupInfo {
  name: string;
  sizeBytes: number;
  createdAt: number;
}

export interface NamedPlayer {
  name: string;
  uuid?: string;
  level?: number;
  reason?: string;
}

export interface PlayerInfo {
  online: string[] | null;
  maxPlayers: number;
  whitelistEnabled: boolean;
  whitelist: NamedPlayer[] | null;
  ops: NamedPlayer[] | null;
  banned: NamedPlayer[] | null;
}

// Jobs / audit

export interface Job {
  id: string;
  kind: string;
  target: string;
  startedAt: number;
  endedAt?: number;
  state: "running" | "done" | "failed" | "canceled";
  err?: string;
  output?: string[];
  dropped?: number;
}

export interface AuditEntry {
  ts: number;
  ip: string;
  action: string;
  target: string;
  detail?: string;
  ok: boolean;
  err?: string;
}

export interface AuditResponse {
  entries: AuditEntry[] | null;
  total: number;
}
