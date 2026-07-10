import { useEffect, useRef } from "react";

export type ReloadForVersion = (version: string) => void;

export function pageURLForVersion(href: string, version: string): string {
  const url = new URL(href);
  url.searchParams.set("_agentx_version", version);
  return url.toString();
}

export function reloadPageForVersion(version: string): void {
  window.location.replace(pageURLForVersion(window.location.href, version));
}

export function useReloadOnServerVersionChange(
  version: string | undefined,
  reload: ReloadForVersion = reloadPageForVersion
): void {
  const loadedVersion = useRef<string | undefined>(undefined);

  useEffect(() => {
    if (!version) {
      return;
    }
    if (loadedVersion.current === undefined) {
      loadedVersion.current = version;
      return;
    }
    if (loadedVersion.current !== version) {
      loadedVersion.current = version;
      reload(version);
    }
  }, [reload, version]);
}
