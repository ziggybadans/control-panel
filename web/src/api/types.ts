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

export interface FeatureInfo {
  smart: boolean;
  snapraid: boolean;
  plex: boolean;
  apps: number;
  minecraftRoot: string;
  minecraftRunAs?: string;
  power: boolean;
  updateRepo?: string;
  fanControl: boolean;
}

export interface PanelInfo {
  goVersion: string;
  pid: number;
  startedAt: number;
  dataDir: string;
  listen: string;
  tls: boolean;
  authMode: string;
  sessionHours: number;
  trustedProxies: number;
  features: FeatureInfo;
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

// Fans

export interface FanState {
  id: string;
  label: string;
  rpm: number; // -1 = no tach
  dutyPct: number;
  mode: "auto" | "manual" | "curve";
  targetPct?: number;
  sourceTempC?: number;
  failsafe?: boolean;
  err?: string;
}

export interface FanSensor {
  id: string;
  label: string;
  c: number;
}

export interface FanCurvePoint {
  tempC: number;
  dutyPct: number;
}

export interface FanSettings {
  mode: "auto" | "manual" | "curve";
  manualPct?: number;
  sensor?: string;
  points?: FanCurvePoint[];
}

export interface FansLive {
  fans: FanState[] | null;
  sensors: FanSensor[] | null;
}

export interface FansSnapshot {
  supported: boolean;
  control: boolean;
  fans: FanState[] | null;
  sensors: FanSensor[] | null;
  settings: Record<string, FanSettings> | null;
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

// Media apps (*arr suite, Overseerr/Jellyseerr)

export interface AppQueueItem {
  title: string;
  status: string;
  progress: number;
  timeLeft?: string;
}

export interface AppRequestCounts {
  pending: number;
  approved: number;
  processing: number;
  available: number;
  total: number;
}

export interface AppStatus {
  name: string;
  type: string;
  url: string;
  reachable: boolean;
  error?: string;
  version?: string;
  healthIssues: string[];
  queueCount: number;
  queue: AppQueueItem[];
  missing?: number;
  upcomingWeek?: number;
  requests?: AppRequestCounts;
}

export interface AppsResponse {
  configured: boolean;
  apps: AppStatus[] | null;
}

// Minecraft file manager / addons / map / setup

export interface FileEntry {
  name: string;
  dir: boolean;
  size: number;
  modTime: number;
}

export interface FilesResponse {
  path: string;
  entries: FileEntry[] | null;
}

export interface Addon {
  file: string;
  dir: string; // "plugins" | "mods"
  name: string;
  version?: string;
  enabled: boolean;
  sizeBytes: number;
}

export interface MapInfo {
  detected: boolean;
  type?: string;
  port?: number;
}

export interface MCCreateSpec {
  id: string;
  flavor: string;
  version: string;
  mem: string;
  port: number;
  motd: string;
  acceptEula: boolean;
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

export interface UpdateRelease {
  tag: string;
  notes: string;
  publishedAt: string;
  assetSize: number;
}

export interface UpdateStatus {
  configured: boolean;
  repo?: string;
  current: string;
  latest?: UpdateRelease | null;
  updateAvailable: boolean;
  error?: string;
}
