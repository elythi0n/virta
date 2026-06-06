export { createDaemonClient } from './client';
export type { ConnectionStatus, DaemonClient, DaemonClientOptions } from './client';
export { useDaemonStream } from './useDaemonStream';
export { toFeedMessage } from './normalize';
export { discover, resetDiscovery } from './discovery';
export { listChannels, joinChannel, leaveChannel, getCapabilities, DaemonUnreachableError } from './api';
export { useChannels } from './useChannels';
export type { ChannelsStatus } from './useChannels';
