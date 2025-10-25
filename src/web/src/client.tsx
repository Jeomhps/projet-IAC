export async function apiGet(path: string, token?: string | null) {
  const resp = await fetch(`/api${path}`, {
    headers: token ? { Authorization: `Bearer ${token}` } : undefined
  });
  if (!resp.ok) throw new Error(`${resp.status}`);
  return resp.json();
}

export async function apiPost(path: string, body: any, token: string) {
  const resp = await fetch(`/api${path}`, {
    method: "POST",
    headers: {
      "Content-Type": "application/json",
      Authorization: `Bearer ${token}`
    },
    body: JSON.stringify(body)
  });
  if (!resp.ok) throw new Error(await resp.text());
  return resp.json();
}

export async function apiDelete(path: string, token: string) {
  const resp = await fetch(`/api${path}`, {
    method: "DELETE",
    headers: {
      Authorization: `Bearer ${token}`
    }
  });
  if (!resp.ok) throw new Error(await resp.text());
  return resp.json().catch(() => ({}));
}
