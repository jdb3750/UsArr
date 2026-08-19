export const COVER_SEARCH_DEFAULT_PROVIDERS = ["duckduckgo", "itunes", "all"] as const;

export type CoverSearchDefaultProvider = (typeof COVER_SEARCH_DEFAULT_PROVIDERS)[number];

export const DEFAULT_COVER_SEARCH_PROVIDER: CoverSearchDefaultProvider = "duckduckgo";

export interface CoverSearchPreferences {
  defaultProvider: CoverSearchDefaultProvider;
}
