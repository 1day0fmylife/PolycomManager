import { FormEvent, useCallback, useEffect, useMemo, useState } from 'react'
import { api } from './api'
import type { AuditEntry, Device, DeviceInput } from './types'

const empty: DeviceInput = { name: '', host: '', port: 22, model: 'auto', location: '', username: 'admin', password: '', enabled: true }

function statusClass(state?: string) {
  if (state === 'connected') return 'ok'
  if (state === 'connecting' || state === 'reconnecting') return 'warn'
  if (state === 'auth_error') return 'bad'
  return 'off'
}

function StateDot({ state }: { state?: string }) {
  return <span className={`dot ${statusClass(state)}`} aria-hidden="true" />
}

function DeviceModal({ current, onClose, onSaved }: { current?: Device; onClose: () => void; onSaved: () => void }) {
  const [form, setForm] = useState<DeviceInput>(() => current ? {
    name: current.name, host: current.host, port: current.port, model: current.model, location: current.location || '', username: current.username, password: '', enabled: current.enabled,
  } : empty)
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState('')
  const set = <K extends keyof DeviceInput>(key: K, value: DeviceInput[K]) => setForm(v => ({ ...v, [key]: value }))
  async function submit(e: FormEvent) {
    e.preventDefault(); setBusy(true); setError('')
    try { current ? await api.update(current.id, form) : await api.create(form); onSaved(); onClose() }
    catch (e) { setError(e instanceof Error ? e.message : String(e)) }
    finally { setBusy(false) }
  }
  return <div className="modal-backdrop" onMouseDown={e => e.target === e.currentTarget && onClose()}>
    <form className="modal" onSubmit={submit}>
      <div className="modal-head"><div><h2>{current ? 'Редактировать терминал' : 'Добавить терминал'}</h2><p>RealPresence Group 300 / 500 / 700</p></div><button className="icon-button" type="button" onClick={onClose}>×</button></div>
      <div className="form-grid">
        <label>Название<input value={form.name} onChange={e => set('name', e.target.value)} required /></label>
        <label>Расположение<input value={form.location} onChange={e => set('location', e.target.value)} placeholder="Переговорная 201" /></label>
        <label>IP / hostname<input value={form.host} onChange={e => set('host', e.target.value)} required /></label>
        <label>SSH порт<input type="number" min="1" max="65535" value={form.port} onChange={e => set('port', Number(e.target.value))} /></label>
        <label>Модель<select value={form.model} onChange={e => set('model', e.target.value)}><option value="auto">Определить автоматически</option><option value="group300">Group 300</option><option value="group500">Group 500</option><option value="group700">Group 700</option></select></label>
        <label>SSH пользователь<input value={form.username} onChange={e => set('username', e.target.value)} required /></label>
        <label className="wide">{current ? 'Новый пароль (оставьте пустым, чтобы не менять)' : 'SSH пароль'}<input type="password" value={form.password} onChange={e => set('password', e.target.value)} required={!current} /></label>
        <label className="check wide"><input type="checkbox" checked={form.enabled} onChange={e => set('enabled', e.target.checked)} /> Автоподключение</label>
      </div>
      {error && <div className="error-box">{error}</div>}
      <div className="modal-actions"><button type="button" className="button secondary" onClick={onClose}>Отмена</button><button className="button primary" disabled={busy}>{busy ? 'Сохранение…' : 'Сохранить'}</button></div>
    </form>
  </div>
}

function DevicePanel({ device, onClose, onEdit, refresh }: { device: Device; onClose: () => void; onEdit: () => void; refresh: () => void }) {
  const [destination, setDestination] = useState('')
  const [command, setCommand] = useState('status')
  const [terminal, setTerminal] = useState<string[]>([])
  const [error, setError] = useState('')
  const [busy, setBusy] = useState('')
  const act = async (name: string, fn: () => Promise<unknown>) => { setBusy(name); setError(''); try { await fn(); setTimeout(refresh, 350) } catch (e) { setError(e instanceof Error ? e.message : String(e)) } finally { setBusy('') } }
  const r = device.runtime
  const connected = r.connection === 'connected'
  async function raw(e: FormEvent) { e.preventDefault(); if (!command.trim()) return; await act('raw', async () => { const out = await api.command(device.id, command); setTerminal(v => [...v, `> ${command}`, ...out.output].slice(-100)); setCommand('') }) }
  return <aside className="drawer">
    <div className="drawer-head"><div><div className="eyeline">{device.location || 'Терминал'}</div><h2>{device.name}</h2><div className="device-address">{device.host}:{device.port}</div></div><button className="icon-button" onClick={onClose}>×</button></div>
    <div className="status-banner"><StateDot state={r.connection}/><strong>{r.connection || 'unknown'}</strong>{r.last_error && <span title={r.last_error}> — {r.last_error}</span>}</div>
    <div className="facts">
      <div><span>Модель</span><b>{r.detected_model || device.model}</b></div><div><span>Система</span><b>{r.system_name || '—'}</b></div>
      <div><span>Firmware</span><b>{r.firmware || '—'}</b></div><div><span>Serial</span><b>{r.serial || '—'}</b></div>
      <div><span>Вызов</span><b>{r.call_state || '—'}</b></div><div><span>Удалённая сторона</span><b>{r.remote_party || '—'}</b></div>
    </div>
    <div className="section">
      <div className="section-title">Подключение</div><div className="button-row"><button className="button secondary" disabled={busy!==''} onClick={() => act('connect',()=>api.connect(device.id))}>Подключить</button><button className="button secondary" disabled={busy!==''} onClick={() => act('disconnect',()=>api.disconnect(device.id))}>Отключить</button><button className="button secondary" onClick={onEdit}>Изменить</button></div>
    </div>
    <div className="section">
      <div className="section-title">Вызов</div><div className="inline-form"><input value={destination} onChange={e=>setDestination(e.target.value)} placeholder="sip:1001@example.local"/><button className="button primary" disabled={!connected||busy!==''||!destination} onClick={()=>act('dial',()=>api.dial(device.id,destination))}>Вызов</button></div>
      <div className="button-row"><button className="button danger" disabled={!connected||busy!==''} onClick={()=>act('hangup',()=>api.hangup(device.id))}>Завершить</button><button className="button secondary" disabled={!connected||busy!==''} onClick={()=>act('mute',()=>api.mute(device.id,!r.muted))}>{r.muted?'Включить микрофон':'Mute'}</button></div>
      <label className="range">Громкость <b>{r.volume ?? 25}</b><input type="range" min="0" max="50" value={r.volume ?? 25} disabled={!connected||busy!==''} onChange={e=>act('volume',()=>api.volume(device.id,Number(e.target.value)))}/></label>
    </div>
    <div className="section terminal-section"><div className="section-title">API Console</div><div className="terminal">{terminal.length ? terminal.map((l,i)=><div key={i}>{l}</div>) : <span className="terminal-muted">Команды выполняются через текущую SSH API-сессию.</span>}</div><form className="terminal-input" onSubmit={raw}><span>&gt;</span><input value={command} onChange={e=>setCommand(e.target.value)} placeholder="status" disabled={!connected}/><button disabled={!connected||busy!==''}>Run</button></form></div>
    {error && <div className="error-box drawer-error">{error}</div>}
  </aside>
}

function DevicesTable({ devices, onOpen, onEdit }: { devices: Device[]; onOpen: (d: Device) => void; onEdit: (d: Device) => void }) {
  return <div className="table-wrap"><table><thead><tr><th>Терминал</th><th>Модель</th><th>Адрес</th><th>SSH</th><th>Вызов</th><th>Последняя активность</th><th aria-label="Действия" /></tr></thead><tbody>{devices.map(d=><tr key={d.id} onClick={()=>onOpen(d)}><td><strong>{d.name}</strong><span>{d.location||'Без расположения'}</span></td><td>{d.runtime.detected_model||d.model}</td><td className="mono">{d.host}:{d.port}</td><td><span className="status-cell"><StateDot state={d.runtime.connection}/>{d.runtime.connection}</span></td><td>{d.runtime.call_state||'—'}{d.runtime.remote_party&&<span>{d.runtime.remote_party}</span>}</td><td>{d.runtime.last_seen_at?new Date(d.runtime.last_seen_at).toLocaleString():'—'}</td><td className="row-actions"><button className="button secondary compact" onClick={e=>{e.stopPropagation();onEdit(d)}}>Изменить</button></td></tr>)}</tbody></table>{devices.length===0&&<div className="empty">Терминалы ещё не добавлены.</div>}</div>
}

function Audit({ entries }: { entries: AuditEntry[] }) {
  return <div className="table-wrap"><table><thead><tr><th>Время</th><th>Операция</th><th>Device ID</th><th>Результат</th><th>Детали</th></tr></thead><tbody>{entries.map(a=><tr key={a.id}><td>{new Date(a.created_at).toLocaleString()}</td><td className="mono">{a.operation}</td><td className="mono subtle">{a.device_id||'—'}</td><td><span className={`result ${a.result==='success'?'success':'failure'}`}>{a.result}</span></td><td className="truncate" title={a.detail||a.command}>{a.detail||a.command||'—'}</td></tr>)}</tbody></table></div>
}

export function App() {
  const [devices,setDevices]=useState<Device[]>([]), [audit,setAudit]=useState<AuditEntry[]>([])
  const [page,setPage]=useState<'devices'|'audit'>('devices'), [selected,setSelected]=useState<string>(), [modal,setModal]=useState<'new'|'edit'>()
  const [error,setError]=useState('')
  const load=useCallback(async()=>{try{setDevices(await api.devices());setError('')}catch(e){setError(e instanceof Error?e.message:String(e))}},[])
  const loadAudit=useCallback(async()=>{try{setAudit(await api.audit())}catch(e){setError(e instanceof Error?e.message:String(e))}},[])
  useEffect(()=>{load(); const t=setInterval(load,5000); const es=new EventSource('/api/v1/events'); es.onmessage=()=>load(); return()=>{clearInterval(t);es.close()}},[load])
  useEffect(()=>{if(page==='audit')loadAudit()},[page,loadAudit])
  const active=useMemo(()=>devices.find(d=>d.id===selected),[devices,selected])
  const online=devices.filter(d=>d.runtime.connection==='connected').length, calls=devices.filter(d=>d.runtime.call_state&&d.runtime.call_state!=='idle').length
  return <div className="app-shell">
    <aside className="sidebar"><div className="brand"><div className="brand-mark">P</div><div><b>Polycom</b><span>Manager</span></div></div><nav><button className={page==='devices'?'active':''} onClick={()=>setPage('devices')}><span>▦</span>Терминалы</button><button className={page==='audit'?'active':''} onClick={()=>setPage('audit')}><span>≡</span>Аудит</button></nav><div className="sidebar-foot"><span>Group 300 / 500 / 700</span><small>SSH Secure API</small></div></aside>
    <main><header><div><h1>{page==='devices'?'Терминалы':'Журнал аудита'}</h1><p>{page==='devices'?'Управление RealPresence Group Series':'История подключений и команд'}</p></div>{page==='devices'&&<button className="button primary" onClick={()=>setModal('new')}>+ Добавить терминал</button>}</header>
      {error&&<div className="error-box page-error">{error}</div>}
      {page==='devices'?<><div className="metrics"><div><span>Всего</span><b>{devices.length}</b></div><div><span>Подключено</span><b>{online}</b></div><div><span>Активные вызовы</span><b>{calls}</b></div><div><span>Ошибки</span><b>{devices.filter(d=>d.runtime.connection==='auth_error'||d.runtime.connection==='error').length}</b></div></div><section className="content-card"><div className="content-title"><div><h2>Устройства</h2><p>Состояние обновляется автоматически</p></div><button className="button secondary" onClick={load}>Обновить</button></div><DevicesTable devices={devices} onOpen={d=>setSelected(d.id)} onEdit={d=>{setSelected(d.id);setModal('edit')}}/></section></>:<section className="content-card"><div className="content-title"><div><h2>Последние операции</h2><p>До 200 последних записей</p></div><button className="button secondary" onClick={loadAudit}>Обновить</button></div><Audit entries={audit}/></section>}
    </main>
    {active&&<DevicePanel device={active} onClose={()=>setSelected(undefined)} onEdit={()=>setModal('edit')} refresh={load}/>} 
    {modal&&<DeviceModal current={modal==='edit'?active:undefined} onClose={()=>setModal(undefined)} onSaved={load}/>} 
  </div>
}
