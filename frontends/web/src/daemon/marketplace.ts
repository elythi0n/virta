import { request } from './http';

export interface MarketplacePlugin {
  id: string;
  name: string;
  version: string;
  description?: string;
  publisher?: string;
  icon?: string;
  tags?: string[];
  install_url: string;
  verified: boolean;
}

export function listMarketplace(): Promise<MarketplacePlugin[]> {
  return request<{ plugins: MarketplacePlugin[] }>('/v1/marketplace')
    .then(r => r.plugins ?? []);
}
