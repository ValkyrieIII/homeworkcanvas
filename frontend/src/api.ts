const base = import.meta.env.VITE_API_BASE ?? "http://localhost:8080/api/v1";
// HTTP headers are byte-oriented; keep the local fallback identity ASCII-safe.
const headers = () => ({
  "X-Canvas-Subject": localStorage.canvasSubject || "dev-user",
  "X-Canvas-Name": localStorage.canvasName || "Local Developer",
});
export async function api<T>(path: string, init: RequestInit = {}): Promise<T> {
  const isForm = init.body instanceof FormData;
  const response = await fetch(base + path, {
    ...init,
    headers: {
      ...(isForm ? {} : { "Content-Type": "application/json" }),
      ...headers(),
      ...init.headers,
    },
  });
  if (!response.ok) {
    const body = await response.json().catch(() => null);
    throw new Error(body?.error?.message ?? "请求失败");
  }
  return response.json();
}
