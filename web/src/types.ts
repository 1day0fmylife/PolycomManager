export type RuntimeState = {
  connection: string
  detected_model?: string
  system_name?: string
  firmware?: string
  serial?: string
  call_state?: string
  remote_party?: string
  muted?: boolean
  volume?: number
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
