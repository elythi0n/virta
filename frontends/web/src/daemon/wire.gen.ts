// Generated from the Go wire types by cmd/apigen. Do not edit by hand.
// Run `make apigen` after changing the daemon's API structs.

export interface Annotations {
  hidden?: boolean;
  highlight?: string;
  masked?: boolean;
  first_time?: boolean;
}

export interface AuthSession {
  id: string;
  authorize_url: string;
  state: string;
  login?: string;
  error?: string;
}

export interface Author {
  id: string;
  login: string;
  display_name: string;
  color: string;
  badges?: Badge[];
}

export interface Badge {
  set: string;
  version: string;
  title?: string;
}

export interface Capabilities {
  read_anonymous: boolean;
  read_authed: boolean;
  send: boolean;
  moderation: boolean;
  replies: boolean;
  held_queue: boolean;
  stability: string;
}

export interface ChannelInfo {
  platform: string;
  slug: string;
  state: string;
  reason?: string;
}

export interface ChannelRef {
  platform: Platform;
  id: string;
  slug: string;
  display_name?: string;
}

export interface ChatSettings {
  emote_only: boolean;
  subs_only: boolean;
  unique_chat: boolean;
  followers_only_minutes: number;
  slow_seconds: number;
}

export interface DeviceSession {
  id: string;
  user_code: string;
  verification_uri: string;
  expires_in: number;
  interval: number;
  state: string;
  login?: string;
  error?: string;
}

export interface Discovery {
  addr: string;
  token: string;
}

export interface EmoteCount {
  name: string;
  count: number;
}

export type EmoteProvider = string;

export interface EmoteRef {
  provider: EmoteProvider;
  id: string;
  name: string;
  url_template: string;
  animated: boolean;
}

export type HealthState = string;

export interface HealthStatus {
  state: HealthState;
  reason?: ReasonCode;
  detail?: string;
}

export interface MessageRef {
  platform_message_id: string;
  author_login: string;
  text_snippet: string;
}

export type MessageType = string;

export type Platform = string;

export type ReasonCode = string;

export interface Segment {
  kind: SegmentKind;
  text: string;
  emote?: EmoteRef;
  cheer_bits?: number;
  reveal?: string;
}

export type SegmentKind = string;

export interface StatsSnapshot {
  window_seconds: number;
  messages_per_sec: number;
  unique_chatters: number;
  top_emotes?: EmoteCount[];
}

export interface UnifiedMessage {
  id: string;
  platform_message_id: string;
  platform: Platform;
  channel: ChannelRef;
  type: MessageType;
  author: Author;
  segments: Segment[];
  reply_to?: MessageRef;
  sent_at: string;
  received_at: string;
  annotations?: Annotations;
}

export interface WireEvent {
  type: string;
  schema_version: number;
  seq: number;
  message?: UnifiedMessage;
  channel?: ChannelRef;
  platform_message_id?: string;
  message_id?: string;
  target_user_id?: string;
  state?: HealthStatus;
  settings?: ChatSettings;
  stats?: StatsSnapshot;
  profile_id?: string;
  profile_name?: string;
}
