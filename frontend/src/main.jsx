import React, { createContext, useContext, useEffect, useMemo, useState } from 'react';
import {
  BrowserRouter,
  Link,
  Navigate,
  NavLink,
  Outlet,
  Route,
  Routes,
  useNavigate,
} from 'react-router-dom';
import { createRoot } from 'react-dom/client';
import Hls from 'hls.js';
import './styles.css';

const apiBaseUrl = import.meta.env.VITE_API_BASE_URL || '';
const AuthContext = createContext(null);

async function api(path, options = {}) {
  const response = await fetch(`${apiBaseUrl}${path}`, {
    credentials: 'include',
    headers: { 'Content-Type': 'application/json', ...(options.headers || {}) },
    ...options,
  });
  const text = await response.text();
  const data = text ? JSON.parse(text) : null;
  if (!response.ok) {
    const err = new Error(data?.error?.message || `Request failed: ${response.status}`);
    err.code = data?.error?.code;
    err.status = response.status;
    throw err;
  }
  return data;
}

function useAuth() {
  return useContext(AuthContext);
}

function App() {
  const [user, setUser] = useState(null);
  const [loading, setLoading] = useState(true);

  async function loadUser() {
    try {
      const data = await api('/api/auth/me');
      setUser(data.user);
    } catch {
      setUser(null);
    } finally {
      setLoading(false);
    }
  }

  useEffect(() => {
    loadUser();
  }, []);

  const value = useMemo(() => ({ user, setUser, reload: loadUser }), [user]);

  if (loading) return <FullPageMessage text="Loading session..." />;

  return (
    <AuthContext.Provider value={value}>
      <BrowserRouter>
        <Routes>
          <Route path="/login" element={<LoginPage />} />
          <Route path="/" element={<Protected><Shell /></Protected>}>
            <Route index element={<Navigate to="/live" replace />} />
            <Route path="storage" element={<AdminOnly><StoragePage /></AdminOnly>} />
            <Route path="cameras" element={<AdminOnly><CamerasPage /></AdminOnly>} />
            <Route path="users" element={<AdminOnly><UsersPage /></AdminOnly>} />
            <Route path="permissions" element={<AdminOnly><PermissionsPage /></AdminOnly>} />
            <Route path="health" element={<AdminOnly><HealthDashboardPage /></AdminOnly>} />
            <Route path="alerts" element={<AdminOnly><AlertsPage /></AdminOnly>} />
            <Route path="layouts" element={<AdminOnly><LayoutsPage /></AdminOnly>} />
            <Route path="live" element={<LiveLayoutPage />} />
            <Route path="playback" element={<PlaybackPage />} />
          </Route>
        </Routes>
      </BrowserRouter>
    </AuthContext.Provider>
  );
}

function Protected({ children }) {
  const { user } = useAuth();
  if (!user) return <Navigate to="/login" replace />;
  return children;
}

function AdminOnly({ children }) {
  const { user } = useAuth();
  if (user?.role !== 'admin') return <EmptyState title="Forbidden" body="Admin access is required for this section." />;
  return children;
}

function Shell() {
  const { user, setUser } = useAuth();
  const navigate = useNavigate();

  async function logout() {
    await api('/api/auth/logout', { method: 'POST', body: '{}' });
    setUser(null);
    navigate('/login');
  }

  return (
    <div className="shell">
      <aside className="sidebar">
        <h1>DT Camera</h1>
        <p className="muted sidebar-tag">Signed in as {user.display_name}</p>
        <nav className="nav-group">
          <NavLink to="/" end>Home</NavLink>
        </nav>
        <nav className="nav-group">
          <span className="nav-label">Workspace</span>
          <NavLink to="/live">Live</NavLink>
          <NavLink to="/playback">Playback</NavLink>
        </nav>
        {user.role === 'admin' && (
          <nav className="nav-group nav-group-admin">
            <span className="nav-label">Admin <AdminBadge /></span>
            <NavLink to="/layouts">Layouts</NavLink>
            <NavLink to="/storage">Storage</NavLink>
            <NavLink to="/cameras">Cameras</NavLink>
            <NavLink to="/users">Users</NavLink>
            <NavLink to="/permissions">Permissions</NavLink>
            <NavLink to="/health">Health</NavLink>
            <NavLink to="/alerts">Alerts</NavLink>
          </nav>
        )}
      </aside>
      <main className="content">
        <header className="topbar">
          <div>
            <strong>{user.display_name}</strong>
            <span>
              {user.email}
              {user.role === 'admin' && <AdminBadge />}
            </span>
          </div>
          <button onClick={logout}>Logout</button>
        </header>
        <Outlet />
      </main>
    </div>
  );
}

function LoginPage() {
  const { setUser } = useAuth();
  const navigate = useNavigate();
  const [form, setForm] = useState({ login: '', password: '' });
  const [error, setError] = useState('');
  const [busy, setBusy] = useState(false);

  async function submit(event) {
    event.preventDefault();
    setError('');
    setBusy(true);
    try {
      const data = await api('/api/auth/login', { method: 'POST', body: JSON.stringify(form) });
      setUser(data.user);
      navigate('/');
    } catch (err) {
      setError(err.message);
    } finally {
      setBusy(false);
    }
  }

  return (
    <main className="login-page">
      <form className="login-box" onSubmit={submit}>
        <h1>DT Camera</h1>
        <p className="muted">Sign in with your email or username.</p>
        <label>Login<input autoFocus value={form.login} onChange={(e) => setForm({ ...form, login: e.target.value })} disabled={busy} /></label>
        <label>Password<input type="password" value={form.password} onChange={(e) => setForm({ ...form, password: e.target.value })} disabled={busy} /></label>
        {error && <ErrorText message={error} />}
        <button type="submit" disabled={busy}>{busy ? 'Signing in…' : 'Login'}</button>
      </form>
    </main>
  );
}

function Dashboard() {
  return (
    <Panel title="System">
      <p>Use the navigation to manage cameras, layouts, live views, and playback.</p>
      <EmptyState title="No activity yet" body="Create storage, then a camera, then a layout to get started." />
    </Panel>
  );
}

function HealthDashboardPage() {
  const [data, setData] = useState({ health: null, recorder: null, storage: [], cameras: [], events: [], openAlerts: [] });
  const { loading, error, run } = useLoader(load);

  async function load() {
    const [health, recorder, storage, cameras, events, alerts, version] = await Promise.all([
      api('/healthz'),
      api('/api/recorder/status'),
      api('/api/storage-locations'),
      api('/api/cameras'),
      api('/api/events?limit=10'),
      api('/api/alerts?status=open&limit=25'),
      api('/api/version'),
    ]);
    setData({
      health,
      recorder,
      storage: storage.storage_locations || [],
      cameras: cameras.cameras || [],
      events: events.events || [],
      openAlerts: alerts.alerts || [],
      version,
    });
  }

  useEffect(() => { run(); }, []);

  async function acknowledgeAlert(alertItem) {
    await api(`/api/alerts/${alertItem.id}/acknowledge`, { method: 'POST', body: '{}' });
    run();
  }
  async function resolveAlert(alertItem) {
    await api(`/api/alerts/${alertItem.id}/resolve`, { method: 'POST', body: '{}' });
    run();
  }

  return (
    <Panel title="Health">
      <Toolbar>
        <button onClick={run}>Refresh</button>
      </Toolbar>
      <section className="build-info">
        <h3>Build &amp; Migrations</h3>
        {data.version ? (
          <DataTable
            rows={[{
              app_version: data.version.app_version,
              git_commit: data.version.git_commit || '—',
              build_time: data.version.build_time || '—',
              latest_migration: data.version.latest_migration ?? 0,
              migrations_applied: data.version.migrations_applied ?? 0,
            }]}
            columns={['app_version', 'git_commit', 'build_time', 'latest_migration', 'migrations_applied']}
          />
        ) : (
          !loading && <p className="muted">Build identity not available.</p>
        )}
      </section>
      <State loading={loading} error={error} />
      <section className="events-panel">
        <h3>Open alerts ({data.openAlerts.length})</h3>
        {data.openAlerts.length ? (
          <DataTable
            rows={data.openAlerts.map((alertItem) => ({
              opened_at: alertItem.opened_at ? new Date(alertItem.opened_at).toLocaleString() : '',
              rule_name: alertItem.rule_name,
              severity: <AlertSeverityBadge severity={alertItem.severity} />,
              status: <AlertStatusBadge status={alertItem.status} />,
              message: alertItem.message,
              actions: (
                <span className="row-actions">
                  {alertItem.status === 'open' && <button type="button" onClick={() => acknowledgeAlert(alertItem)}>Acknowledge</button>}
                  <button type="button" onClick={() => resolveAlert(alertItem)}>Resolve</button>
                </span>
              ),
            }))}
            columns={['opened_at', 'rule_name', 'severity', 'status', 'message', 'actions']}
          />
        ) : (
          !loading && <EmptyState title="No open alerts" body="All systems are within configured thresholds." />
        )}
      </section>
      <div className="dashboard-grid">
        <section>
          <h3>Backend</h3>
          <DataTable rows={data.health ? [data.health] : []} columns={['service', 'status', 'database', 'started_at']} />
        </section>
        <section>
          <h3>Recorder Heartbeats</h3>
          {data.recorder?.heartbeats?.length ? (
            <DataTable
              rows={data.recorder.heartbeats.map((row) => ({ ...row, status: <StatusBadge kind={recorderStatusKind(row)} text={row.status} /> }))}
              columns={['worker_id', 'status', 'active_job_count', 'last_seen_at']}
            />
          ) : (
            <EmptyState title="No recorder heartbeats" body="The recorder worker has not checked in yet." />
          )}
        </section>
        <section>
          <h3>Storage</h3>
          {data.storage.length ? (
            <DataTable rows={data.storage.map(formatStorageRow)} columns={['name', 'health_status', 'exists', 'writable', 'used_percent', 'free_bytes', 'latest_validation_error']} />
          ) : (
            <EmptyState title="No storage configured" body="Add a storage location to start recording." />
          )}
        </section>
        <section>
          <h3>Cameras</h3>
          {data.cameras.length ? (
            <DataTable rows={data.cameras.map(formatCameraRow)} columns={['name', 'enabled', 'recording_enabled', 'retention_days', 'storage_location_id']} />
          ) : (
            <EmptyState title="No cameras" body="Add a camera to begin capturing RTSP streams." />
          )}
        </section>
        <section>
          <h3>Recorder Jobs</h3>
          {data.recorder?.active_jobs?.length ? (
            <DataTable rows={data.recorder.active_jobs} columns={['camera_name', 'status', 'worker_id', 'last_error', 'updated_at']} />
          ) : (
            <EmptyState title="No recorder jobs" body="No ffmpeg processes are currently running." />
          )}
        </section>
        <section>
          <h3>Latest Segments</h3>
          {data.recorder?.last_segments?.length ? (
            <DataTable rows={data.recorder.last_segments} columns={['camera_name', 'status', 'start_time', 'end_time']} />
          ) : (
            <EmptyState title="No segments yet" body="Recorded segments will appear here once the recorder is running." />
          )}
        </section>
      </div>
      <section className="events-panel">
        <h3>Latest Events</h3>
        {data.events.length ? (
          <DataTable rows={data.events.map(formatEventRow)} columns={['created_at', 'severity', 'event_type', 'entity_type', 'message']} />
        ) : (
          <EmptyState title="No events" body="System events such as logins, CRUD, and recorder activity will appear here." />
        )}
      </section>
    </Panel>
  );
}

function StoragePage() {
  const [items, setItems] = useState([]);
  const [form, setForm] = useState({ name: '', container_path: '/recordings', enabled: true });
  const { loading, error, run } = useLoader(load);

  async function load() {
    const data = await api('/api/storage-locations');
    setItems(data.storage_locations || []);
  }
  useEffect(() => { run(); }, []);

  async function create(event) {
    event.preventDefault();
    await api('/api/storage-locations', { method: 'POST', body: JSON.stringify(form) });
    setForm({ name: '', container_path: '/recordings', enabled: true });
    run();
  }

  const summary = storageSummary(items);

  return (
    <Panel title="Storage Locations">
      <section className="storage-hero">
        <div>
          <h3>Recording storage health</h3>
          <p>Monitor mounted recording folders, disk usage, writeability, and validation errors before they interrupt recording.</p>
        </div>
        <button type="button" onClick={run} disabled={loading}>{loading ? 'Refreshing...' : 'Refresh health'}</button>
      </section>

      <div className="storage-summary-grid">
        <StorageMetric label="Locations" value={items.length} detail={`${summary.enabled} enabled`} />
        <StorageMetric label="Total capacity" value={formatBytes(summary.total)} detail={`${formatBytes(summary.used)} used`} />
        <StorageMetric label="Free space" value={formatBytes(summary.free)} detail={`${summary.usedPercent.toFixed(1)}% used overall`} />
        <StorageMetric label="Attention" value={summary.problemCount} detail="warnings or errors" tone={summary.problemCount ? 'warning' : 'ok'} />
      </div>

      <section className="storage-create-panel">
        <div className="section-heading">
          <h3>Add storage location</h3>
          <p>Use the container path mounted into backend and recorder, for example <code>/recordings</code>.</p>
        </div>
        <form className="storage-create-form" onSubmit={create}>
          <Field label="Display name" help="Shown in camera setup and health dashboards.">
            <input placeholder="Primary recordings" value={form.name} onChange={(e) => setForm({ ...form, name: e.target.value })} required />
          </Field>
          <Field label="Container path" help="Backend validates that this folder exists and is writable inside the container.">
            <input placeholder="/recordings" value={form.container_path} onChange={(e) => setForm({ ...form, container_path: e.target.value })} required />
          </Field>
          <label className="switch-row storage-enable-row">
            <input type="checkbox" checked={form.enabled} onChange={(e) => setForm({ ...form, enabled: e.target.checked })} />
            <span><strong>Enable immediately</strong><small>Allow cameras to use this location after validation passes.</small></span>
          </label>
          <div className="form-actions">
            <button>Create storage</button>
          </div>
        </form>
      </section>
      <State loading={loading} error={error} />
      {items.length ? (
        <div className="storage-card-grid">
          {items.map((item) => <StorageLocationCard key={item.id} item={item} />)}
        </div>
      ) : (
        !loading && <EmptyState title="No storage locations" body="Create a storage location to enable recording." />
      )}
    </Panel>
  );
}

function StorageMetric({ label, value, detail, tone = '' }) {
  return (
    <div className={`storage-metric ${tone ? `storage-metric-${tone}` : ''}`}>
      <span>{label}</span>
      <strong>{value}</strong>
      <small>{detail}</small>
    </div>
  );
}

function StorageLocationCard({ item }) {
  const usedPercent = clampPercent(item.used_percent);
  const status = storageStatus(item);
  const used = Number(item.used_bytes || 0);
  const free = Number(item.free_bytes || 0);
  const total = Number(item.total_bytes || used + free || 0);
  const path = item.container_path || 'No path configured';
  return (
    <section className={`storage-card storage-card-${status.kind}`}>
      <div className="storage-card-header">
        <div>
          <h3>{item.name}</h3>
          <p>{path}</p>
        </div>
        <StatusBadge kind={status.badge} text={status.label} />
      </div>
      <div className="storage-usage-row">
        <div className="storage-donut" style={{ '--used': `${usedPercent}%` }}>
          <span>{usedPercent.toFixed(0)}%</span>
        </div>
        <div className="storage-usage-main">
          <div className="storage-usage-label">
            <strong>{formatBytes(used)} used</strong>
            <span>{formatBytes(free)} free</span>
          </div>
          <div className="storage-usage-bar" aria-label={`${usedPercent.toFixed(1)} percent used`}>
            <span style={{ width: `${usedPercent}%` }} />
          </div>
          <small>{total ? `${formatBytes(total)} total capacity` : 'Capacity unavailable'}</small>
        </div>
      </div>
      <dl className="storage-health-grid">
        <div><dt>Enabled</dt><dd>{item.is_enabled ? 'Yes' : 'No'}</dd></div>
        <div><dt>Exists</dt><dd>{item.exists ? 'Yes' : 'No'}</dd></div>
        <div><dt>Writable</dt><dd>{item.writable ? 'Yes' : 'No'}</dd></div>
        <div><dt>Health</dt><dd>{item.health_status || 'unknown'}</dd></div>
      </dl>
      {item.latest_validation_error ? (
        <p className="storage-error">{item.latest_validation_error}</p>
      ) : !item.is_enabled ? (
        <p className="storage-muted">Storage is disabled. Enable it before assigning cameras for recording.</p>
      ) : (
        <p className="storage-ok">Validated and ready for recorder access.</p>
      )}
    </section>
  );
}

function CamerasPage() {
  const [cameras, setCameras] = useState([]);
  const [storage, setStorage] = useState([]);
  const [form, setForm] = useState(newCameraForm());
  const [editingId, setEditingId] = useState('');
  const [editForm, setEditForm] = useState(null);
  const [scanOpen, setScanOpen] = useState(false);
  const [manualAddOpen, setManualAddOpen] = useState(false);
  const [scanForm, setScanForm] = useState(newCameraScanForm());
  const [scanResult, setScanResult] = useState(null);
  const [scanning, setScanning] = useState(false);
  const [scanProgress, setScanProgress] = useState(null);
  const [onvifForms, setOnvifForms] = useState({});
  const [onvifBusy, setOnvifBusy] = useState({});
  const [onvifMessages, setOnvifMessages] = useState({});
  const [onvifPreviews, setOnvifPreviews] = useState({});
  const [cameraPreviews, setCameraPreviews] = useState({});
  const [cameraPreviewBusy, setCameraPreviewBusy] = useState({});
  const [cameraPreviewErrors, setCameraPreviewErrors] = useState({});
  const [cameraPreviewSources, setCameraPreviewSources] = useState({});
  const [configuringDeviceKey, setConfiguringDeviceKey] = useState('');
  const [actionError, setActionError] = useState('');
  const { loading, error, run } = useLoader(load);

  async function load() {
    const [cameraData, storageData] = await Promise.all([api('/api/cameras'), api('/api/storage-locations')]);
    const nextCameras = cameraData.cameras || [];
    setCameras(nextCameras);
    setStorage(storageData.storage_locations || []);
    loadCameraPreviews(nextCameras);
  }
  useEffect(() => { run(); }, []);

  function storageName(id) {
    return storage.find((item) => item.id === id)?.name || (id ? shortID(id) : 'No storage');
  }

  function cameraPayload(values, includeRTSP) {
    const payload = {
      name: values.name.trim(),
      storage_location_id: values.storage_location_id || null,
      location: values.location.trim() || null,
      camera_group: values.camera_group.trim() || null,
      enabled: values.enabled,
      recording_enabled: values.recording_enabled,
      record_audio: values.record_audio,
      stream_enabled: values.stream_enabled,
      stream_audio: values.stream_audio,
      retention_days: Number(values.retention_days),
      max_storage_bytes: values.max_storage_bytes ? Number(values.max_storage_bytes) : null,
    };
    if (includeRTSP || values.rtsp_url.trim()) {
      payload.rtsp_url = values.rtsp_url.trim();
    }
    return payload;
  }

  async function create(event) {
    event.preventDefault();
    setActionError('');
    try {
      await api('/api/cameras', { method: 'POST', body: JSON.stringify(cameraPayload(form, true)) });
      setForm(newCameraForm());
      run();
    } catch (err) {
      setActionError(err.message);
    }
  }

  function startEdit(camera) {
    setActionError('');
    setEditingId(camera.id);
    setEditForm(cameraToForm(camera));
  }

  function cancelEdit() {
    setEditingId('');
    setEditForm(null);
  }

  async function saveEdit(event) {
    event.preventDefault();
    if (!editingId || !editForm) return;
    setActionError('');
    try {
      await api(`/api/cameras/${editingId}`, { method: 'PATCH', body: JSON.stringify(cameraPayload(editForm, false)) });
      cancelEdit();
      run();
    } catch (err) {
      setActionError(err.message);
    }
  }

  async function quickPatch(camera, patch) {
    setActionError('');
    try {
      await api(`/api/cameras/${camera.id}`, { method: 'PATCH', body: JSON.stringify(patch) });
      run();
    } catch (err) {
      setActionError(err.message);
    }
  }

  function loadCameraPreviews(nextCameras) {
    nextCameras.forEach((camera) => {
      loadCameraPreview(camera);
    });
  }

  async function loadCameraPreview(camera) {
    setCameraPreviewBusy((prev) => ({ ...prev, [camera.id]: true }));
    setCameraPreviewErrors((prev) => ({ ...prev, [camera.id]: '' }));
    try {
      const response = await fetch(`${apiBaseUrl}/api/cameras/${camera.id}/preview`, {
        method: 'GET',
        credentials: 'include',
      });
      if (!response.ok) {
        const data = await response.json().catch(() => null);
        throw new Error(data?.error?.message || 'Unable to load preview.');
      }
      const blob = await response.blob();
      const source = response.headers.get('X-Preview-Source') || 'live';
      const imageUrl = URL.createObjectURL(blob);
      setCameraPreviews((prev) => {
        if (prev[camera.id]) URL.revokeObjectURL(prev[camera.id]);
        return { ...prev, [camera.id]: imageUrl };
      });
      setCameraPreviewSources((prev) => ({ ...prev, [camera.id]: source }));
    } catch (err) {
      setCameraPreviewErrors((prev) => ({ ...prev, [camera.id]: err.message || 'Unable to load preview.' }));
    } finally {
      setCameraPreviewBusy((prev) => ({ ...prev, [camera.id]: false }));
    }
  }

  async function scanCameras(event) {
    event.preventDefault();
    setActionError('');
    setScanning(true);
    setScanResult(null);
    const ports = scanPortsFromForm(scanForm.ports);
    const targetEstimate = estimateScanTargets(scanForm, ports);
    const timeoutMS = Number(scanForm.timeout_ms) || 900;
    const estimatedMS = Math.max(1800, Math.min(18000, targetEstimate.targets * Math.max(timeoutMS, 200) / 36));
    const startedAt = Date.now();
    setScanProgress({
      percent: 3,
      message: `Preparing ${targetEstimate.hosts || 'the'} host${targetEstimate.hosts === 1 ? '' : 's'} across ${ports.length || 1} ONVIF port${ports.length === 1 ? '' : 's'}...`,
    });
    const timer = window.setInterval(() => {
      const elapsed = Date.now() - startedAt;
      const percent = Math.min(94, Math.round((elapsed / estimatedMS) * 90) + 4);
      setScanProgress({ percent, message: scanProgressMessage(percent, targetEstimate) });
    }, 450);
    try {
      const payload = {
        cidr: scanForm.mode === 'cidr' ? scanForm.cidr.trim() : '',
        start_ip: scanForm.mode === 'range' ? scanForm.start_ip.trim() : '',
        end_ip: scanForm.mode === 'range' ? scanForm.end_ip.trim() : '',
        ports,
        timeout_ms: timeoutMS,
      };
      const result = await api('/api/cameras/scan', { method: 'POST', body: JSON.stringify(payload) });
      setScanProgress({ percent: 100, message: `Finished scan. Found ${result.summary?.found || 0} ONVIF candidate${result.summary?.found === 1 ? '' : 's'}.` });
      setScanResult(result);
    } catch (err) {
      setActionError(err.message || 'Unable to scan for cameras.');
      setScanProgress({ percent: 100, message: 'Scan stopped before completion.' });
    } finally {
      window.clearInterval(timer);
      setScanning(false);
    }
  }

  function deviceKey(device) {
    return `${device.ip}:${device.port}`;
  }

  function onvifFormFor(device) {
    return onvifForms[deviceKey(device)] || newONVIFImportForm(device);
  }

  function updateONVIFForm(device, patch) {
    const key = deviceKey(device);
    setOnvifForms((prev) => ({ ...prev, [key]: { ...onvifFormFor(device), ...patch } }));
  }

  function onvifPayload(device) {
    const values = onvifFormFor(device);
    return {
      xaddr: device.xaddr,
      name: values.name.trim(),
      username: values.username.trim(),
      password: values.password,
      storage_location_id: values.storage_location_id || null,
      retention_days: Number(values.retention_days),
      max_storage_bytes: values.max_storage_bytes ? Number(values.max_storage_bytes) : null,
      enabled: values.enabled,
      recording_enabled: values.recording_enabled,
      record_audio: values.record_audio,
      stream_enabled: values.stream_enabled,
      stream_audio: values.stream_audio,
    };
  }

  async function testONVIFDevice(device) {
    const key = deviceKey(device);
    setOnvifBusy((prev) => ({ ...prev, [key]: 'test' }));
    setOnvifMessages((prev) => ({ ...prev, [key]: '' }));
    try {
      await api('/api/cameras/onvif/test', { method: 'POST', body: JSON.stringify(onvifPayload(device)) });
      setOnvifMessages((prev) => ({ ...prev, [key]: 'Connection test passed. ONVIF returned an RTSP stream internally.' }));
      loadONVIFPreview(device);
    } catch (err) {
      setOnvifMessages((prev) => ({ ...prev, [key]: err.message || 'Connection test failed.' }));
    } finally {
      setOnvifBusy((prev) => ({ ...prev, [key]: '' }));
    }
  }

  async function loadONVIFPreview(device) {
    const key = deviceKey(device);
    setOnvifBusy((prev) => ({ ...prev, [key]: 'preview' }));
    try {
      const response = await fetch(`${apiBaseUrl}/api/cameras/onvif/preview`, {
        method: 'POST',
        credentials: 'include',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify(onvifPayload(device)),
      });
      if (!response.ok) {
        const data = await response.json().catch(() => null);
        throw new Error(data?.error?.message || 'Unable to capture preview.');
      }
      const blob = await response.blob();
      const oldPreview = onvifPreviews[key];
      const imageUrl = URL.createObjectURL(blob);
      setOnvifPreviews((prev) => ({ ...prev, [key]: imageUrl }));
      if (oldPreview) URL.revokeObjectURL(oldPreview);
    } catch (err) {
      setOnvifMessages((prev) => ({ ...prev, [key]: err.message || 'Unable to capture preview.' }));
    } finally {
      setOnvifBusy((prev) => ({ ...prev, [key]: '' }));
    }
  }

  async function importONVIFDevice(device) {
    const key = deviceKey(device);
    setOnvifBusy((prev) => ({ ...prev, [key]: 'import' }));
    setOnvifMessages((prev) => ({ ...prev, [key]: '' }));
    try {
      await api('/api/cameras/onvif/import', { method: 'POST', body: JSON.stringify(onvifPayload(device)) });
      setOnvifMessages((prev) => ({ ...prev, [key]: 'Camera added.' }));
      setScanResult((prev) => prev ? {
        ...prev,
        devices: (prev.devices || []).map((item) => (
          deviceKey(item) === key ? { ...item, existing_camera_name: onvifFormFor(device).name.trim() } : item
        )),
      } : prev);
      setConfiguringDeviceKey('');
      run();
    } catch (err) {
      setOnvifMessages((prev) => ({ ...prev, [key]: err.message || 'Unable to add camera.' }));
    } finally {
      setOnvifBusy((prev) => ({ ...prev, [key]: '' }));
    }
  }

  useEffect(() => {
    if (!configuringDeviceKey) return undefined;
    function closeOnEscape(event) {
      if (event.key === 'Escape') setConfiguringDeviceKey('');
    }
    window.addEventListener('keydown', closeOnEscape);
    return () => window.removeEventListener('keydown', closeOnEscape);
  }, [configuringDeviceKey]);

  const configuringDevice = (scanResult?.devices || []).find((device) => deviceKey(device) === configuringDeviceKey);

  return (
    <Panel title="Cameras">
      <section className="camera-admin-section camera-scan-panel">
        <div className="section-heading">
          <h3>Discover ONVIF Cameras</h3>
          <p>Scan a private IP range for ONVIF device services. Credentials and RTSP URLs are not guessed or exposed.</p>
        </div>
        <button type="button" onClick={() => setScanOpen(!scanOpen)}>{scanOpen ? 'Hide scanner' : 'Scan for cameras'}</button>
        {scanOpen && (
          <>
            <form className="camera-scan-form" onSubmit={scanCameras}>
              <Field label="Scan mode">
                <select value={scanForm.mode} onChange={(e) => setScanForm({ ...scanForm, mode: e.target.value })}>
                  <option value="cidr">CIDR</option>
                  <option value="range">IP range</option>
                </select>
              </Field>
              {scanForm.mode === 'cidr' ? (
                <Field label="CIDR range">
                  <input value={scanForm.cidr} onChange={(e) => setScanForm({ ...scanForm, cidr: e.target.value })} placeholder="192.168.1.0/24" />
                </Field>
              ) : (
                <>
                  <Field label="Start IP">
                    <input value={scanForm.start_ip} onChange={(e) => setScanForm({ ...scanForm, start_ip: e.target.value })} placeholder="192.168.1.1" />
                  </Field>
                  <Field label="End IP">
                    <input value={scanForm.end_ip} onChange={(e) => setScanForm({ ...scanForm, end_ip: e.target.value })} placeholder="192.168.1.254" />
                  </Field>
                </>
              )}
              <Field label="ONVIF ports">
                <input value={scanForm.ports} onChange={(e) => setScanForm({ ...scanForm, ports: e.target.value })} placeholder="80,8899" />
              </Field>
              <Field label="Timeout ms">
                <input type="number" min="200" max="5000" value={scanForm.timeout_ms} onChange={(e) => setScanForm({ ...scanForm, timeout_ms: e.target.value })} />
              </Field>
              <div className="form-actions">
                <button disabled={scanning}>{scanning ? 'Scanning...' : 'Start scan'}</button>
              </div>
            </form>
            {scanProgress && (
              <div className="scan-progress" role="status" aria-live="polite">
                <div>
                  <strong>{scanProgress.message}</strong>
                  <span>{Math.round(scanProgress.percent)}%</span>
                </div>
                <div className="scan-progress-track">
                  <span style={{ width: `${scanProgress.percent}%` }} />
                </div>
              </div>
            )}
            {scanResult && (
              <div className="camera-scan-results">
                <p className="muted">
                  Scanned {scanResult.summary?.targets || 0} targets. Found {scanResult.summary?.found || 0} ONVIF candidate{scanResult.summary?.found === 1 ? '' : 's'}.
                </p>
                {(scanResult.devices || []).length ? (
                  <div className="scan-device-grid">
                    {scanResult.devices.map((device) => (
                      <section className="scan-device-card" key={`${device.ip}:${device.port}`}>
                        <div>
                          <strong>{device.manufacturer || 'ONVIF camera'} {device.model || ''}</strong>
                          <span>{device.ip}:{device.port}</span>
                        </div>
                        <div className="scan-device-badges">
                          <StatusBadge kind={device.auth_required ? 'warning' : 'ok'} text={device.auth_required ? 'auth required' : device.status} />
                          {device.existing_camera_name && <span className="scan-existing-badge">Already added: {device.existing_camera_name}</span>}
                        </div>
                        <dl>
                          <div><dt>XAddr</dt><dd>{device.xaddr}</dd></div>
                          {device.serial_number && <div><dt>Serial</dt><dd>{device.serial_number}</dd></div>}
                          {device.firmware_version && <div><dt>Firmware</dt><dd>{device.firmware_version}</dd></div>}
                        </dl>
                        {!device.existing_camera_name && (
                          <button type="button" onClick={() => setConfiguringDeviceKey(deviceKey(device))}>
                            {configuringDeviceKey === deviceKey(device) ? 'Configuring...' : 'Configure'}
                          </button>
                        )}
                        {device.existing_camera_name && <button type="button" disabled>Already in system</button>}
                      </section>
                    ))}
                  </div>
                ) : (
                  <EmptyState title="No ONVIF cameras found" body="Try a smaller range, confirm the camera is powered on, or include its ONVIF HTTP port." />
                )}
              </div>
            )}
          </>
        )}
      </section>
      {configuringDevice && (
        <div className="onvif-modal-backdrop" role="presentation" onMouseDown={() => setConfiguringDeviceKey('')}>
          <section
            className="onvif-modal"
            role="dialog"
            aria-modal="true"
            aria-labelledby="onvif-modal-title"
            onMouseDown={(event) => event.stopPropagation()}
          >
            <div className="onvif-modal-header">
              <div>
                <span>Discovered camera</span>
                <h3 id="onvif-modal-title">{configuringDevice.manufacturer || 'ONVIF camera'} {configuringDevice.model || ''}</h3>
                <p>{configuringDevice.ip}:{configuringDevice.port}</p>
              </div>
              <button type="button" className="ghost-button" onClick={() => setConfiguringDeviceKey('')}>Close</button>
            </div>
            <ONVIFImportForm
              device={configuringDevice}
              form={onvifFormFor(configuringDevice)}
              storage={storage}
              busy={onvifBusy[deviceKey(configuringDevice)]}
              message={onvifMessages[deviceKey(configuringDevice)]}
              previewUrl={onvifPreviews[deviceKey(configuringDevice)]}
              onChange={(patch) => updateONVIFForm(configuringDevice, patch)}
              onTest={() => testONVIFDevice(configuringDevice)}
              onPreview={() => loadONVIFPreview(configuringDevice)}
              onImport={() => importONVIFDevice(configuringDevice)}
              onCancel={() => setConfiguringDeviceKey('')}
            />
          </section>
        </div>
      )}
      <section className="camera-admin-section camera-manual-panel">
        <button
          type="button"
          className="camera-manual-toggle"
          aria-expanded={manualAddOpen}
          onClick={() => setManualAddOpen((open) => !open)}
        >
          <span>
            <strong>Add camera manually</strong>
            <small>Use this when ONVIF discovery is not available or you already know the RTSP stream URL.</small>
          </span>
          <b>{manualAddOpen ? 'Hide form' : 'Open form'}</b>
        </button>
        {manualAddOpen && (
          <div className="camera-manual-body">
            <p>RTSP credentials are stored privately by the backend and are never returned to the browser.</p>
            <CameraForm
              form={form}
              setForm={setForm}
              storage={storage}
              onSubmit={create}
              submitText="Create camera"
              rtspRequired
            />
          </div>
        )}
      </section>
      <State loading={loading} error={error} />
      {actionError && <ErrorText message={actionError} />}
      {cameras.length ? (
        <div className="camera-list">
          {cameras.map((camera) => {
            const isEditing = editingId === camera.id;
            return (
              <section className="camera-card" key={camera.id}>
                <div className="camera-card-header">
                  <div>
                    <h3>{camera.name}</h3>
                    <p>{camera.location || camera.camera_group || shortID(camera.id)}</p>
                  </div>
                  <div className="camera-statuses">
                    <StatusBadge kind={camera.enabled ? 'ok' : 'muted'} text={camera.enabled ? 'enabled' : 'disabled'} />
                    <StatusBadge kind={camera.recording_enabled ? 'ok' : 'warning'} text={camera.recording_enabled ? 'recording' : 'not recording'} />
                    <StatusBadge kind={camera.stream_enabled ? 'ok' : 'muted'} text={camera.stream_enabled ? 'streaming' : 'stream off'} />
                  </div>
                </div>
                <div className="camera-preview-panel">
                  {cameraPreviews[camera.id] ? (
                    <img src={cameraPreviews[camera.id]} alt={`Preview from ${camera.name}`} />
                  ) : (
                    <div className="camera-preview-placeholder">
                      <strong>{cameraPreviewBusy[camera.id] ? 'Loading preview...' : 'No preview yet'}</strong>
                      <span>{camera.enabled ? (cameraPreviewErrors[camera.id] || 'Preview will appear when the camera responds.') : 'Camera is disabled.'}</span>
                    </div>
                  )}
                  <div className="camera-preview-overlay">
                    <span>
                      {camera.name}
                      {cameraPreviewSources[camera.id] === 'cache' && <em>Cached</em>}
                    </span>
                    <button type="button" disabled={cameraPreviewBusy[camera.id]} onClick={() => loadCameraPreview(camera)}>
                      {cameraPreviewBusy[camera.id] ? 'Refreshing...' : 'Refresh preview'}
                    </button>
                  </div>
                </div>
                {isEditing ? (
                  <CameraForm
                    form={editForm}
                    setForm={setEditForm}
                    storage={storage}
                    onSubmit={saveEdit}
                    submitText="Save changes"
                    secondaryAction={<button type="button" onClick={cancelEdit}>Cancel</button>}
                  />
                ) : (
                  <>
                    <dl className="camera-facts">
                      <div><dt>Storage</dt><dd>{storageName(camera.storage_location_id)}</dd></div>
                      <div><dt>Retention</dt><dd>{camera.retention_days} days</dd></div>
                      <div><dt>Audio</dt><dd>{camera.record_audio || camera.stream_audio ? `${camera.record_audio ? 'Record' : ''}${camera.record_audio && camera.stream_audio ? ' + ' : ''}${camera.stream_audio ? 'Live' : ''}` : 'Off'}</dd></div>
                      <div><dt>Max storage</dt><dd>{camera.max_storage_bytes ? formatBytes(camera.max_storage_bytes) : 'No cap'}</dd></div>
                      <div><dt>Updated</dt><dd>{formatDateTime(camera.updated_at)}</dd></div>
                    </dl>
                    <div className="camera-actions">
                      <button type="button" onClick={() => startEdit(camera)}>Edit</button>
                      <button
                        type="button"
                        disabled={!camera.recording_enabled && !camera.storage_location_id}
                        onClick={() => quickPatch(camera, { recording_enabled: !camera.recording_enabled })}
                      >
                        {camera.recording_enabled ? 'Stop recording' : 'Start recording'}
                      </button>
                      <button
                        type="button"
                        onClick={() => quickPatch(camera, { enabled: !camera.enabled })}
                      >
                        {camera.enabled ? 'Disable camera' : 'Enable camera'}
                      </button>
                    </div>
                  </>
                )}
              </section>
            );
          })}
        </div>
      ) : (
        !loading && <EmptyState title="No cameras" body="Add a camera to begin capturing RTSP streams." />
      )}
    </Panel>
  );
}

function newCameraForm() {
  return {
    name: '',
    rtsp_url: '',
    storage_location_id: '',
    location: '',
    camera_group: '',
    enabled: true,
    recording_enabled: false,
    record_audio: false,
    stream_enabled: true,
    stream_audio: false,
    retention_days: 30,
    max_storage_bytes: '',
  };
}

function newCameraScanForm() {
  return {
    mode: 'cidr',
    cidr: '192.168.1.0/24',
    start_ip: '192.168.1.1',
    end_ip: '192.168.1.254',
    ports: '80,8899',
    timeout_ms: 900,
  };
}

function newONVIFImportForm(device) {
  return {
    name: [device.manufacturer, device.model].filter(Boolean).join(' ') || `Camera ${device.ip}`,
    username: '',
    password: '',
    storage_location_id: '',
    retention_days: 30,
    max_storage_bytes: '',
    enabled: true,
    recording_enabled: false,
    record_audio: false,
    stream_enabled: true,
    stream_audio: false,
  };
}

function ONVIFImportForm({ device, form, storage, busy, message, previewUrl, onChange, onTest, onPreview, onImport, onCancel }) {
  return (
    <div className="onvif-import-form">
      <div className="onvif-preview">
        {previewUrl ? (
          <img src={previewUrl} alt={`Preview from ${device.ip}`} />
        ) : (
          <div><strong>Preview unavailable</strong><span>Run Test or Preview after entering authentication.</span></div>
        )}
      </div>
      <Field label="Camera name">
        <input value={form.name} onChange={(e) => onChange({ name: e.target.value })} />
      </Field>
      <Field label="Storage">
        <select value={form.storage_location_id} onChange={(e) => onChange({ storage_location_id: e.target.value })}>
          <option value="">No storage</option>
          {storage.map((item) => <option key={item.id} value={item.id}>{item.name}</option>)}
        </select>
      </Field>
      <Field label={device.auth_required ? 'ONVIF username' : 'ONVIF username optional'}>
        <input value={form.username} onChange={(e) => onChange({ username: e.target.value })} autoComplete="off" />
      </Field>
      <Field label={device.auth_required ? 'ONVIF password' : 'ONVIF password optional'}>
        <input type="password" value={form.password} onChange={(e) => onChange({ password: e.target.value })} autoComplete="new-password" />
      </Field>
      <Field label="Retention days">
        <input type="number" min="1" value={form.retention_days} onChange={(e) => onChange({ retention_days: e.target.value })} />
      </Field>
      <Field label="Max storage bytes">
        <input type="number" min="1" value={form.max_storage_bytes} onChange={(e) => onChange({ max_storage_bytes: e.target.value })} placeholder="Optional" />
      </Field>
      <div className="camera-switches">
        <label className="switch-row"><input type="checkbox" checked={form.enabled} onChange={(e) => onChange({ enabled: e.target.checked })} /> Camera enabled</label>
        <label className="switch-row"><input type="checkbox" checked={form.recording_enabled} onChange={(e) => onChange({ recording_enabled: e.target.checked })} /> Record video</label>
        <label className="switch-row"><input type="checkbox" checked={form.record_audio} onChange={(e) => onChange({ record_audio: e.target.checked })} /> Record audio</label>
        <label className="switch-row"><input type="checkbox" checked={form.stream_enabled} onChange={(e) => onChange({ stream_enabled: e.target.checked })} /> Stream video</label>
        <label className="switch-row"><input type="checkbox" checked={form.stream_audio} onChange={(e) => onChange({ stream_audio: e.target.checked })} /> Stream audio</label>
      </div>
      <div className="form-actions">
        <button type="button" disabled={Boolean(busy)} onClick={onTest}>{busy === 'test' ? 'Testing...' : 'Test'}</button>
        <button type="button" disabled={Boolean(busy)} onClick={onPreview}>{busy === 'preview' ? 'Loading preview...' : 'Preview'}</button>
        <button type="button" disabled={Boolean(busy) || !form.name.trim()} onClick={onImport}>{busy === 'import' ? 'Adding...' : 'Add camera'}</button>
        <button type="button" disabled={Boolean(busy)} onClick={onCancel}>Cancel</button>
      </div>
      {message && <p className={message.includes('passed') || message.includes('added') ? 'success-text' : 'error'}>{message}</p>}
    </div>
  );
}

function scanPortsFromForm(value) {
  const ports = String(value || '')
    .split(',')
    .map((item) => Number(item.trim()))
    .filter((item) => Number.isFinite(item) && item > 0);
  return ports.length ? ports : [80, 8899];
}

function estimateScanTargets(form, ports) {
  const hosts = form.mode === 'range'
    ? estimateIPRangeCount(form.start_ip, form.end_ip)
    : estimateCIDRHostCount(form.cidr);
  return { hosts, targets: Math.max(1, hosts || 1) * Math.max(1, ports.length || 1) };
}

function estimateCIDRHostCount(cidr) {
  const parts = String(cidr || '').split('/');
  const prefix = Number(parts[1]);
  if (!Number.isInteger(prefix) || prefix < 0 || prefix > 32) return 0;
  return Math.min(512, 2 ** (32 - prefix));
}

function estimateIPRangeCount(start, end) {
  const startValue = ipv4ToNumber(start);
  const endValue = ipv4ToNumber(end);
  if (startValue === null || endValue === null || endValue < startValue) return 0;
  return Math.min(512, endValue - startValue + 1);
}

function ipv4ToNumber(value) {
  const parts = String(value || '').trim().split('.').map((part) => Number(part));
  if (parts.length !== 4 || parts.some((part) => !Number.isInteger(part) || part < 0 || part > 255)) return null;
  return (((parts[0] * 256 + parts[1]) * 256 + parts[2]) * 256 + parts[3]);
}

function scanProgressMessage(percent, estimate) {
  if (percent < 18) return `Preparing ${estimate.targets || 'selected'} scan target${estimate.targets === 1 ? '' : 's'}...`;
  if (percent < 48) return 'Probing ONVIF service ports...';
  if (percent < 76) return 'Waiting for device service responses...';
  if (percent < 94) return 'Checking ONVIF device information...';
  return 'Finalizing discovered camera candidates...';
}

function cameraToForm(camera) {
  return {
    name: camera.name || '',
    rtsp_url: '',
    storage_location_id: camera.storage_location_id || '',
    location: camera.location || '',
    camera_group: camera.camera_group || '',
    enabled: Boolean(camera.enabled),
    recording_enabled: Boolean(camera.recording_enabled),
    record_audio: Boolean(camera.record_audio),
    stream_enabled: camera.stream_enabled !== false,
    stream_audio: Boolean(camera.stream_audio),
    retention_days: camera.retention_days || 30,
    max_storage_bytes: camera.max_storage_bytes || '',
  };
}

function CameraForm({ form, setForm, storage, onSubmit, submitText, rtspRequired = false, secondaryAction }) {
  return (
    <form className="camera-form" onSubmit={onSubmit}>
      <div className="camera-form-section camera-form-section-primary">
        <div className="camera-form-section-heading">
          <strong>Connection</strong>
          <span>Give the channel a clear name and provide the private RTSP stream URL.</span>
        </div>
        <Field label="Camera name" help="Shown in Live, Playback, layouts, and alerts.">
          <input value={form.name} onChange={(e) => setForm({ ...form, name: e.target.value })} placeholder="Front gate" required />
        </Field>
        <Field label={rtspRequired ? 'RTSP stream URL' : 'Replace RTSP stream URL'} help={rtspRequired ? 'Example: rtsp://user:password@192.168.1.50:554/cam/realmonitor?channel=1&subtype=0' : 'Leave blank to keep the saved private stream URL.'}>
          <input
            value={form.rtsp_url}
            onChange={(e) => setForm({ ...form, rtsp_url: e.target.value })}
            placeholder={rtspRequired ? 'rtsp://user:password@camera-host:554/stream' : 'Leave blank to keep current private URL'}
            required={rtspRequired}
            spellCheck="false"
          />
        </Field>
      </div>

      <div className="camera-form-section">
        <div className="camera-form-section-heading">
          <strong>Recording</strong>
          <span>Choose where clips are stored and how long this camera keeps footage.</span>
        </div>
        <Field label="Storage location" help={storage.length ? 'Recording can only start when a writable storage location is selected.' : 'Create a storage location before enabling recording.'}>
          <select value={form.storage_location_id} onChange={(e) => setForm({ ...form, storage_location_id: e.target.value })}>
            <option value="">No storage selected</option>
            {storage.map((item) => <option key={item.id} value={item.id}>{item.name}</option>)}
          </select>
        </Field>
        <Field label="Retention days" help="Old segments for this camera become eligible for cleanup after this many days.">
          <input type="number" min="1" value={form.retention_days} onChange={(e) => setForm({ ...form, retention_days: e.target.value })} />
        </Field>
        <Field label="Max storage bytes" help="Optional per-camera cap. Leave blank for no camera-specific limit.">
          <input type="number" min="1" value={form.max_storage_bytes} onChange={(e) => setForm({ ...form, max_storage_bytes: e.target.value })} placeholder="Optional" />
        </Field>
      </div>

      <div className="camera-form-section">
        <div className="camera-form-section-heading">
          <strong>Organization</strong>
          <span>Add simple labels and choose whether the camera is active immediately.</span>
        </div>
        <Field label="Location" help="Physical place or viewing angle.">
          <input value={form.location} onChange={(e) => setForm({ ...form, location: e.target.value })} placeholder="Gate, lobby, office..." />
        </Field>
        <Field label="Group" help="Useful for filtering larger camera systems.">
          <input value={form.camera_group} onChange={(e) => setForm({ ...form, camera_group: e.target.value })} placeholder="Outdoor, warehouse..." />
        </Field>
        <div className="camera-switches">
          <label className="switch-row">
            <input type="checkbox" checked={form.enabled} onChange={(e) => setForm({ ...form, enabled: e.target.checked })} />
            <span><strong>Camera enabled</strong><small>Allow live view, preview, and recorder discovery.</small></span>
          </label>
          <label className="switch-row">
            <input type="checkbox" checked={form.recording_enabled} onChange={(e) => setForm({ ...form, recording_enabled: e.target.checked })} />
            <span><strong>Record video</strong><small>Start recording when storage is selected and the recorder sees this channel.</small></span>
          </label>
          <label className="switch-row">
            <input type="checkbox" checked={form.record_audio} onChange={(e) => setForm({ ...form, record_audio: e.target.checked })} />
            <span><strong>Record audio</strong><small>Include camera audio in saved recording segments when available.</small></span>
          </label>
          <label className="switch-row">
            <input type="checkbox" checked={form.stream_enabled} onChange={(e) => setForm({ ...form, stream_enabled: e.target.checked })} />
            <span><strong>Stream video</strong><small>Keep this channel available in Live views and warm HLS buffers.</small></span>
          </label>
          <label className="switch-row">
            <input type="checkbox" checked={form.stream_audio} onChange={(e) => setForm({ ...form, stream_audio: e.target.checked })} />
            <span><strong>Stream audio</strong><small>Include camera audio in HLS live streams when available.</small></span>
          </label>
        </div>
      </div>
      <div className="form-actions">
        <button>{submitText}</button>
        {secondaryAction}
      </div>
    </form>
  );
}

function UsersPage() {
  const [users, setUsers] = useState([]);
  const [form, setForm] = useState({ email: '', username: '', display_name: '', password: '', role: 'user' });
  const { loading, error, run } = useLoader(load);

  async function load() {
    const data = await api('/api/users');
    setUsers(data.users || []);
  }
  useEffect(() => { run(); }, []);

  async function create(event) {
    event.preventDefault();
    await api('/api/users', { method: 'POST', body: JSON.stringify(form) });
    setForm({ email: '', username: '', display_name: '', password: '', role: 'user' });
    run();
  }

  return (
    <Panel title="Users">
      <FormGrid onSubmit={create}>
        <input placeholder="Email" value={form.email} onChange={(e) => setForm({ ...form, email: e.target.value })} />
        <input placeholder="Username" value={form.username} onChange={(e) => setForm({ ...form, username: e.target.value })} />
        <input placeholder="Display name" value={form.display_name} onChange={(e) => setForm({ ...form, display_name: e.target.value })} />
        <input placeholder="Password" type="password" value={form.password} onChange={(e) => setForm({ ...form, password: e.target.value })} />
        <select value={form.role} onChange={(e) => setForm({ ...form, role: e.target.value })}><option>user</option><option>admin</option></select>
        <button>Create</button>
      </FormGrid>
      <State loading={loading} error={error} />
      {users.length ? (
        <DataTable rows={users} columns={['email', 'username', 'display_name', 'role', 'active']} />
      ) : (
        !loading && <EmptyState title="No users" body="Create your first user account." />
      )}
    </Panel>
  );
}

function PermissionsPage() {
  const [users, setUsers] = useState([]);
  const [cameras, setCameras] = useState([]);
  const [form, setForm] = useState({ user_id: '', camera_id: '', can_view_live: true, can_view_playback: true });
  const { loading, error, run } = useLoader(load);

  async function load() {
    const [userData, cameraData] = await Promise.all([api('/api/users'), api('/api/cameras')]);
    setUsers(userData.users || []);
    setCameras(cameraData.cameras || []);
  }
  useEffect(() => { run(); }, []);

  async function grant(event) {
    event.preventDefault();
    await api(`/api/users/${form.user_id}/camera-permissions/${form.camera_id}`, { method: 'PUT', body: JSON.stringify({ can_view_live: form.can_view_live, can_view_playback: form.can_view_playback }) });
    run();
  }
  async function revoke() {
    await api(`/api/users/${form.user_id}/camera-permissions/${form.camera_id}`, { method: 'DELETE' });
  }

  return (
    <Panel title="Camera Permissions">
      <FormGrid onSubmit={grant}>
        <select value={form.user_id} onChange={(e) => setForm({ ...form, user_id: e.target.value })}><option value="">User</option>{users.map((u) => <option key={u.id} value={u.id}>{u.email}</option>)}</select>
        <select value={form.camera_id} onChange={(e) => setForm({ ...form, camera_id: e.target.value })}><option value="">Camera</option>{cameras.map((c) => <option key={c.id} value={c.id}>{c.name}</option>)}</select>
        <label className="check"><input type="checkbox" checked={form.can_view_live} onChange={(e) => setForm({ ...form, can_view_live: e.target.checked })} /> Live</label>
        <label className="check"><input type="checkbox" checked={form.can_view_playback} onChange={(e) => setForm({ ...form, can_view_playback: e.target.checked })} /> Playback</label>
        <button>Grant</button>
        <button type="button" onClick={revoke}>Revoke</button>
      </FormGrid>
      <State loading={loading} error={error} />
      {!users.length && !loading && <EmptyState title="No users" body="Create a user before granting permissions." />}
      {!cameras.length && !loading && <EmptyState title="No cameras" body="Add a camera before granting permissions." />}
    </Panel>
  );
}

function LayoutCanvas({ layout, cameras, onChange, onError }) {
  const items = (layout.layout_items || []).slice().sort((a, b) => (a.display_order || 0) - (b.display_order || 0));
  const cols = Math.max(1, Number(layout.settings?.columns) || 4);
  const maxRow = items.reduce((m, item) => Math.max(m, Number(item.y || 0) + Number(item.height || 1)), 0);
  const rows = Math.max(3, maxRow + 2);
  const [selectedId, setSelectedId] = useState(null);
  const [pendingCell, setPendingCell] = useState(null);
  const [drag, setDrag] = useState(null);
  const gridRef = React.useRef(null);
  const suppressNextGridClickRef = React.useRef(false);
  const cameraById = new Map((cameras || []).map((camera) => [camera.id, camera]));

  function itemID(item) {
    return item.id || item.item_id;
  }

  function gridRect() {
    const el = gridRef.current;
    if (!el) return null;
    const r = el.getBoundingClientRect();
    return { left: r.left, top: r.top, width: r.width, height: r.height };
  }

  function pixelToCell(px, py) {
    const r = gridRect();
    if (!r) return { x: 0, y: 0 };
    const cw = r.width / cols || 1;
    const ch = r.height / rows || 1;
    let cx = Math.floor((px - r.left) / cw);
    let cy = Math.floor((py - r.top) / ch);
    cx = Math.max(0, Math.min(cols - 1, cx));
    cy = Math.max(0, Math.min(rows - 1, cy));
    return { x: cx, y: cy };
  }

  function startDrag(item, mode, event) {
    event.preventDefault();
    event.stopPropagation();
    suppressNextGridClickRef.current = false;
    setSelectedId(itemID(item));
    setDrag({
      itemId: itemID(item),
      mode,
      startMouseX: event.clientX,
      startMouseY: event.clientY,
      startX: Number(item.x || 0),
      startY: Number(item.y || 0),
      startW: Math.max(1, Number(item.width || 1)),
      startH: Math.max(1, Number(item.height || 1)),
      moved: mode !== 'move',
    });
  }

  function dragHasMoved(activeDrag, event) {
    if (!activeDrag) return false;
    if (activeDrag.moved) return true;
    const dx = event.clientX - activeDrag.startMouseX;
    const dy = event.clientY - activeDrag.startMouseY;
    return Math.sqrt(dx * dx + dy * dy) > 4;
  }

  function updatedItemsForDrag(event) {
    if (!drag) return items;
    const rect = gridRect();
    if (!rect) return items;
    const cw = rect.width / cols || 1;
    const ch = rect.height / rows || 1;
    const dxc = Math.round((event.clientX - drag.startMouseX) / cw);
    const dyc = Math.round((event.clientY - drag.startMouseY) / ch);
    return items.map((item) => {
      if (itemID(item) !== drag.itemId) return item;
      let x = drag.startX;
      let y = drag.startY;
      let width = drag.startW;
      let height = drag.startH;
      if (drag.mode === 'move') {
        x = Math.max(0, Math.min(cols - width, drag.startX + dxc));
        y = Math.max(0, drag.startY + dyc);
      } else {
        if (drag.mode.includes('w')) {
          x = Math.max(0, drag.startX + dxc);
          width = Math.max(1, drag.startW - (x - drag.startX));
        }
        if (drag.mode.includes('e')) width = Math.max(1, drag.startW + dxc);
        if (drag.mode.includes('n')) {
          y = Math.max(0, drag.startY + dyc);
          height = Math.max(1, drag.startH - (y - drag.startY));
        }
        if (drag.mode.includes('s')) height = Math.max(1, drag.startH + dyc);
        if (x + width > cols) width = Math.max(1, cols - x);
      }
      return { ...item, x, y, width, height };
    });
  }

  function onGridMouseMove(event) {
    if (!drag) return;
    if (!drag.moved && dragHasMoved(drag, event)) {
      setDrag({ ...drag, moved: true });
    }
    onChange(updatedItemsForDrag(event), { refetch: false });
  }

  function onGridMouseUp(event) {
    if (!drag) return;
    const shouldSuppressClick = dragHasMoved(drag, event);
    const updated = updatedItemsForDrag(event);
    const changed = updated.find((item) => itemID(item) === drag.itemId);
    if (shouldSuppressClick) suppressNextGridClickRef.current = true;
    setDrag(null);
    onChange(updated, { refetch: false });
    if (changed) saveItem(changed);
  }

  async function saveItem(item) {
    try {
      await api(`/api/layouts/${layout.id}/items/${itemID(item)}`, { method: 'PATCH', body: JSON.stringify({
        camera_id: item.camera_id,
        x: item.x, y: item.y, width: item.width, height: item.height,
        display_order: item.display_order, tile_type: item.tile_type,
      })});
      onChange((layout.layout_items || []).map((x) => itemID(x) === itemID(item) ? item : x), { refetch: true });
    } catch (e) { onError(e.message); }
  }

  async function deleteItem(itemId) {
    try {
      await api(`/api/layouts/${layout.id}/items/${itemId}`, { method: 'DELETE' });
      onChange(items.filter((x) => itemID(x) !== itemId), { refetch: true });
      setSelectedId(null);
    } catch (e) { onError(e.message); }
  }

  async function addItemAt(x, y, cameraId) {
    if (!cameraId) return;
    try {
      const data = await api(`/api/layouts/${layout.id}/items`, {
        method: 'POST',
        body: JSON.stringify({
          camera_id: cameraId, x, y, width: 1, height: 1,
          display_order: items.length, tile_type: 'custom',
        }),
      });
      const created = data?.layout_item || data?.item || data;
      if (created && created.id) onChange([...items, created], { refetch: true });
    } catch (e) { onError(e.message); }
  }

  function onGridClick(event) {
    if (suppressNextGridClickRef.current) {
      suppressNextGridClickRef.current = false;
      event.preventDefault();
      event.stopPropagation();
      return;
    }
    if (event.target !== gridRef.current) return;
    const cell = pixelToCell(event.clientX, event.clientY);
    setSelectedId(null);
    setPendingCell(cell);
  }

  const selected = selectedId ? items.find((x) => itemID(x) === selectedId) : null;
  const tileCamera = selected ? cameraById.get(selected.camera_id) : null;

  return (
    <div className="layout-canvas-wrap">
      <div
        ref={gridRef}
        className="live-layout-grid layout-editor-grid editable"
        style={{
          '--layout-columns': cols,
          gridTemplateColumns: `repeat(${cols}, minmax(0, 1fr))`,
          gridTemplateRows: `repeat(${rows}, minmax(96px, auto))`,
        }}
        onMouseMove={onGridMouseMove}
        onMouseUp={onGridMouseUp}
        onMouseLeave={onGridMouseUp}
        onClick={onGridClick}
      >
        {items.map((it) => {
          const cam = cameraById.get(it.camera_id);
          return (
            <section
              key={itemID(it)}
              className={'video-tile live-layout-tile layout-editor-tile' + (selectedId === itemID(it) ? ' selected' : '')}
              style={{ gridColumn: `${(it.x || 0) + 1} / span ${Math.max(1, it.width || 1)}`, gridRow: `${(it.y || 0) + 1} / span ${Math.max(1, it.height || 1)}` }}
              onClick={(e) => { e.stopPropagation(); setSelectedId(itemID(it)); }}
            >
              <div className="live-tile-bar" onMouseDown={(e) => startDrag(it, 'move', e)}>
                <strong>{cam ? cam.name : shortID(it.camera_id)}</strong>
                <span className="muted">{it.width || 1}×{it.height || 1}</span>
              </div>
              <div className="live-tile-video layout-editor-tile-body">
                <p>{cam ? (cam.location || cam.camera_group || shortID(cam.id)) : 'Camera unavailable'}</p>
              </div>
              {selectedId === itemID(it) && (
                <>
                  {['nw','n','ne','e','se','s','sw','w'].map((h) => (
                    <span
                      key={h}
                      className={`layout-handle layout-handle-${h}`}
                      onMouseDown={(e) => { e.stopPropagation(); startDrag(it, h, e); }}
                    />
                  ))}
                </>
              )}
            </section>
          );
        })}
      </div>
      {selected && (
        <div className="layout-tile-edit">
          <span>
            <strong>{tileCamera ? tileCamera.name : shortID(selected.camera_id)}</strong>
            <span className="muted"> · {selected.width}×{selected.height} @ ({selected.x},{selected.y})</span>
          </span>
          <button type="button" onClick={() => setSelectedId(null)}>Close</button>
          <button type="button" className="danger" onClick={() => { if (window.confirm('Delete this tile?')) deleteItem(selected.id); }}>Delete</button>
        </div>
      )}
      {pendingCell && (
        <div className="layout-add-popup">
          <div className="layout-add-card">
            <h3>Add camera at ({pendingCell.x}, {pendingCell.y})</h3>
            <select
              onChange={(e) => {
                if (e.target.value) {
                  addItemAt(pendingCell.x, pendingCell.y, e.target.value);
                  setPendingCell(null);
                }
              }}
            >
              <option value="">Select a camera…</option>
              {cameras.map((c) => <option key={c.id} value={c.id}>{c.name}</option>)}
            </select>
            <button type="button" onClick={() => setPendingCell(null)}>Cancel</button>
          </div>
        </div>
      )}
    </div>
  );
}

function LayoutsPage() {
  const { user } = useAuth();
  const [layouts, setLayouts] = useState([]);
  const [cameras, setCameras] = useState([]);
  const [form, setForm] = useState({ name: '', columns: 4 });
  const [activeId, setActiveId] = useState(null);
  const [errorMsg, setErrorMsg] = useState('');
  const { loading, error, run } = useLoader(load);

  async function load() {
    const layoutData = await api('/api/layouts');
    const list = layoutData.layouts || [];
    setLayouts(list);
    if (!activeId && list.length) setActiveId(list[0].id);
    if (user.role === 'admin') {
      const cameraData = await api('/api/cameras');
      setCameras(cameraData.cameras || []);
    }
  }
  useEffect(() => { run(); }, []);

  async function createLayout(event) {
    event.preventDefault();
    try {
      const data = await api('/api/layouts', { method: 'POST', body: JSON.stringify({ name: form.name, settings: { columns: Number(form.columns) } }) });
      setForm({ name: '', columns: 4 });
      setActiveId(data?.layout?.id || data?.id);
      run();
    } catch (e) { setErrorMsg(e.message); }
  }
  async function setDefault(id) {
    try {
      await api(`/api/layouts/${id}/default`, { method: 'PATCH' });
      run();
    } catch (e) { setErrorMsg(e.message); }
  }
  async function deleteLayout(id) {
    if (!window.confirm('Delete this layout and all of its items?')) return;
    try {
      await api(`/api/layouts/${id}`, { method: 'DELETE' });
      setActiveId(null);
      run();
    } catch (e) { setErrorMsg(e.message); }
  }
  async function patchColumns(id, cols) {
    try {
      await api(`/api/layouts/${id}`, { method: 'PATCH', body: JSON.stringify({ settings: { columns: Number(cols) } }) });
      run();
    } catch (e) { setErrorMsg(e.message); }
  }
  function activeLayout() {
    return layouts.find((l) => l.id === activeId) || null;
  }
  function onItemsChanged(updatedItems, options = {}) {
    setLayouts((prev) => prev.map((l) => l.id === activeId ? { ...l, layout_items: updatedItems } : l));
    if (options.refetch) run();
  }

  const layout = activeLayout();

  return (
    <Panel title="Layouts">
      {errorMsg && <ErrorText message={errorMsg} />}
      {user.role === 'admin' && (
        <FormGrid onSubmit={createLayout}>
          <input placeholder="New layout name" value={form.name} onChange={(e) => setForm({ ...form, name: e.target.value })} required />
          <input type="number" min="1" max="32" placeholder="Columns" value={form.columns} onChange={(e) => setForm({ ...form, columns: e.target.value })} />
          <button>Create layout</button>
        </FormGrid>
      )}
      <State loading={loading} error={error} />
      {!layouts.length && !loading ? (
        <EmptyState title="No layouts" body={user.role === 'admin' ? 'Create a layout above to start adding camera tiles.' : 'No layouts have been shared with you yet.'} />
      ) : (
        <>
          <Toolbar>
            <select value={activeId || ''} onChange={(e) => setActiveId(e.target.value)}>
              <option value="" disabled>Pick a layout…</option>
              {layouts.map((l) => <option key={l.id} value={l.id}>{l.name}{l.is_default ? ' (default)' : ''}</option>)}
            </select>
            {layout && user.role === 'admin' && (
              <>
                <label className="check">Columns
                  <input
                    type="number" min="1" max="32" style={{ width: '64px' }}
                    defaultValue={layout.settings?.columns || 4}
                    onBlur={(e) => {
                      const v = Number(e.target.value);
                      if (v && v !== (layout.settings?.columns || 4)) patchColumns(layout.id, v);
                    }}
                  />
                </label>
                {!layout.is_default && <button type="button" onClick={() => setDefault(layout.id)}>Make default</button>}
                <button type="button" className="danger" onClick={() => deleteLayout(layout.id)}>Delete layout</button>
              </>
            )}
            {layout && (
              <span className="muted" style={{ marginLeft: 'auto' }}>
                {(layout.layout_items || []).length} tile{(layout.layout_items || []).length === 1 ? '' : 's'}
              </span>
            )}
          </Toolbar>
          {layout && (
            <>
              <p className="muted layout-help">
                Click an empty cell to add a camera. Drag a tile to move it. Drag a corner or edge handle (visible when the tile is selected) to resize. Edits save automatically.
              </p>
              <LayoutCanvas
                layout={layout}
                cameras={cameras}
                onChange={onItemsChanged}
                onError={setErrorMsg}
              />
            </>
          )}
        </>
      )}
    </Panel>
  );
}

function LiveLayoutPage() {
  const { user } = useAuth();
  const [layouts, setLayouts] = useState([]);
  const [layoutId, setLayoutId] = useState('');
  const [result, setResult] = useState(null);
  const [actionError, setActionError] = useState('');
  const [opening, setOpening] = useState(false);
  const [autoOpenedId, setAutoOpenedId] = useState('');
  const { loading, error, run } = useLoader(load);

  async function load() {
    const data = await api('/api/layouts');
    const list = data.layouts || [];
    setLayouts(list);
    if (!layoutId && list.length) {
      const preferred = list.find((item) => item.name === 'Main') || list.find((item) => item.is_default) || list[0];
      setLayoutId(preferred.id);
    }
  }
  useEffect(() => { run(); }, []);

  const layout = layouts.find((l) => l.id === layoutId) || null;

  async function loadLive(targetLayoutId = layoutId, { silent = false } = {}) {
    if (!targetLayoutId) return;
    if (!silent) setOpening(true);
    setActionError('');
    try {
      setResult(await api(`/api/live/layouts/${targetLayoutId}`));
      setAutoOpenedId(targetLayoutId);
    } catch (err) {
      setActionError(err.message || 'Unable to open live view.');
    } finally {
      if (!silent) setOpening(false);
    }
  }

  async function openLive(targetLayoutId = layoutId) {
    await loadLive(targetLayoutId);
  }

  useEffect(() => {
    if (layoutId && layoutId !== autoOpenedId && !opening && !result) {
      openLive(layoutId);
    }
  }, [layoutId, autoOpenedId, opening, result]);

  useEffect(() => {
    if (!layoutId || !result?.cameras?.some((camera) => camera.status === 'starting')) return undefined;
    const timer = window.setTimeout(() => {
      loadLive(layoutId, { silent: true });
    }, 2000);
    return () => window.clearTimeout(timer);
  }, [layoutId, result]);

  return (
    <Panel title="Live Layout">
      <Toolbar>
        <select value={layoutId} onChange={(e) => { setResult(null); setAutoOpenedId(''); setLayoutId(e.target.value); }} disabled={!layouts.length}>
          {layouts.length === 0 && <option value="">No layouts</option>}
          {layouts.map((l) => <option key={l.id} value={l.id}>{l.name}</option>)}
        </select>
        <button onClick={() => openLive()} disabled={!layoutId || opening}>{opening ? 'Opening...' : 'Open live'}</button>
      </Toolbar>
      <State loading={loading} error={error} />
      {actionError && <ErrorText message={actionError} />}
      {!layouts.length && !loading && <EmptyState title="No layouts" body="Create a layout to start streaming live video." />}
      {result && layout && (
        <LiveLayoutGrid
          layout={layout}
          cameras={result?.cameras || []}
          editable={user?.role === 'admin'}
          onError={setActionError}
          onChange={(updatedItems) => {
            setLayouts((prev) => prev.map((item) => (
              item.id === layout.id ? { ...item, layout_items: updatedItems } : item
            )));
          }}
        />
      )}
    </Panel>
  );
}

function PlaybackPage() {
  const [layouts, setLayouts] = useState([]);
  const [layoutId, setLayoutId] = useState('');
  const [selectedTime, setSelectedTime] = useState(() => new Date());
  const [timeline, setTimeline] = useState(null);
  const [result, setResult] = useState(null);
  const [actionError, setActionError] = useState('');
  const [opening, setOpening] = useState(false);
  const [playing, setPlaying] = useState(false);
  const [autoOpenedId, setAutoOpenedId] = useState('');
  const [focusedCameraId, setFocusedCameraId] = useState('');
  const videoRefs = React.useRef(new Map());
  const { loading, error, run } = useLoader(load);

  async function load() {
    const data = await api('/api/layouts');
    const list = data.layouts || [];
    setLayouts(list);
    if (!layoutId && list.length) setLayoutId(preferredLayout(list)?.id || list[0].id);
  }
  useEffect(() => { run(); }, []);

  const layout = layouts.find((item) => item.id === layoutId);
  useEffect(() => {
    if (!layoutId || autoOpenedId === layoutId || opening) return;
    openPlayback(layoutId, selectedTime, true);
  }, [layoutId, autoOpenedId, opening]);

  async function loadTimeline(targetLayout, timestamp) {
    const ids = (targetLayout?.layout_items || []).map((item) => item.camera_id).filter(Boolean);
    if (!ids.length) {
      setTimeline(null);
      return;
    }
    const [start, end] = dayBounds(timestamp);
    const params = new URLSearchParams();
    ids.forEach((id) => params.append('camera_id', id));
    params.set('start_time', start.toISOString());
    params.set('end_time', end.toISOString());
    params.set('gap_threshold_seconds', '3');
    setTimeline(await api(`/api/recordings/timeline?${params.toString()}`));
  }

  async function openPlayback(targetLayoutId = layoutId, timestamp = selectedTime, automatic = false) {
    if (!targetLayoutId) return;
    const targetLayout = layouts.find((item) => item.id === targetLayoutId);
    setOpening(true);
    setActionError('');
    setPlaying(false);
    try {
      const prepared = await api('/api/playback/prepare', {
        method: 'POST',
        body: JSON.stringify({ layout_id: targetLayoutId, selected_timestamp: timestamp.toISOString() }),
      });
      setResult(prepared);
      await loadTimeline(targetLayout, timestamp);
      setAutoOpenedId(targetLayoutId);
    } catch (err) {
      setActionError(err.message || 'Unable to open playback.');
    } finally {
      setOpening(false);
    }
  }

  function handleLayoutChange(nextLayoutId) {
    setLayoutId(nextLayoutId);
    setResult(null);
    setTimeline(null);
    setAutoOpenedId('');
    setFocusedCameraId('');
    setPlaying(false);
  }

  function handleTimeChange(value) {
    const next = new Date(value);
    if (Number.isNaN(next.getTime())) return;
    setSelectedTime(next);
    setAutoOpenedId('');
    openPlayback(layoutId, next);
  }

  function togglePlayback() {
    const next = !playing;
    videoRefs.current.forEach((video) => {
      if (!video) return;
      if (next) video.play?.().catch(() => {});
      else video.pause?.();
    });
    setPlaying(next);
  }

  const playableCount = (result?.cameras || []).filter((camera) => camera.status === 'ok').length;

  return (
    <Panel title="Playback Review">
      <div className="playback-toolbar">
        <Field label="Layout">
          <select value={layoutId} onChange={(e) => handleLayoutChange(e.target.value)} disabled={!layouts.length}>
            {layouts.length === 0 && <option value="">No layouts</option>}
            {layouts.map((l) => <option key={l.id} value={l.id}>{l.name}</option>)}
          </select>
        </Field>
        <Field label="Date and time">
          <input type="datetime-local" value={toLocalInputValue(selectedTime)} onChange={(e) => handleTimeChange(e.target.value)} />
        </Field>
        <button onClick={() => openPlayback(layoutId, selectedTime)} disabled={!layoutId || opening}>{opening ? 'Opening...' : 'Open playback'}</button>
        <button onClick={togglePlayback} disabled={!playableCount}>{playing ? 'Pause all' : 'Play all'}</button>
      </div>
      <State loading={loading} error={error} />
      {actionError && <ErrorText message={actionError} />}
      {!layouts.length && !loading && <EmptyState title="No layouts" body="Create a layout to start playing recorded footage." />}
      {layout && (
        <PlaybackTimeline
          layout={layout}
          cameras={result?.cameras || []}
          timeline={timeline}
          selectedTime={selectedTime}
          resultLoaded={!!result}
          onSelect={(timestamp) => {
            setSelectedTime(timestamp);
            setAutoOpenedId('');
            openPlayback(layoutId, timestamp);
          }}
        />
      )}
      {result && (
        <PlaybackLayoutGrid
          layout={layout}
          cameras={result?.cameras || []}
          playing={playing}
          videoRefs={videoRefs}
          focusedCameraId={focusedCameraId}
          onFocusCamera={setFocusedCameraId}
          selectedTime={selectedTime}
          timeline={timeline}
          onSelectTime={(timestamp) => {
            setSelectedTime(timestamp);
            setAutoOpenedId('');
            openPlayback(layoutId, timestamp);
          }}
          onTogglePlayback={togglePlayback}
        />
      )}
      {!result && layout && !opening && <EmptyState title="Playback is loading" body={`${layout.name} opens automatically when the page is ready.`} />}
    </Panel>
  );
}

function PlaybackTimeline({ layout, cameras, timeline, selectedTime, resultLoaded, onSelect }) {
  const [start, end] = dayBounds(selectedTime);
  const duration = end.getTime() - start.getTime();
  const playheadLeft = pct((selectedTime.getTime() - start.getTime()) / duration);
  const availability = new Map((timeline?.camera_availability || []).map((item) => [item.camera_id, item]));
  const cameraNames = new Map(cameras.map((camera) => [camera.camera_id, camera.camera_name || shortID(camera.camera_id)]));
  const visibleCameraIDs = resultLoaded ? new Set(cameras.map((camera) => camera.camera_id)) : null;
  const items = (layout?.layout_items || []).filter((item) => item.camera_id && (!visibleCameraIDs || visibleCameraIDs.has(item.camera_id)));
  const hasRanges = (timeline?.camera_availability || []).some((item) => (item.ranges || []).length > 0);

  function selectFromEvent(event) {
    const rect = event.currentTarget.getBoundingClientRect();
    const ratio = Math.min(Math.max((event.clientX - rect.left) / rect.width, 0), 1);
    onSelect(new Date(start.getTime() + ratio * duration));
  }

  return (
    <section className="timeline-panel">
      <div className="timeline-header">
        <div>
          <strong>Recording timeline</strong>
          <span>{formatDateTime(selectedTime)}</span>
        </div>
        <span>{hasRanges ? 'Recorded ranges are highlighted' : 'No stored video on this day'}</span>
      </div>
      <div className="timeline-scale">
        {['00:00', '04:00', '08:00', '12:00', '16:00', '20:00', '24:00'].map((label) => <span key={label}>{label}</span>)}
      </div>
      <div className="timeline-stack">
        {items.map((item) => {
          const cameraID = item.camera_id;
          const row = availability.get(cameraID) || { ranges: [] };
          return (
            <div className="timeline-row" key={item.id || item.item_id || cameraID}>
              <span title={cameraNames.get(cameraID) || cameraID}>{cameraNames.get(cameraID) || shortID(cameraID)}</span>
              <button className="timeline-track" type="button" onClick={selectFromEvent} aria-label={`Select playback time for ${cameraNames.get(cameraID) || cameraID}`}>
                {(row.ranges || []).map((range, index) => (
                  <span
                    className="timeline-range"
                    key={`${range.start_time}-${index}`}
                    style={rangeStyle(range.start_time, range.end_time, start, end)}
                  />
                ))}
                <span className="timeline-playhead" style={{ left: `${playheadLeft}%` }} />
              </button>
            </div>
          );
        })}
      </div>
    </section>
  );
}

function PlaybackLayoutGrid({ layout, cameras, playing, videoRefs, focusedCameraId, onFocusCamera, selectedTime, timeline, onSelectTime, onTogglePlayback }) {
  const byCamera = new Map(cameras.map((camera) => [camera.camera_id, camera]));
  const tileRefs = React.useRef(new Map());
  const [fullscreenFallbackId, setFullscreenFallbackId] = useState('');
  const layoutItems = (layout?.layout_items || []).filter((item) => item.camera_id && byCamera.has(item.camera_id));
  const items = layout ? layoutItems : cameras.map((camera, index) => ({
    camera_id: camera.camera_id,
    x: index % 2,
    y: Math.floor(index / 2),
    width: 1,
    height: 1,
  }));
  const columns = Math.max(1, Number(layout?.settings?.columns) || Math.max(...items.map((item) => Number(item.x || 0) + Number(item.width || 1)), 1));
  const rows = Math.max(1, Math.max(...items.map((item) => Number(item.y || 0) + Number(item.height || 1)), 1));
  const focusedItem = items.find((item) => item.camera_id === focusedCameraId);
  const visibleFocusedItem = focusedItem || null;
  const sideItems = visibleFocusedItem ? items.filter((item) => item.camera_id !== visibleFocusedItem.camera_id) : [];

  useEffect(() => {
    const onFullscreenChange = () => {
      if (!document.fullscreenElement) setFullscreenFallbackId('');
    };
    document.addEventListener('fullscreenchange', onFullscreenChange);
    return () => document.removeEventListener('fullscreenchange', onFullscreenChange);
  }, []);

  if (!items.length) return <EmptyState title="No available channels" body="No camera channels are available for this layout." />;

  function requestFullscreen(cameraID) {
    const target = tileRefs.current.get(cameraID) || videoRefs.current.get(cameraID);
    if (!target?.requestFullscreen) {
      setFullscreenFallbackId(cameraID);
      return;
    }
    target.requestFullscreen().then(() => {
      setFullscreenFallbackId('');
    }).catch(() => {
      setFullscreenFallbackId(cameraID);
    });
  }

  function renderTile(item, options = {}) {
    const camera = byCamera.get(item.camera_id) || { camera_id: item.camera_id, status: 'no_recording' };
    const isFocused = camera.camera_id === focusedCameraId;
    const isFullscreenFallback = camera.camera_id === fullscreenFallbackId;
    const isNoData = camera.status === 'no_recording';
    const register = (video) => {
      if (video) videoRefs.current.set(camera.camera_id, video);
      else videoRefs.current.delete(camera.camera_id);
    };
    return (
      <section
        className={'video-tile layout-video-tile' + (options.compact ? ' compact' : '') + (isFocused ? ' focused' : '') + (isFullscreenFallback ? ' fullscreen-fallback' : '') + (isNoData ? ' no-data' : '')}
        key={item.id || item.item_id || item.camera_id}
        ref={(node) => {
          if (node) tileRefs.current.set(camera.camera_id, node);
          else tileRefs.current.delete(camera.camera_id);
        }}
        style={options.style}
      >
        <div className="playback-tile-bar">
          <div>
            <strong>{camera.camera_name || shortID(camera.camera_id)}</strong>
            <span><PlaybackStatusBadge status={camera.status} /></span>
          </div>
          <div className="playback-tile-actions">
            {!options.compact && (
              <button type="button" onClick={() => (isFocused ? onFocusCamera('') : onFocusCamera(camera.camera_id))}>
                {isFocused ? 'Exit focus' : 'Focus'}
              </button>
            )}
            {options.compact && <button type="button" onClick={() => onFocusCamera(camera.camera_id)}>Focus</button>}
            {camera.status === 'ok' && (
              <button type="button" onClick={() => (isFullscreenFallback ? setFullscreenFallbackId('') : requestFullscreen(camera.camera_id))}>
                {isFullscreenFallback ? 'Exit fullscreen' : 'Fullscreen'}
              </button>
            )}
          </div>
        </div>
        {camera.status === 'ok' && (
          <>
            <VideoPlayer
              src={camera.segment?.playback_url}
              offsetSeconds={camera.offset_seconds || 0}
              controlled
              playing={playing}
              register={register}
            />
            <PlaybackFullscreenControls
              cameraID={camera.camera_id}
              playing={playing}
              selectedTime={selectedTime}
              timeline={timeline}
              onSelectTime={onSelectTime}
              onTogglePlayback={onTogglePlayback}
            />
          </>
        )}
        {camera.status !== 'ok' && (
          camera.status === 'no_recording'
            ? <NoDataPlaybackState />
            : <p>{playbackStatusText(camera.status)}</p>
        )}
      </section>
    );
  }

  if (visibleFocusedItem) {
    return (
      <div className="playback-focus-layout">
        <div className="playback-focus-main">
          {renderTile(visibleFocusedItem, { style: {} })}
        </div>
        {sideItems.length > 0 && (
          <aside className="playback-focus-strip">
            {sideItems.map((item) => renderTile(item, { compact: true }))}
          </aside>
        )}
      </div>
    );
  }

  return (
    <div className="layout-playback-grid" style={{ gridTemplateColumns: `repeat(${columns}, minmax(0, 1fr))`, gridAutoRows: 'minmax(180px, auto)' }}>
      {items.map((item) => renderTile(item, {
        style: {
          gridColumn: `${Number(item.x || 0) + 1} / span ${Math.max(Number(item.width || 1), 1)}`,
          gridRow: `${Number(item.y || 0) + 1} / span ${Math.max(Number(item.height || 1), 1)}`,
        },
      }))}
    </div>
  );
}

function PlaybackFullscreenControls({ cameraID, playing, selectedTime, timeline, onSelectTime, onTogglePlayback }) {
  const [start, end] = dayBounds(selectedTime);
  const duration = end.getTime() - start.getTime();
  const playheadLeft = pct((selectedTime.getTime() - start.getTime()) / duration);
  const availability = (timeline?.camera_availability || []).find((item) => item.camera_id === cameraID);

  function selectFromEvent(event) {
    const rect = event.currentTarget.getBoundingClientRect();
    const ratio = Math.min(Math.max((event.clientX - rect.left) / rect.width, 0), 1);
    onSelectTime(new Date(start.getTime() + ratio * duration));
  }

  return (
    <div className="playback-fullscreen-controls">
      <button type="button" onClick={onTogglePlayback}>{playing ? 'Pause' : 'Play'}</button>
      <div className="fullscreen-timeline-wrap">
        <span>{formatTimeOnly(start)}</span>
        <button className="fullscreen-timeline-track" type="button" onClick={selectFromEvent} aria-label="Select playback time">
          {(availability?.ranges || []).map((range, index) => (
            <span
              className="timeline-range"
              key={`${range.start_time}-${index}`}
              style={rangeStyle(range.start_time, range.end_time, start, end)}
            />
          ))}
          <span className="timeline-playhead" style={{ left: `${playheadLeft}%` }} />
        </button>
        <span>{formatTimeOnly(selectedTime)}</span>
      </div>
    </div>
  );
}

function NoDataPlaybackState() {
  return (
    <div className="playback-no-data">
      <div className="playback-no-data-core">
        <span />
        <strong>No data in this time yet</strong>
      </div>
    </div>
  );
}

function VideoGrid({ cameras, live = false }) {
  return (
    <div className="video-grid">
      {cameras.map((camera) => (
        <section className="video-tile" key={camera.camera_id}>
          <strong>{camera.camera_id}</strong>
          <span><LiveStatusBadge status={camera.status} /></span>
          {camera.status === 'ok' && <VideoPlayer src={live ? camera.hls_url : camera.segment?.playback_url} autoPlay={live} />}
          {camera.status !== 'ok' && <p>{camera.status === 'no_recording' ? 'No recording for this time.' : camera.status}</p>}
        </section>
      ))}
    </div>
  );
}

function preferredLayout(layouts) {
  return layouts.find((item) => item.name === 'Main') || layouts.find((item) => item.is_default) || layouts[0];
}

function dayBounds(date) {
  const start = new Date(date);
  start.setHours(0, 0, 0, 0);
  const end = new Date(start);
  end.setDate(end.getDate() + 1);
  return [start, end];
}

function pct(value) {
  if (!Number.isFinite(value)) return 0;
  return Math.min(Math.max(value * 100, 0), 100);
}

function rangeStyle(startValue, endValue, dayStart, dayEnd) {
  const start = new Date(startValue).getTime();
  const end = new Date(endValue || startValue).getTime();
  const min = dayStart.getTime();
  const max = dayEnd.getTime();
  const duration = max - min;
  const left = pct((Math.max(start, min) - min) / duration);
  const right = pct((Math.min(end, max) - min) / duration);
  return { left: `${left}%`, width: `${Math.max(right - left, 0.4)}%` };
}

function LiveLayoutGrid({ layout, cameras, editable, onChange, onError }) {
  const byCamera = new Map(cameras.map((camera) => [camera.camera_id, camera]));
  const items = (layout.layout_items || [])
    .filter((item) => item.camera_id && byCamera.has(item.camera_id))
    .slice()
    .sort((a, b) => (a.display_order || 0) - (b.display_order || 0));
  const cols = Math.max(1, Number(layout.settings?.columns) || Math.max(...items.map((item) => Number(item.x || 0) + Number(item.width || 1)), 1));
  const maxRow = items.reduce((m, item) => Math.max(m, Number(item.y || 0) + Number(item.height || 1)), 0);
  const rows = Math.max(1, maxRow || 1);
  const [selectedId, setSelectedId] = useState(null);
  const [drag, setDrag] = useState(null);
  const gridRef = React.useRef(null);

  function itemID(item) {
    return item.id || item.item_id;
  }

  function gridRect() {
    const el = gridRef.current;
    if (!el) return null;
    const r = el.getBoundingClientRect();
    return { left: r.left, top: r.top, width: r.width, height: r.height };
  }

  function startDrag(item, mode, event) {
    if (!editable) return;
    event.preventDefault();
    event.stopPropagation();
    setSelectedId(itemID(item));
    setDrag({
      itemId: itemID(item),
      mode,
      startMouseX: event.clientX,
      startMouseY: event.clientY,
      startX: Number(item.x || 0),
      startY: Number(item.y || 0),
      startW: Math.max(1, Number(item.width || 1)),
      startH: Math.max(1, Number(item.height || 1)),
    });
  }

  function updatedItemsForDrag(event) {
    if (!drag) return items;
    const rect = gridRect();
    if (!rect) return items;
    const cw = rect.width / cols || 1;
    const ch = rect.height / rows || 1;
    const dxc = Math.round((event.clientX - drag.startMouseX) / cw);
    const dyc = Math.round((event.clientY - drag.startMouseY) / ch);
    return items.map((item) => {
      if (itemID(item) !== drag.itemId) return item;
      let x = drag.startX;
      let y = drag.startY;
      let width = drag.startW;
      let height = drag.startH;
      if (drag.mode === 'move') {
        x = Math.max(0, Math.min(cols - width, drag.startX + dxc));
        y = Math.max(0, drag.startY + dyc);
      } else {
        if (drag.mode.includes('w')) {
          x = Math.max(0, drag.startX + dxc);
          width = Math.max(1, drag.startW - (x - drag.startX));
        }
        if (drag.mode.includes('e')) width = Math.max(1, drag.startW + dxc);
        if (drag.mode.includes('n')) {
          y = Math.max(0, drag.startY + dyc);
          height = Math.max(1, drag.startH - (y - drag.startY));
        }
        if (drag.mode.includes('s')) height = Math.max(1, drag.startH + dyc);
        if (x + width > cols) width = Math.max(1, cols - x);
      }
      return { ...item, x, y, width, height };
    });
  }

  async function saveItem(item) {
    try {
      await api(`/api/layouts/${layout.id}/items/${itemID(item)}`, {
        method: 'PATCH',
        body: JSON.stringify({
          camera_id: item.camera_id,
          x: item.x,
          y: item.y,
          width: item.width,
          height: item.height,
          display_order: item.display_order,
          tile_type: item.tile_type,
        }),
      });
    } catch (err) {
      onError(err.message || 'Unable to save layout.');
    }
  }

  function onMouseMove(event) {
    if (!drag) return;
    onChange(updatedItemsForDrag(event));
  }

  function onMouseUp(event) {
    if (!drag) return;
    const updated = updatedItemsForDrag(event);
    const changed = updated.find((item) => itemID(item) === drag.itemId);
    setDrag(null);
    onChange(updated);
    if (changed) saveItem(changed);
  }

  if (!items.length) return <EmptyState title="No available channels" body="No camera channels are available for this layout." />;

  return (
    <div
      ref={gridRef}
      className={'live-layout-grid' + (editable ? ' editable' : '')}
      style={{ gridTemplateColumns: `repeat(${cols}, minmax(0, 1fr))` }}
      onMouseMove={onMouseMove}
      onMouseUp={onMouseUp}
      onMouseLeave={onMouseUp}
    >
      {items.map((item) => {
        const id = itemID(item);
        const camera = byCamera.get(item.camera_id) || { camera_id: item.camera_id, status: 'stream_unavailable' };
        return (
          <section
            key={id}
            className={'video-tile live-layout-tile' + (selectedId === id ? ' selected' : '')}
            style={{
              gridColumn: `${Number(item.x || 0) + 1} / span ${Math.max(1, Number(item.width || 1))}`,
              gridRow: `${Number(item.y || 0) + 1} / span ${Math.max(1, Number(item.height || 1))}`,
            }}
            onClick={() => editable && setSelectedId(id)}
          >
            <div className="live-tile-bar" onMouseDown={(event) => startDrag(item, 'move', event)}>
              <strong>{camera.camera_name || shortID(camera.camera_id)}</strong>
              <span><LiveStatusBadge status={camera.status} /></span>
            </div>
            <div className="live-tile-video">
              {camera.status === 'ok' && <VideoPlayer src={camera.hls_url} autoPlay />}
              {camera.status !== 'ok' && <p>{liveStatusText(camera.status)}</p>}
            </div>
            {editable && selectedId === id && (
              <>
                {['nw','n','ne','e','se','s','sw','w'].map((handle) => (
                  <span
                    key={handle}
                    className={`layout-handle layout-handle-${handle}`}
                    onMouseDown={(event) => startDrag(item, handle, event)}
                  />
                ))}
              </>
            )}
          </section>
        );
      })}
    </div>
  );
}

function VideoPlayer({ src, offsetSeconds = 0, controlled = false, register, playing = false, autoPlay = false }) {
  const ref = React.useRef(null);
  const [playbackError, setPlaybackError] = useState('');

  useEffect(() => {
    if (!src || !ref.current) return;
    const video = ref.current;
    const url = `${apiBaseUrl}${src}`;
    const isHLS = src.includes('.m3u8');
    setPlaybackError('');
    const onVideoError = () => {
      const code = video.error?.code;
      setPlaybackError(code ? `Video stream error (${code}).` : 'Video stream error.');
    };
    video.addEventListener('error', onVideoError);
    if (isHLS && Hls.isSupported()) {
      const hls = new Hls({ xhrSetup: (xhr) => { xhr.withCredentials = true; } });
      hls.on(Hls.Events.ERROR, (_event, data) => {
        if (data?.fatal) {
          setPlaybackError(`Live stream error: ${data.details || data.type || 'unavailable'}.`);
        }
      });
      hls.loadSource(url);
      hls.attachMedia(video);
      return () => {
        video.removeEventListener('error', onVideoError);
        hls.destroy();
      };
    }
    if (isHLS && video.canPlayType('application/vnd.apple.mpegurl')) {
      video.src = url;
      return () => video.removeEventListener('error', onVideoError);
    }
    video.src = url;
    return () => video.removeEventListener('error', onVideoError);
  }, [src]);
  useEffect(() => {
    const video = ref.current;
    if (!video || !autoPlay || controlled || !src) return undefined;
    const play = () => video.play().catch(() => {});
    video.addEventListener('canplay', play);
    play();
    return () => video.removeEventListener('canplay', play);
  }, [autoPlay, controlled, src]);
  useEffect(() => {
    if (!ref.current || !register) return undefined;
    register(ref.current);
    return () => register(null);
  }, [register]);
  useEffect(() => {
    const video = ref.current;
    if (!video || !src) return undefined;
    const seek = () => {
      if (Number.isFinite(offsetSeconds) && offsetSeconds > 0 && video.duration) {
        video.currentTime = Math.min(offsetSeconds, Math.max(video.duration - 0.2, 0));
      }
    };
    video.addEventListener('loadedmetadata', seek);
    seek();
    return () => video.removeEventListener('loadedmetadata', seek);
  }, [src, offsetSeconds]);
  useEffect(() => {
    const video = ref.current;
    if (!video || !controlled) return;
    if (playing) {
      video.play().catch(() => {});
    } else {
      video.pause();
    }
  }, [controlled, playing, src]);
  return (
    <>
      <video ref={ref} controls={!controlled} muted playsInline autoPlay={autoPlay && !controlled} />
      {playbackError && <ErrorText message={playbackError} />}
    </>
  );
}

function toLocalInputValue(date) {
  const local = new Date(date.getTime() - date.getTimezoneOffset() * 60000);
  return local.toISOString().slice(0, 16);
}

function formatDateTime(value) {
  const date = value instanceof Date ? value : new Date(value);
  if (Number.isNaN(date.getTime())) return '';
  return date.toLocaleString();
}

function formatTimeOnly(value) {
  const date = value instanceof Date ? value : new Date(value);
  if (Number.isNaN(date.getTime())) return '';
  return date.toLocaleTimeString([], { hour: '2-digit', minute: '2-digit' });
}

function shortID(id) {
  return id ? id.slice(0, 8) : 'camera';
}

function playbackStatusText(status) {
  if (status === 'ok') return 'Ready';
  if (status === 'no_recording') return 'No data in this time yet.';
  if (status === 'forbidden') return 'Permission required.';
  return status || 'Unavailable';
}

function liveStatusText(status) {
  if (status === 'starting') return 'Starting stream...';
  if (status === 'camera_disabled') return 'Camera disabled.';
  if (status === 'stream_disabled') return 'Live stream disabled.';
  if (status === 'stream_unavailable') return 'Stream unavailable.';
  if (status === 'no_permission') return 'No permission.';
  return status || 'Unavailable';
}

function formatStorageRow(item) {
  return {
    ...item,
    enabled: item.is_enabled ? 'Yes' : 'No',
    exists: item.exists ? 'Yes' : 'No',
    writable: item.writable ? 'Yes' : 'No',
    used_percent: `${Number(item.used_percent || 0).toFixed(1)}%`,
    free_bytes: formatBytes(item.free_bytes || 0),
    latest_validation_error: item.latest_validation_error || '',
  };
}

function clampPercent(value) {
  const percent = Number(value || 0);
  if (!Number.isFinite(percent)) return 0;
  return Math.max(0, Math.min(100, percent));
}

function storageSummary(items) {
  const summary = items.reduce((acc, item) => {
    const used = Number(item.used_bytes || 0);
    const free = Number(item.free_bytes || 0);
    const total = Number(item.total_bytes || used + free || 0);
    acc.used += used;
    acc.free += free;
    acc.total += total;
    if (item.is_enabled) acc.enabled += 1;
    if (storageStatus(item).kind !== 'ok') acc.problemCount += 1;
    return acc;
  }, { used: 0, free: 0, total: 0, enabled: 0, problemCount: 0 });
  summary.usedPercent = summary.total ? (summary.used / summary.total) * 100 : 0;
  return summary;
}

function storageStatus(item) {
  if (item.latest_validation_error || !item.exists || !item.writable || item.health_status === 'error') {
    return { kind: 'error', badge: 'error', label: 'error' };
  }
  const usedPercent = clampPercent(item.used_percent);
  if (item.health_status === 'warning' || usedPercent >= 90) {
    return { kind: 'warning', badge: item.is_enabled ? 'warning' : 'muted', label: item.is_enabled ? 'warning' : 'disabled' };
  }
  if (!item.is_enabled) {
    return { kind: 'muted', badge: 'muted', label: 'disabled' };
  }
  return { kind: 'ok', badge: 'ok', label: item.health_status || 'healthy' };
}

function formatCameraRow(item) {
  return {
    ...item,
    max_storage_bytes: item.max_storage_bytes ? formatBytes(item.max_storage_bytes) : '',
  };
}

function formatEventRow(item) {
  return {
    ...item,
    created_at: item.created_at ? new Date(item.created_at).toLocaleString() : '',
  };
}

function formatBytes(value) {
  const bytes = Number(value || 0);
  if (bytes < 1024) return `${bytes} B`;
  const units = ['KB', 'MB', 'GB', 'TB'];
  let size = bytes / 1024;
  let unitIndex = 0;
  while (size >= 1024 && unitIndex < units.length - 1) {
    size /= 1024;
    unitIndex += 1;
  }
  return `${size.toFixed(1)} ${units[unitIndex]}`;
}

function recorderStatusKind(row) {
  if (!row) return 'muted';
  const seen = row.last_seen_at ? new Date(row.last_seen_at).getTime() : 0;
  const stale = !seen || Date.now() - seen > 60_000;
  if (row.status === 'running' && !stale) return 'ok';
  if (row.status === 'stopped') return 'muted';
  return 'warning';
}

function LayoutSummary({ layout }) {
  return (
    <section className="layout-row">
      <div><strong>{layout.name}</strong><span>{layout.is_default ? 'Default' : 'Layout'}</span></div>
      <DataTable rows={layout.layout_items || []} columns={['camera_id', 'x', 'y', 'width', 'height', 'display_order', 'tile_type']} />
    </section>
  );
}

function useLoader(fn) {
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState('');
  async function run() {
    setLoading(true);
    setError('');
    try {
      await fn();
    } catch (err) {
      setError(err.message);
    } finally {
      setLoading(false);
    }
  }
  return { loading, error, run };
}

function Panel({ title, children }) {
  return <section className="panel"><h2>{title}</h2>{children}</section>;
}

function FormGrid({ children, onSubmit }) {
  return <form className="form-grid" onSubmit={onSubmit}>{children}</form>;
}

function Field({ label, help, children }) {
  return <label className="field"><span>{label}</span>{children}{help && <small>{help}</small>}</label>;
}

function Toolbar({ children }) {
  return <div className="toolbar">{children}</div>;
}

function State({ loading, error }) {
  if (loading) return <p className="muted">Loading…</p>;
  if (error) return <ErrorText message={error} />;
  return null;
}

function ErrorText({ message }) {
  return <p className="error" role="alert">{message}</p>;
}

function DataTable({ rows, columns }) {
  if (!rows?.length) return <p className="muted">No rows.</p>;
  return (
    <div className="table-wrap">
      <table>
        <thead><tr>{columns.map((col) => <th key={col}>{col}</th>)}</tr></thead>
        <tbody>{rows.map((row, rowIndex) => <tr key={row.id || row.camera_id || rowIndex}>{columns.map((col) => <td key={col}>{renderCell(row[col])}</td>)}</tr>)}</tbody>
      </table>
    </div>
  );
}

function renderCell(value) {
  if (value === null || value === undefined || value === '') return <span className="muted">—</span>;
  if (React.isValidElement(value)) return value;
  return String(value);
}

function FullPageMessage({ text }) {
  return <main className="login-page"><div className="login-box"><p>{text}</p></div></main>;
}

function EmptyState({ title, body, action }) {
  return (
    <div className="empty-state">
      <strong>{title}</strong>
      {body && <p>{body}</p>}
      {action}
    </div>
  );
}

function AdminBadge() {
  return <span className="badge badge-admin">Admin</span>;
}

function StatusBadge({ kind, text }) {
  return <span className={`badge badge-${kind || 'muted'}`}>{text || 'unknown'}</span>;
}

function LiveStatusBadge({ status }) {
  if (status === 'ok') return <StatusBadge kind="ok" text="live" />;
  if (status === 'starting') return <StatusBadge kind="warning" text="starting" />;
  if (status === 'camera_disabled') return <StatusBadge kind="muted" text="disabled" />;
  if (status === 'stream_disabled') return <StatusBadge kind="muted" text="stream off" />;
  if (status === 'no_permission') return <StatusBadge kind="warning" text="no permission" />;
  if (status === 'stream_unavailable') return <StatusBadge kind="error" text="stream unavailable" />;
  return <StatusBadge kind="muted" text={status || 'unknown'} />;
}

function PlaybackStatusBadge({ status }) {
  if (status === 'ok') return <StatusBadge kind="ok" text="ready" />;
  if (status === 'no_recording') return <StatusBadge kind="muted" text="no data" />;
  if (status === 'forbidden') return <StatusBadge kind="warning" text="no permission" />;
  return <StatusBadge kind="error" text={status || 'unavailable'} />;
}

function AlertStatusBadge({ status }) {
  if (status === 'open') return <StatusBadge kind="error" text="open" />;
  if (status === 'acknowledged') return <StatusBadge kind="warning" text="acknowledged" />;
  if (status === 'resolved') return <StatusBadge kind="ok" text="resolved" />;
  return <StatusBadge kind="muted" text={status || 'unknown'} />;
}

function AlertSeverityBadge({ severity }) {
  if (severity === 'error') return <StatusBadge kind="error" text="error" />;
  if (severity === 'warning') return <StatusBadge kind="warning" text="warning" />;
  if (severity === 'info') return <StatusBadge kind="muted" text="info" />;
  if (severity === 'debug') return <StatusBadge kind="muted" text="debug" />;
  return <StatusBadge kind="muted" text={severity || 'unknown'} />;
}

const alertRuleTypeOptions = [
  { value: 'recorder_stale', label: 'Recorder stale' },
  { value: 'camera_recording_failed', label: 'Camera recording failed' },
  { value: 'storage_low_disk', label: 'Storage low disk' },
  { value: 'live_stream_failed', label: 'Live stream failed' },
];

function AlertsPage() {
  const [rules, setRules] = useState([]);
  const [alerts, setAlerts] = useState([]);
  const [ruleForm, setRuleForm] = useState({
    name: '',
    type: 'recorder_stale',
    severity: 'warning',
    cooldown_seconds: 300,
    enabled: true,
    threshold_text: '',
  });
  const [busy, setBusy] = useState(false);
  const { loading, error, run } = useLoader(load);

  async function load() {
    const [rulesData, alertsData] = await Promise.all([
      api('/api/alert-rules'),
      api('/api/alerts?limit=100'),
    ]);
    setRules(rulesData.alert_rules || []);
    setAlerts(alertsData.alerts || []);
  }
  useEffect(() => { run(); }, []);

  async function createRule(event) {
    event.preventDefault();
    setBusy(true);
    try {
      let threshold = {};
      const raw = (ruleForm.threshold_text || '').trim();
      if (raw) {
        try {
          threshold = JSON.parse(raw);
        } catch (err) {
          throw new Error(`threshold must be valid JSON: ${err.message}`);
        }
      }
      const payload = {
        name: ruleForm.name,
        type: ruleForm.type,
        severity: ruleForm.severity,
        cooldown_seconds: Number(ruleForm.cooldown_seconds),
        enabled: ruleForm.enabled,
        threshold,
      };
      await api('/api/alert-rules', { method: 'POST', body: JSON.stringify(payload) });
      setRuleForm({ ...ruleForm, name: '', threshold_text: '' });
      run();
    } catch (err) {
      alert(err.message);
    } finally {
      setBusy(false);
    }
  }

  async function toggleRule(rule) {
    await api(`/api/alert-rules/${rule.id}`, {
      method: 'PATCH',
      body: JSON.stringify({ enabled: !rule.enabled }),
    });
    run();
  }

  async function deleteRule(rule) {
    if (!window.confirm(`Delete alert rule "${rule.name}"? This also drops its alerts.`)) return;
    await api(`/api/alert-rules/${rule.id}`, { method: 'DELETE' });
    run();
  }

  async function acknowledgeAlert(alertItem) {
    await api(`/api/alerts/${alertItem.id}/acknowledge`, { method: 'POST', body: '{}' });
    run();
  }

  async function resolveAlert(alertItem) {
    await api(`/api/alerts/${alertItem.id}/resolve`, { method: 'POST', body: '{}' });
    run();
  }

  function describeType(ruleType) {
    switch (ruleType) {
      case 'recorder_stale': return 'Worker has not checked in within threshold.';
      case 'camera_recording_failed': return 'Too many recorder.job_failure events in window.';
      case 'storage_low_disk': return 'Storage location above used_percent threshold.';
      case 'live_stream_failed': return 'Too many live.failure events in window.';
      default: return ruleType;
    }
  }

  return (
    <Panel title="Alerts">
      <Toolbar>
        <button onClick={run}>Refresh</button>
      </Toolbar>
      <State loading={loading} error={error} />
      <h3>Rules</h3>
      <FormGrid onSubmit={createRule}>
        <input placeholder="Rule name" value={ruleForm.name} onChange={(e) => setRuleForm({ ...ruleForm, name: e.target.value })} required />
        <select value={ruleForm.type} onChange={(e) => setRuleForm({ ...ruleForm, type: e.target.value })}>
          {alertRuleTypeOptions.map((opt) => <option key={opt.value} value={opt.value}>{opt.label}</option>)}
        </select>
        <select value={ruleForm.severity} onChange={(e) => setRuleForm({ ...ruleForm, severity: e.target.value })}>
          <option value="info">info</option>
          <option value="warning">warning</option>
          <option value="error">error</option>
        </select>
        <input type="number" min="0" placeholder="Cooldown seconds" value={ruleForm.cooldown_seconds} onChange={(e) => setRuleForm({ ...ruleForm, cooldown_seconds: e.target.value })} />
        <label className="check"><input type="checkbox" checked={ruleForm.enabled} onChange={(e) => setRuleForm({ ...ruleForm, enabled: e.target.checked })} /> Enabled</label>
        <input placeholder='Threshold JSON (e.g. {"min_used_percent": 90})' value={ruleForm.threshold_text} onChange={(e) => setRuleForm({ ...ruleForm, threshold_text: e.target.value })} />
        <button type="submit" disabled={busy}>Create rule</button>
      </FormGrid>
      {rules.length ? (
        <DataTable
          rows={rules.map((rule) => ({
            ...rule,
            enabled: rule.enabled ? <StatusBadge kind="ok" text="on" /> : <StatusBadge kind="muted" text="off" />,
            severity: <AlertSeverityBadge severity={rule.severity} />,
            actions: (
              <span className="row-actions">
                <button type="button" onClick={() => toggleRule(rule)}>{rule.enabled ? 'Disable' : 'Enable'}</button>
                <button type="button" onClick={() => deleteRule(rule)}>Delete</button>
              </span>
            ),
          }))}
          columns={['name', 'type', 'enabled', 'severity', 'cooldown_seconds', 'actions']}
        />
      ) : (
        !loading && <EmptyState title="No alert rules" body={`Create a rule. Available types: ${alertRuleTypeOptions.map((o) => o.label).join(', ')}.`} />
      )}
      <p className="muted rule-help">Type details: {alertRuleTypeOptions.map((o) => `${o.label}: ${describeType(o.value)}`).join(' • ')}</p>
      <h3>Alerts</h3>
      {alerts.length ? (
        <DataTable
          rows={alerts.map((alertItem) => ({
            ...alertItem,
            opened_at: alertItem.opened_at ? new Date(alertItem.opened_at).toLocaleString() : '',
            severity: <AlertSeverityBadge severity={alertItem.severity} />,
            status: <AlertStatusBadge status={alertItem.status} />,
            actions: alertItem.status === 'resolved' ? <span className="muted">—</span> : (
              <span className="row-actions">
                {alertItem.status === 'open' && <button type="button" onClick={() => acknowledgeAlert(alertItem)}>Acknowledge</button>}
                <button type="button" onClick={() => resolveAlert(alertItem)}>Resolve</button>
              </span>
            ),
          }))}
          columns={['opened_at', 'rule_name', 'rule_type', 'severity', 'status', 'message', 'actions']}
        />
      ) : (
        !loading && <EmptyState title="No alerts" body="Open alerts will appear here once a rule fires." />
      )}
    </Panel>
  );
}

createRoot(document.getElementById('root')).render(<App />);
