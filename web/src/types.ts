export type RuntimeCall = {
  call_id: string
  far_site_name?: string
  far_site_number?: string
  speed?: string
  connection_status?: string
  mute_status?: string
  direction?: string
  type?: string
  protocol?: string
  started_at?: string
  duration_seconds?: number
}

export type RuntimeState = {
  connection: string
  detected_model?: string
  system_name?: string
  firmware?: string
  serial?: string
  call_state?: string
  remote_party?: string
  calls?: RuntimeCall[]
  muted?: boolean
  volume?: number
  content_state?: string
  content_source?: number
  near_camera_source?: number
  far_camera_source?: number
  last_error?: string
  last_seen_at?: string
  last_connected_at?: string
}

export type Device = {
  id: string
  name: string
  host: string
  port: number
  model: string
  location?: string
  username: string
  enabled: boolean
  created_at: string
  updated_at: string
  runtime: RuntimeState
}

export type DeviceInput = {
  name: string
  host: string
  port: number
  model: string
  location: string
  username: string
  password: string
  enabled: boolean
}

export type AuditEntry = {
  id: number
  device_id?: string
  operation: string
  command?: string
  result: string
  detail?: string
  created_at: string
}
