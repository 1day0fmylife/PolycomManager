import type { AuditEntry, Device, DeviceInput } from './types'

async function request<T>(url: string, init?: RequestInit): Promise<T> {
  const res = await fetch(url, { ...init, headers: { 'Content-Type': 'application/json', ...(init?.headers || {}) } })
  if (!res.ok) {
    const payload = await res.json().catch(() => ({ error: res.statusText }))
    throw new Error(payload.error || res.statusText)
  }
  if (res.status === 204) return undefined as T
  return res.json()
}

export const api = {
  devices: () => request<Device[]>('/api/v1/devices'),
  create: (v: DeviceInput) => request<Device>('/api/v1/devices', { method: 'POST', body: JSON.stringify(v) }),
  update: (id: string, v: DeviceInput) => request<Device>(`/api/v1/devices/${id}`, { method: 'PUT', body: JSON.stringify(v) }),
  remove: (id: string) => request<void>(`/api/v1/devices/${id}`, { method: 'DELETE' }),
  connect: (id: string) => request(`/api/v1/devices/${id}/connect`, { method: 'POST' }),
  disconnect: (id: string) => request(`/api/v1/devices/${id}/disconnect`, { method: 'POST' }),
  dial: (id: string, destination: string, speed = 512) => request(`/api/v1/devices/${id}/dial`, { method: 'POST', body: JSON.stringify({ destination, speed }) }),
  hangup: (id: string) => request(`/api/v1/devices/${id}/hangup`, { method: 'POST' }),
  mute: (id: string, muted: boolean) => request(`/api/v1/devices/${id}/mute`, { method: 'POST', body: JSON.stringify({ muted }) }),
  volume: (id: string, volume: number) => request(`/api/v1/devices/${id}/volume`, { method: 'POST', body: JSON.stringify({ volume }) }),
  command: (id: string, command: string) => request<{ output: string[] }>(`/api/v1/devices/${id}/command`, { method: 'POST', body: JSON.stringify({ command }) }),
  audit: () => request<AuditEntry[]>('/api/v1/audit?limit=200'),
}
