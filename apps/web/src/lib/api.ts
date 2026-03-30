const DEFAULT_API_BASE = "/v1";

export async function apiGet<T>(path: string): Promise<T> {
  const response = await fetch(buildApiURL(path), {
    headers: {
      Accept: "application/json",
    },
  });

  if (!response.ok) {
    throw new Error(`request failed: ${response.status}`);
  }

  return response.json() as Promise<T>;
}

function buildApiURL(path: string): string {
  if (path.startsWith("http://") || path.startsWith("https://")) {
    return path;
  }
  if (path.startsWith("/v1/")) {
    return path;
  }
  if (path.startsWith("/")) {
    return `${DEFAULT_API_BASE}${path}`;
  }
  return `${DEFAULT_API_BASE}/${path}`;
}
