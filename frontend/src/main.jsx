import React, { createContext, useContext, useEffect, useMemo, useState } from 'react';
import {
  BrowserRouter,
  Link,
  Navigate,
  NavLink,
  Outlet,
  Route,
  Routes,
  useLocation,
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

function useIsMobileLayout() {
  const [isMobile, setIsMobile] = useState(() => (
    typeof window !== 'undefined' ? window.matchMedia('(max-width: 760px)').matches : false
  ));
  useEffect(() => {
    if (typeof window === 'undefined') return undefined;
    const query = window.matchMedia('(max-width: 760px)');
    const update = () => setIsMobile(query.matches);
    update();
    query.addEventListener('change', update);
    return () => query.removeEventListener('change', update);
  }, []);
  return isMobile;
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
            <Route path="notifications" element={<AdminOnly><NotificationsPage /></AdminOnly>} />
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
  const location = useLocation();
  const [menuOpen, setMenuOpen] = useState(false);

  useEffect(() => {
    setMenuOpen(false);
  }, [location.pathname]);

  async function logout() {
    await api('/api/auth/logout', { method: 'POST', body: '{}' });
    setUser(null);
    navigate('/login');
  }

  return (
    <div className={`shell${menuOpen ? ' mobile-menu-open' : ''}`}>
      <aside
        className={`sidebar${menuOpen ? ' open' : ''}`}
        onClick={(event) => {
          if (event.target.closest?.('a')) setMenuOpen(false);
        }}
      >
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
            <NavLink to="/notifications">Notifications</NavLink>
            <NavLink to="/alerts">Alerts</NavLink>
          </nav>
        )}
      </aside>
      <button
        className="mobile-menu-backdrop"
        type="button"
        aria-label="Close navigation menu"
        onClick={() => setMenuOpen(false)}
      />
      <main className="content">
        <header className="topbar">
          <button
            className="mobile-menu-button"
            type="button"
            aria-label="Open navigation menu"
            aria-expanded={menuOpen}
            onClick={() => setMenuOpen((value) => !value)}
          >
            <span />
            <span />
            <span />
          </button>
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
      storage: (storage.storage_locations || []).map(normalizeStorageLocation),
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

  const summary = healthSummary(data);

  return (
    <Panel title="Health">
      <section className={`health-hero health-hero-${summary.tone}`}>
        <div className="health-hero-copy">
          <span>System health</span>
          <h3>{summary.title}</h3>
          <p>{summary.message}</p>
        </div>
        <div className="health-score" style={{ '--score': `${summary.score}%` }}>
          <strong>{summary.score}</strong>
          <span>score</span>
        </div>
        <button type="button" onClick={run} disabled={loading}>{loading ? 'Refreshing...' : 'Refresh'}</button>
      </section>
      <State loading={loading} error={error} />

      <div className="health-kpi-grid">
        <HealthKPI title="Backend" value={data.health?.status || 'unknown'} detail={`Database: ${data.health?.database || 'unknown'}`} tone={data.health?.status === 'ok' && data.health?.database === 'ok' ? 'ok' : 'warning'} />
        <HealthKPI title="Recorder" value={`${summary.recorderOnline}/${summary.recorderTotal}`} detail={`${summary.activeJobs} active jobs`} tone={summary.recorderOnline ? 'ok' : 'error'} />
        <HealthKPI title="Storage" value={`${summary.storageHealthy}/${summary.storageTotal}`} detail={`${formatBytes(summary.storageFree)} free`} tone={summary.storageProblems ? 'warning' : 'ok'} />
        <HealthKPI title="Cameras" value={`${summary.recordingCameras}/${summary.enabledCameras}`} detail={`${summary.streamingCameras} live enabled`} tone={summary.enabledCameras ? 'ok' : 'warning'} />
      </div>

      <section className="health-section">
        <div className="health-section-heading">
          <div>
            <h3>Open Alerts</h3>
            <p>{data.openAlerts.length ? 'Items that need operator attention.' : 'No active alert requires attention.'}</p>
          </div>
          <StatusBadge kind={data.openAlerts.length ? 'warning' : 'ok'} text={`${data.openAlerts.length} open`} />
        </div>
        {data.openAlerts.length ? (
          <div className="health-alert-list">
            {data.openAlerts.map((alertItem) => (
              <article className="health-alert-card" key={alertItem.id}>
                <div>
                  <AlertSeverityBadge severity={alertItem.severity} />
                  <AlertStatusBadge status={alertItem.status} />
                </div>
                <strong>{alertItem.rule_name || humanizeKey(alertItem.event_type || 'Alert')}</strong>
                <p>{alertItem.message}</p>
                <small>Opened {formatRelativeTime(alertItem.opened_at)}</small>
                <span className="row-actions">
                  {alertItem.status === 'open' && <button type="button" onClick={() => acknowledgeAlert(alertItem)}>Acknowledge</button>}
                  <button type="button" onClick={() => resolveAlert(alertItem)}>Resolve</button>
                </span>
              </article>
            ))}
          </div>
        ) : (
          !loading && <EmptyState title="No open alerts" body="All monitored thresholds are currently clear." />
        )}
      </section>

      <div className="health-card-grid">
        <section className="health-section">
          <div className="health-section-heading">
            <div>
              <h3>Recorder Workers</h3>
              <p>Worker heartbeat freshness and active ffmpeg workload.</p>
            </div>
          </div>
          {data.recorder?.heartbeats?.length ? (
            <div className="health-list">
              {data.recorder.heartbeats.map((row) => (
                <HealthListItem
                  key={row.worker_id}
                  title={row.worker_id}
                  meta={`${row.active_job_count || 0} active jobs`}
                  detail={`Last seen ${formatRelativeTime(row.last_seen_at)}`}
                  badge={<StatusBadge kind={recorderStatusKind(row)} text={recorderStatusText(row)} />}
                />
              ))}
            </div>
          ) : (
            <EmptyState title="No recorder heartbeats" body="The recorder worker has not checked in yet." />
          )}
        </section>

        <section className="health-section">
          <div className="health-section-heading">
            <div>
              <h3>Storage Capacity</h3>
              <p>Recording folders, writeability, and disk pressure.</p>
            </div>
          </div>
          {data.storage.length ? (
            <div className="health-storage-list">
              {data.storage.map((item) => <HealthStorageRow key={item.id} item={item} />)}
            </div>
          ) : (
            <EmptyState title="No storage configured" body="Add a storage location to start recording." />
          )}
        </section>

        <section className="health-section">
          <div className="health-section-heading">
            <div>
              <h3>Cameras</h3>
              <p>Channel availability, recording state, and live-stream readiness.</p>
            </div>
          </div>
          {data.cameras.length ? (
            <div className="health-camera-grid">
              {data.cameras.map((camera) => <HealthCameraCard key={camera.id} camera={camera} />)}
            </div>
          ) : (
            <EmptyState title="No cameras" body="Add a camera to begin capturing RTSP streams." />
          )}
        </section>

        <section className="health-section">
          <div className="health-section-heading">
            <div>
              <h3>Recorder Jobs</h3>
              <p>Current recording processes and latest errors.</p>
            </div>
          </div>
          {data.recorder?.active_jobs?.length ? (
            <div className="health-list">
              {data.recorder.active_jobs.map((job) => (
                <HealthListItem
                  key={job.camera_id || `${job.camera_name}-${job.worker_id}`}
                  title={job.camera_name || 'Camera'}
                  meta={humanizeKey(job.status || 'unknown')}
                  detail={job.last_error || `Updated ${formatRelativeTime(job.updated_at)}`}
                  badge={<StatusBadge kind={job.status === 'running' ? 'ok' : 'warning'} text={job.status || 'unknown'} />}
                />
              ))}
            </div>
          ) : (
            <EmptyState title="No recorder jobs" body="No ffmpeg recording processes are currently running." />
          )}
        </section>
      </div>

      <section className="health-section">
        <div className="health-section-heading">
          <div>
            <h3>Latest Segments</h3>
            <p>Most recent saved recording clips.</p>
          </div>
        </div>
        {data.recorder?.last_segments?.length ? (
          <div className="health-list">
            {data.recorder.last_segments.map((segment) => (
              <HealthListItem
                key={segment.id || `${segment.camera_name}-${segment.start_time}`}
                title={segment.camera_name || 'Camera'}
                meta={humanizeKey(segment.status || 'segment')}
                detail={segment.start_time && segment.end_time ? `${formatDateTime(segment.start_time)} → ${formatDateTime(segment.end_time)}` : 'No segment saved yet'}
                badge={<StatusBadge kind={segment.status === 'completed' ? 'ok' : 'warning'} text={segment.status || 'unknown'} />}
              />
            ))}
          </div>
        ) : (
          <EmptyState title="No segments yet" body="Recorded segments will appear here once the recorder is running." />
        )}
      </section>

      <section className="health-section">
        <div className="health-section-heading">
          <div>
            <h3>Latest Events</h3>
            <p>Recent backend, recorder, cleanup, and live stream activity.</p>
          </div>
        </div>
        {data.events.length ? (
          <div className="health-event-list">
            {data.events.map((eventItem) => (
              <article className="health-event-row" key={eventItem.id || `${eventItem.created_at}-${eventItem.event_type}`}>
                <AlertSeverityBadge severity={eventItem.severity || 'info'} />
                <div>
                  <strong>{humanizeKey(eventItem.event_type)}</strong>
                  <p>{eventItem.message || humanizeKey(eventItem.entity_type || 'event')}</p>
                </div>
                <time>{formatRelativeTime(eventItem.created_at)}</time>
              </article>
            ))}
          </div>
        ) : (
          <EmptyState title="No events" body="System events such as logins, CRUD, and recorder activity will appear here." />
        )}
      </section>

      <section className="health-section health-build-section">
        <div className="health-section-heading">
          <div>
            <h3>Build &amp; Migrations</h3>
            <p>Version and database migration state.</p>
          </div>
        </div>
        {data.version ? (
          <div className="health-build-grid">
            <HealthMiniStat label="App version" value={data.version.app_version || 'dev'} />
            <HealthMiniStat label="Git commit" value={shortID(data.version.git_commit || '') || 'unknown'} />
            <HealthMiniStat label="Build time" value={data.version.build_time || 'not set'} />
            <HealthMiniStat label="Migrations" value={`${data.version.migrations_applied ?? 0}/${data.version.latest_migration ?? 0}`} />
          </div>
        ) : (
          !loading && <p className="muted">Build identity not available.</p>
        )}
      </section>
    </Panel>
  );
}

function HealthKPI({ title, value, detail, tone = 'muted' }) {
  return (
    <section className={`health-kpi health-kpi-${tone}`}>
      <span>{title}</span>
      <strong>{value}</strong>
      <small>{detail}</small>
    </section>
  );
}

function HealthListItem({ title, meta, detail, badge }) {
  return (
    <article className="health-list-item">
      <div>
        <strong>{title}</strong>
        <span>{meta}</span>
        <p>{detail}</p>
      </div>
      {badge}
    </article>
  );
}

function HealthStorageRow({ item }) {
  const insight = storageInsights(item);
  const status = insight.status;
  return (
    <article className={`health-storage-row health-storage-${status.kind}`}>
      <div className="health-storage-top">
        <div>
          <strong>{item.name}</strong>
          <span>{item.container_path}</span>
        </div>
        <StatusBadge kind={status.badge} text={status.label} />
      </div>
      <div className="health-usage-bar">
        <span style={{ width: `${insight.usedPercent}%` }} />
      </div>
      <div className="health-storage-meta">
        <span>{insight.usedPercent.toFixed(1)}% used</span>
        <span>{formatBytes(insight.free)} available</span>
        <span>{item.exists ? 'Exists' : 'Missing'}</span>
        <span>{item.writable ? 'Writable' : 'Not writable'}</span>
      </div>
      {insight.reasons.length > 0 && <p>{insight.reasons.join(' ')}</p>}
    </article>
  );
}

function HealthCameraCard({ camera }) {
  const enabled = Boolean(camera.enabled);
  const recording = Boolean(camera.recording_enabled);
  const streaming = camera.stream_enabled !== false;
  return (
    <article className="health-camera-card">
      <div>
        <strong>{camera.name}</strong>
        <span>{camera.location || camera.camera_group || shortID(camera.id)}</span>
      </div>
      <div className="health-camera-badges">
        <StatusBadge kind={enabled ? 'ok' : 'muted'} text={enabled ? 'enabled' : 'disabled'} />
        <StatusBadge kind={recording ? 'ok' : 'warning'} text={recording ? 'recording' : 'not recording'} />
        <StatusBadge kind={streaming ? 'ok' : 'muted'} text={streaming ? 'live enabled' : 'live off'} />
      </div>
      <small>{camera.record_audio || camera.stream_audio ? `Audio: ${camera.record_audio ? 'recording' : ''}${camera.record_audio && camera.stream_audio ? ' + ' : ''}${camera.stream_audio ? 'live' : ''}` : 'Audio off'}</small>
    </article>
  );
}

function HealthMiniStat({ label, value }) {
  return (
    <div className="health-mini-stat">
      <span>{label}</span>
      <strong>{value}</strong>
    </div>
  );
}

function StoragePage() {
  const [items, setItems] = useState([]);
  const [form, setForm] = useState(newStorageForm());
  const [editingId, setEditingId] = useState('');
  const [editForm, setEditForm] = useState(null);
  const [actionError, setActionError] = useState('');
  const { loading, error, run } = useLoader(load);

  async function load() {
    const data = await api('/api/storage-locations');
    setItems((data.storage_locations || []).map(normalizeStorageLocation));
  }
  useEffect(() => { run(); }, []);

  async function create(event) {
    event.preventDefault();
    await api('/api/storage-locations', { method: 'POST', body: JSON.stringify(storagePayload(form)) });
    setForm(newStorageForm());
    run();
  }

  function startEdit(item) {
    setEditingId(item.id);
    setEditForm(storageToForm(item));
    setActionError('');
  }

  function cancelEdit() {
    setEditingId('');
    setEditForm(null);
    setActionError('');
  }

  async function saveEdit(event) {
    event.preventDefault();
    if (!editingId || !editForm) return;
    setActionError('');
    try {
      await api(`/api/storage-locations/${editingId}`, { method: 'PATCH', body: JSON.stringify(storagePayload(editForm)) });
      cancelEdit();
      run();
    } catch (err) {
      setActionError(err.message);
    }
  }

  async function setStorageEnabled(item, enabled) {
    setActionError('');
    try {
      await api(`/api/storage-locations/${item.id}/enabled`, { method: 'PATCH', body: JSON.stringify({ enabled }) });
      run();
    } catch (err) {
      setActionError(err.message);
    }
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
        <StorageMetric label="Attention" value={summary.problemCount} detail={summary.problemCount ? 'locations with active warnings or errors' : 'all enabled storage ready'} tone={summary.problemCount ? 'warning' : 'ok'} />
      </div>

      {summary.attentionItems.length > 0 && (
        <section className="storage-attention-panel">
          <div className="section-heading">
            <h3>Attention details</h3>
            <p>These are the exact reasons included in the Attention count.</p>
          </div>
          <div className="storage-attention-list">
            {summary.attentionItems.map((entry) => (
              <article key={entry.id}>
                <StatusBadge kind={entry.status.badge} text={entry.status.label} />
                <strong>{entry.name}</strong>
                <span>{entry.reasons.join(' ')}</span>
              </article>
            ))}
          </div>
        </section>
      )}

      <section className="storage-create-panel">
        <div className="section-heading">
          <h3>Add storage location</h3>
          <p>Use the container path mounted into backend and recorder, for example <code>/recordings</code>.</p>
        </div>
        <form className="storage-form storage-create-form" onSubmit={create}>
          <StorageFields form={form} onChange={(patch) => setForm({ ...form, ...patch })} />
          <div className="form-actions">
            <button>Create storage</button>
          </div>
        </form>
      </section>
      <State loading={loading} error={error} />
      {actionError && <div className="error">{actionError}</div>}
      {items.length ? (
        <div className="storage-card-grid">
          {items.map((item) => (
            <StorageLocationCard
              editing={editingId === item.id}
              editForm={editForm}
              item={item}
              key={item.id}
              onCancelEdit={cancelEdit}
              onEdit={() => startEdit(item)}
              onEditChange={(patch) => setEditForm({ ...editForm, ...patch })}
              onSaveEdit={saveEdit}
              onSetEnabled={(enabled) => setStorageEnabled(item, enabled)}
            />
          ))}
        </div>
      ) : (
        !loading && <EmptyState title="No storage locations" body="Create a storage location to enable recording." />
      )}
    </Panel>
  );
}

function newStorageForm() {
  return { name: '', container_path: '/recordings', enabled: true, max_storage_size: '', max_storage_unit: 'GB' };
}

function normalizeStorageLocation(item) {
  return { ...item, is_enabled: item.is_enabled ?? item.enabled };
}

function storagePayload(values) {
  return {
    name: values.name.trim(),
    container_path: values.container_path.trim(),
    enabled: values.enabled,
    max_storage_bytes: storageSizeToBytes(values.max_storage_size, values.max_storage_unit),
  };
}

function storageSizeToBytes(value, unit) {
  const size = Number(value);
  if (!Number.isFinite(size) || size <= 0) return null;
  const multipliers = { MB: 1024 ** 2, GB: 1024 ** 3, TB: 1024 ** 4 };
  return Math.round(size * (multipliers[unit] || multipliers.GB));
}

function bytesToStorageSize(bytes) {
  const value = Number(bytes || 0);
  if (!value) return { value: '', unit: 'GB' };
  const units = [
    ['TB', 1024 ** 4],
    ['GB', 1024 ** 3],
    ['MB', 1024 ** 2],
  ];
  const [unit, divisor] = units.find(([, amount]) => value >= amount) || units[1];
  const display = value / divisor;
  return { value: Number(display.toFixed(display >= 100 ? 0 : 2)).toString(), unit };
}

function storageToForm(item) {
  const size = bytesToStorageSize(item.max_storage_bytes);
  return {
    name: item.name || '',
    container_path: item.container_path || '/recordings',
    enabled: Boolean(item.enabled ?? item.is_enabled),
    max_storage_size: size.value,
    max_storage_unit: size.unit,
  };
}

function StorageFields({ form, onChange }) {
  return (
    <>
      <Field className="storage-field-name" label="Display name" help="Shown in camera setup and health dashboards.">
        <input placeholder="Primary recordings" value={form.name} onChange={(e) => onChange({ name: e.target.value })} required />
      </Field>
      <Field className="storage-field-path" label="Container path" help="Backend validates that this folder exists and is writable inside the container.">
        <input placeholder="/recordings" value={form.container_path} onChange={(e) => onChange({ container_path: e.target.value })} required />
      </Field>
      <div className="storage-size-fields">
        <Field className="storage-field-limit" label="Configured limit" help="Optional operator limit used for capacity display.">
          <input type="number" min="1" step="0.1" placeholder="No limit" value={form.max_storage_size} onChange={(e) => onChange({ max_storage_size: e.target.value })} />
        </Field>
        <Field className="storage-field-unit" label="Unit" help="MB, GB, or TB.">
          <select value={form.max_storage_unit} onChange={(e) => onChange({ max_storage_unit: e.target.value })}>
            <option value="MB">MB</option>
            <option value="GB">GB</option>
            <option value="TB">TB</option>
          </select>
        </Field>
      </div>
      <label className="switch-row storage-enable-row">
        <input type="checkbox" checked={form.enabled} onChange={(e) => onChange({ enabled: e.target.checked })} />
        <span><strong>Enable storage</strong><small>Allow cameras to use this location after validation passes.</small></span>
      </label>
    </>
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

function StorageLocationCard({ item, editing, editForm, onCancelEdit, onEdit, onEditChange, onSaveEdit, onSetEnabled }) {
  const insight = storageInsights(item);
  const status = insight.status;
  const path = item.container_path || 'No path configured';
  return (
    <section className={`storage-card storage-card-${status.kind}`}>
      {editing ? (
        <form className="storage-form storage-edit-form" onSubmit={onSaveEdit}>
          <StorageFields form={editForm} onChange={onEditChange} />
          <div className="form-actions">
            <button>Save storage</button>
            <button type="button" onClick={onCancelEdit}>Cancel</button>
          </div>
        </form>
      ) : (
        <>
      <div className="storage-card-header">
        <div>
          <h3>{item.name}</h3>
          <p>{path}</p>
        </div>
        <div className="storage-card-actions">
          <StatusBadge kind={status.badge} text={status.label} />
          <button type="button" onClick={onEdit}>Edit</button>
          <button type="button" onClick={() => onSetEnabled(!item.is_enabled)}>{item.is_enabled ? 'Disable' : 'Enable'}</button>
        </div>
      </div>
      <div className="storage-usage-row">
        <div className="storage-donut" style={{ '--used': `${insight.usedPercent}%` }}>
          <span>{insight.usedPercent.toFixed(0)}%</span>
        </div>
        <div className="storage-usage-main">
          <div className="storage-usage-label">
            <strong>{formatBytes(insight.used)} used</strong>
            <span>{formatBytes(insight.free)} available</span>
          </div>
          <div className="storage-usage-bar" aria-label={`${insight.usedPercent.toFixed(1)} percent used`}>
            <span style={{ width: `${insight.usedPercent}%` }} />
          </div>
          <small>{insight.configuredLimit ? `${formatBytes(insight.configuredLimit)} configured limit, ${formatBytes(insight.detectedTotal)} detected on disk` : (insight.detectedTotal ? `${formatBytes(insight.detectedTotal)} detected disk capacity` : 'Capacity unavailable')}</small>
        </div>
      </div>
      <dl className="storage-health-grid">
        <div><dt>Enabled</dt><dd>{item.is_enabled ? 'Yes' : 'No'}</dd></div>
        <div><dt>Status</dt><dd>{status.label}</dd></div>
        <div><dt>Exists</dt><dd>{item.exists ? 'Yes' : 'No'}</dd></div>
        <div><dt>Writable</dt><dd>{item.writable ? 'Yes' : 'No'}</dd></div>
        <div><dt>Limit</dt><dd>{insight.configuredLimit ? formatBytes(insight.configuredLimit) : 'None'}</dd></div>
      </dl>
      <div className={`storage-reason-list storage-reason-${status.kind}`}>
        {insight.reasons.map((reason) => <span key={reason}>{reason}</span>)}
      </div>
        </>
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
    const storageLocations = storageData.storage_locations || [];
    setCameras(nextCameras);
    setStorage(storageLocations);
    setForm((current) => {
      if (current.storage_location_id || current.name || current.rtsp_url) return current;
      const storageID = firstEnabledStorageID(storageLocations);
      if (!storageID) return current;
      return { ...current, storage_location_id: storageID, recording_enabled: true };
    });
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
      motion_detection_enabled: values.motion_detection_enabled,
      motion_sensitivity: Number(values.motion_sensitivity),
      motion_min_duration_seconds: Number(values.motion_min_duration_seconds),
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
    return onvifForms[deviceKey(device)] || newONVIFImportForm(device, storage);
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
      motion_detection_enabled: values.motion_detection_enabled,
      motion_sensitivity: Number(values.motion_sensitivity),
      motion_min_duration_seconds: Number(values.motion_min_duration_seconds),
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
                    <StatusBadge kind={camera.motion_detection_enabled ? 'ok' : 'muted'} text={camera.motion_detection_enabled ? 'motion on' : 'motion off'} />
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
                      <div><dt>Motion detector</dt><dd>{camera.motion_detection_enabled ? `On · ${Number(camera.motion_sensitivity || 0.35).toFixed(2)} sensitivity` : 'Off'}</dd></div>
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
                      <button
                        type="button"
                        onClick={() => quickPatch(camera, { motion_detection_enabled: !camera.motion_detection_enabled })}
                      >
                        {camera.motion_detection_enabled ? 'Disable motion' : 'Enable motion'}
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
    recording_enabled: true,
    record_audio: false,
    stream_enabled: true,
    stream_audio: false,
    motion_detection_enabled: false,
    motion_sensitivity: 0.35,
    motion_min_duration_seconds: 1,
    retention_days: 30,
    max_storage_bytes: '',
  };
}

function firstEnabledStorageID(storageLocations = []) {
  return storageLocations.find((item) => item.enabled !== false && item.is_enabled !== false)?.id || '';
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

function newONVIFImportForm(device, storageLocations = []) {
  const storageID = firstEnabledStorageID(storageLocations);
  return {
    name: [device.manufacturer, device.model].filter(Boolean).join(' ') || `Camera ${device.ip}`,
    username: '',
    password: '',
    storage_location_id: storageID,
    retention_days: 30,
    max_storage_bytes: '',
    enabled: true,
    recording_enabled: true,
    record_audio: false,
    stream_enabled: true,
    stream_audio: false,
    motion_detection_enabled: false,
    motion_sensitivity: 0.35,
    motion_min_duration_seconds: 1,
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
      <DetectorSettingsPanel form={form} onChange={onChange} compact />
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
    motion_detection_enabled: Boolean(camera.motion_detection_enabled),
    motion_sensitivity: camera.motion_sensitivity || 0.35,
    motion_min_duration_seconds: camera.motion_min_duration_seconds ?? 1,
    retention_days: camera.retention_days || 30,
    max_storage_bytes: camera.max_storage_bytes || '',
  };
}

function DetectorSettingsPanel({ form, onChange, compact = false }) {
  const enabled = Boolean(form.motion_detection_enabled);
  const sensitivity = Number(form.motion_sensitivity || 0.35);
  const minSeconds = Number(form.motion_min_duration_seconds ?? 1);
  return (
    <div className={compact ? 'detector-settings compact' : 'detector-settings'}>
      <div className={enabled ? 'detector-card active' : 'detector-card'}>
        <div className="detector-card-header">
          <div>
            <strong>Motion detector</strong>
            <span>{enabled ? 'Live detector active for this camera' : 'Live detector disabled for this camera'}</span>
          </div>
          <StatusBadge kind={enabled ? 'ok' : 'muted'} text={enabled ? 'on' : 'off'} />
        </div>
        <label className="detector-toggle">
          <input
            type="checkbox"
            checked={enabled}
            onChange={(event) => onChange({ motion_detection_enabled: event.target.checked })}
          />
          <span>
            <strong>Enable motion alerts</strong>
            <small>Runs a low-FPS live detector and uses notification rules when motion is found.</small>
          </span>
        </label>
        <div className="detector-input-grid">
          <Field label="Sensitivity" help="Lower values catch smaller changes.">
            <div className="detector-range-row">
              <input
                type="range"
                min="0.01"
                max="1"
                step="0.01"
                value={sensitivity}
                onChange={(event) => onChange({ motion_sensitivity: event.target.value })}
              />
              <input
                className="detector-number"
                type="number"
                min="0.01"
                max="1"
                step="0.01"
                value={form.motion_sensitivity}
                onChange={(event) => onChange({ motion_sensitivity: event.target.value })}
              />
            </div>
          </Field>
          <Field label="Minimum duration" help="Ignore brief changes below this length.">
            <div className="detector-duration-row">
              <input
                type="number"
                min="0"
                value={form.motion_min_duration_seconds}
                onChange={(event) => onChange({ motion_min_duration_seconds: event.target.value })}
              />
              <span>{minSeconds === 1 ? 'second' : 'seconds'}</span>
            </div>
          </Field>
        </div>
      </div>
    </div>
  );
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

      <div className="camera-form-section camera-form-section-detectors">
        <div className="camera-form-section-heading">
          <strong>Detection</strong>
          <span>Manage per-camera detectors that can trigger notifications.</span>
        </div>
        <DetectorSettingsPanel form={form} onChange={(patch) => setForm({ ...form, ...patch })} />
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
  const [selectedUserID, setSelectedUserID] = useState('');
  const [permissions, setPermissions] = useState([]);
  const [permissionError, setPermissionError] = useState('');
  const [permissionLoading, setPermissionLoading] = useState(false);
  const [savingCameraID, setSavingCameraID] = useState('');
  const { loading, error, run } = useLoader(load);

  async function load() {
    const [userData, cameraData] = await Promise.all([api('/api/users'), api('/api/cameras')]);
    const loadedUsers = userData.users || [];
    setUsers(loadedUsers);
    setCameras(cameraData.cameras || []);
    setSelectedUserID((current) => current || loadedUsers.find((user) => user.role !== 'admin')?.id || loadedUsers[0]?.id || '');
  }
  useEffect(() => { run(); }, []);

  async function loadPermissions(userID) {
    if (!userID) {
      setPermissions([]);
      return;
    }
    setPermissionLoading(true);
    setPermissionError('');
    setPermissions([]);
    try {
      const data = await api(`/api/users/${userID}/camera-permissions`);
      setPermissions(data.camera_permissions || []);
    } catch (err) {
      setPermissionError(err.message);
    } finally {
      setPermissionLoading(false);
    }
  }

  useEffect(() => { loadPermissions(selectedUserID); }, [selectedUserID]);

  const selectedUser = users.find((user) => user.id === selectedUserID);
  const selectedUserIsAdmin = selectedUser?.role === 'admin';
  const permissionMap = new Map(permissions.map((permission) => [permission.camera_id, permission]));
  const summary = cameras.reduce((acc, camera) => {
    if (selectedUserIsAdmin) {
      acc.live += 1;
      acc.playback += 1;
      acc.any += 1;
      return acc;
    }
    const permission = permissionMap.get(camera.id);
    if (permission?.can_view_live) acc.live += 1;
    if (permission?.can_view_playback) acc.playback += 1;
    if (permission?.can_view_live || permission?.can_view_playback) acc.any += 1;
    return acc;
  }, { any: 0, live: 0, playback: 0 });

  async function savePermission(cameraID, nextPermission) {
    if (!selectedUserID || !cameraID) return;
    setSavingCameraID(cameraID);
    setPermissionError('');
    try {
      if (!nextPermission.can_view_live && !nextPermission.can_view_playback) {
        await api(`/api/users/${selectedUserID}/camera-permissions/${cameraID}`, { method: 'DELETE' });
      } else {
        await api(`/api/users/${selectedUserID}/camera-permissions/${cameraID}`, { method: 'PUT', body: JSON.stringify(nextPermission) });
      }
      await loadPermissions(selectedUserID);
    } catch (err) {
      setPermissionError(err.message);
    } finally {
      setSavingCameraID('');
    }
  }

  function togglePermission(cameraID, key) {
    const current = permissionMap.get(cameraID) || { can_view_live: false, can_view_playback: false };
    savePermission(cameraID, {
      can_view_live: key === 'can_view_live' ? !current.can_view_live : current.can_view_live,
      can_view_playback: key === 'can_view_playback' ? !current.can_view_playback : current.can_view_playback,
    });
  }

  function grantBoth(cameraID) {
    savePermission(cameraID, { can_view_live: true, can_view_playback: true });
  }

  function revokeAll(cameraID) {
    savePermission(cameraID, { can_view_live: false, can_view_playback: false });
  }

  return (
    <Panel title="Camera Permissions">
      <State loading={loading} error={error} />
      {permissionError && <div className="error">{permissionError}</div>}
      {!users.length && !loading && <EmptyState title="No users" body="Create a user before granting permissions." />}
      {!cameras.length && !loading && <EmptyState title="No cameras" body="Add a camera before granting permissions." />}
      {users.length > 0 && cameras.length > 0 && (
        <div className="permissions-layout">
          <aside className="permissions-users">
            <div className="section-heading">
              <h3>Users</h3>
              <p>Select a user to review their camera access.</p>
            </div>
            <div className="permissions-user-list">
              {users.map((user) => (
                <button
                  className={'permissions-user-card' + (user.id === selectedUserID ? ' selected' : '')}
                  key={user.id}
                  type="button"
                  onClick={() => setSelectedUserID(user.id)}
                >
                  <strong>{user.display_name || user.email}</strong>
                  <span>{user.email}</span>
                  <small>
                    <StatusBadge kind={user.role === 'admin' ? 'ok' : 'muted'} text={user.role} />
                    <StatusBadge kind={user.active === false || user.is_active === false ? 'muted' : 'ok'} text={user.active === false || user.is_active === false ? 'inactive' : 'active'} />
                  </small>
                </button>
              ))}
            </div>
          </aside>

          <section className="permissions-main">
            <div className="permissions-hero">
              <div>
                <span>Selected user</span>
                <h3>{selectedUser?.display_name || selectedUser?.email || 'No user selected'}</h3>
                <p>{selectedUserIsAdmin ? 'Admins have inherited access to every camera. Explicit camera grants are only needed for normal users.' : 'Grant only the live and playback access this person should use.'}</p>
              </div>
              <div className="permissions-summary-grid">
                <PermissionMetric label="Any access" value={`${summary.any}/${cameras.length}`} />
                <PermissionMetric label="Live" value={`${summary.live}/${cameras.length}`} />
                <PermissionMetric label="Playback" value={`${summary.playback}/${cameras.length}`} />
              </div>
            </div>

            {permissionLoading ? <p className="muted">Loading permissions...</p> : (
              <div className="permissions-camera-grid">
                {cameras.map((camera) => {
                  const explicitPermission = permissionMap.get(camera.id) || { can_view_live: false, can_view_playback: false };
                  const permission = selectedUserIsAdmin ? { can_view_live: true, can_view_playback: true } : explicitPermission;
                  const saving = savingCameraID === camera.id;
                  const hasAny = permission.can_view_live || permission.can_view_playback;
                  return (
                    <article className={'permissions-camera-card' + (hasAny ? ' granted' : '') + (selectedUserIsAdmin ? ' inherited' : '')} key={camera.id}>
                      <div className="permissions-camera-heading">
                        <div>
                          <strong>{camera.name}</strong>
                          <span>{camera.location || camera.camera_group || shortID(camera.id)}</span>
                        </div>
                        <StatusBadge kind={camera.enabled ? 'ok' : 'muted'} text={camera.enabled ? 'enabled' : 'disabled'} />
                      </div>
                      <div className="permissions-toggle-row">
                        <PermissionToggle
                          active={permission.can_view_live}
                          disabled={saving || selectedUserIsAdmin}
                          label="Live"
                          statusText={selectedUserIsAdmin ? 'Admin access' : undefined}
                          onClick={() => togglePermission(camera.id, 'can_view_live')}
                        />
                        <PermissionToggle
                          active={permission.can_view_playback}
                          disabled={saving || selectedUserIsAdmin}
                          label="Playback"
                          statusText={selectedUserIsAdmin ? 'Admin access' : undefined}
                          onClick={() => togglePermission(camera.id, 'can_view_playback')}
                        />
                      </div>
                      <div className="permissions-card-actions">
                        {selectedUserIsAdmin ? (
                          <span>Role-based access. No camera grant required.</span>
                        ) : (
                          <>
                            <button type="button" disabled={saving || (permission.can_view_live && permission.can_view_playback)} onClick={() => grantBoth(camera.id)}>Grant both</button>
                            <button type="button" disabled={saving || !hasAny} onClick={() => revokeAll(camera.id)}>Revoke</button>
                          </>
                        )}
                      </div>
                    </article>
                  );
                })}
              </div>
            )}
          </section>
        </div>
      )}
    </Panel>
  );
}

function PermissionMetric({ label, value }) {
  return (
    <div className="permission-metric">
      <span>{label}</span>
      <strong>{value}</strong>
    </div>
  );
}

function PermissionToggle({ active, disabled, label, onClick, statusText }) {
  return (
    <button className={'permission-toggle' + (active ? ' active' : '')} type="button" disabled={disabled} onClick={onClick}>
      <span>{label}</span>
      <strong>{statusText || (active ? 'Allowed' : 'Blocked')}</strong>
    </button>
  );
}

function LayoutCanvas({ layout, cameras, onChange, onError, editable = true }) {
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

  function gridMetrics() {
    const el = gridRef.current;
    if (!el) return null;
    const r = el.getBoundingClientRect();
    const style = window.getComputedStyle(el);
    const paddingLeft = Number.parseFloat(style.paddingLeft) || 0;
    const paddingRight = Number.parseFloat(style.paddingRight) || 0;
    const paddingTop = Number.parseFloat(style.paddingTop) || 0;
    const paddingBottom = Number.parseFloat(style.paddingBottom) || 0;
    const columnGap = Number.parseFloat(style.columnGap) || 0;
    const rowGap = Number.parseFloat(style.rowGap) || 0;
    const contentWidth = Math.max(1, r.width - paddingLeft - paddingRight - columnGap * Math.max(0, cols - 1));
    const contentHeight = Math.max(1, r.height - paddingTop - paddingBottom - rowGap * Math.max(0, rows - 1));
    return {
      left: r.left + paddingLeft,
      top: r.top + paddingTop,
      cellWidth: contentWidth / cols,
      cellHeight: contentHeight / rows,
    };
  }

  function pixelToCell(px, py) {
    const metrics = gridMetrics();
    if (!metrics) return { x: 0, y: 0 };
    let cx = Math.floor((px - metrics.left) / metrics.cellWidth);
    let cy = Math.floor((py - metrics.top) / metrics.cellHeight);
    cx = Math.max(0, Math.min(cols - 1, cx));
    cy = Math.max(0, Math.min(rows - 1, cy));
    return { x: cx, y: cy };
  }

  function startDrag(item, mode, event) {
    if (!editable) return;
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
    const metrics = gridMetrics();
    if (!metrics) return items;
    const dxc = Math.round((event.clientX - drag.startMouseX) / metrics.cellWidth);
    const dyc = Math.round((event.clientY - drag.startMouseY) / metrics.cellHeight);
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
    if (!editable || !drag) return;
    if (!drag.moved && dragHasMoved(drag, event)) {
      setDrag({ ...drag, moved: true });
    }
    onChange(updatedItemsForDrag(event), { refetch: false });
  }

  function onGridMouseUp(event) {
    if (!editable || !drag) return;
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
    if (!editable) return;
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
        className={`live-layout-grid layout-editor-grid${editable ? ' editable' : ''}`}
        style={{
          '--layout-columns': cols,
          gridTemplateColumns: `repeat(${cols}, minmax(0, 1fr))`,
          gridTemplateRows: `repeat(${rows}, var(--layout-row-height))`,
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
              onClick={(e) => { e.stopPropagation(); if (editable) setSelectedId(itemID(it)); }}
            >
              <div className="live-tile-bar" onMouseDown={(e) => startDrag(it, 'move', e)}>
                <strong>{cam ? cam.name : shortID(it.camera_id)}</strong>
                <span className="muted">{it.width || 1}×{it.height || 1}</span>
              </div>
              <div className="live-tile-video layout-editor-tile-body">
                <p>{cam ? (cam.location || cam.camera_group || shortID(cam.id)) : 'Camera unavailable'}</p>
              </div>
              {editable && selectedId === itemID(it) && (
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
      {editable && selected && (
        <div className="layout-tile-edit">
          <span>
            <strong>{tileCamera ? tileCamera.name : shortID(selected.camera_id)}</strong>
            <span className="muted"> · {selected.width}×{selected.height} @ ({selected.x},{selected.y})</span>
          </span>
          <button type="button" onClick={() => setSelectedId(null)}>Close</button>
          <button type="button" className="danger" onClick={() => { if (window.confirm('Delete this tile?')) deleteItem(selected.id); }}>Delete</button>
        </div>
      )}
      {editable && pendingCell && (
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
  const isMobileLayout = useIsMobileLayout();
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
  const canEditLayout = user.role === 'admin' && !isMobileLayout;

  return (
    <Panel title="Layouts">
      {errorMsg && <ErrorText message={errorMsg} />}
      {canEditLayout && (
        <form className="layout-create-form" onSubmit={createLayout}>
          <div className="layout-form-heading">
            <strong>Create Layout</strong>
            <span>Set up a camera grid for Live and Playback review.</span>
          </div>
          <Field label="Layout name" help="Use a short name that operators can recognize quickly.">
            <input placeholder="Main entrance" value={form.name} onChange={(e) => setForm({ ...form, name: e.target.value })} required />
          </Field>
          <Field label="Grid columns" help="More columns allow finer tile placement on desktop screens.">
            <input type="number" min="1" max="32" value={form.columns} onChange={(e) => setForm({ ...form, columns: e.target.value })} />
          </Field>
          <button>Create layout</button>
        </form>
      )}
      <State loading={loading} error={error} />
      {!layouts.length && !loading ? (
        <EmptyState title="No layouts" body={user.role === 'admin' ? 'Create a layout above to start adding camera tiles.' : 'No layouts have been shared with you yet.'} />
      ) : (
        <>
          <div className="layout-toolbar">
            <Field label="Active layout" className="layout-active-field">
              <select value={activeId || ''} onChange={(e) => setActiveId(e.target.value)}>
                <option value="" disabled>Pick a layout...</option>
                {layouts.map((l) => <option key={l.id} value={l.id}>{l.name}{l.is_default ? ' (default)' : ''}</option>)}
              </select>
            </Field>
            {layout && canEditLayout && (
              <div className="layout-toolbar-actions">
                <Field label="Columns" className="layout-columns-field">
                  <input
                    type="number" min="1" max="32"
                    defaultValue={layout.settings?.columns || 4}
                    onBlur={(e) => {
                      const v = Number(e.target.value);
                      if (v && v !== (layout.settings?.columns || 4)) patchColumns(layout.id, v);
                    }}
                  />
                </Field>
                {!layout.is_default && <button type="button" onClick={() => setDefault(layout.id)}>Make default</button>}
                <button type="button" className="danger" onClick={() => deleteLayout(layout.id)}>Delete layout</button>
              </div>
            )}
            {layout && (
              <span className="layout-tile-count">
                {(layout.layout_items || []).length} tile{(layout.layout_items || []).length === 1 ? '' : 's'}
              </span>
            )}
          </div>
          {layout && (
            <>
              <p className="muted layout-help">
                {canEditLayout
                  ? 'Click an empty cell to add a camera. Drag a tile to move it. Drag a corner or edge handle (visible when the tile is selected) to resize. Edits save automatically.'
                  : 'Mobile view is read-only. Use a desktop screen to edit layout tiles.'}
              </p>
              <LayoutCanvas
                layout={layout}
                cameras={cameras}
                onChange={onItemsChanged}
                onError={setErrorMsg}
                editable={canEditLayout}
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
  const isMobileLayout = useIsMobileLayout();
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
    if (!layoutId || !result) return undefined;
    const delay = result?.cameras?.some((camera) => camera.status === 'starting' || camera.active_event) ? 1500 : 3000;
    const timer = window.setTimeout(() => {
      loadLive(layoutId, { silent: true });
    }, delay);
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
        <div className={user?.role === 'admin' && !isMobileLayout ? 'live-layout-wrap' : undefined}>
          <LiveLayoutGrid
            layout={layout}
            cameras={result?.cameras || []}
            editable={user?.role === 'admin' && !isMobileLayout}
            onError={setActionError}
            onChange={(updatedItems) => {
              setLayouts((prev) => prev.map((item) => (
                item.id === layout.id ? { ...item, layout_items: updatedItems } : item
              )));
            }}
          />
        </div>
      )}
    </Panel>
  );
}

function PlaybackPage() {
  const isMobileLayout = useIsMobileLayout();
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
          simpleList={isMobileLayout}
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
  const [dayStart, dayEnd] = dayBounds(selectedTime);
  const [visibleWindow, setVisibleWindow] = useState(() => timelineWindowForSelectedTime(selectedTime));
  const start = visibleWindow.start;
  const end = visibleWindow.end;
  const duration = end.getTime() - start.getTime();
  const playheadLeft = pct((selectedTime.getTime() - start.getTime()) / duration);
  const availability = new Map((timeline?.camera_availability || []).map((item) => [item.camera_id, item]));
  const cameraNames = new Map(cameras.map((camera) => [camera.camera_id, camera.camera_name || shortID(camera.camera_id)]));
  const visibleCameraIDs = resultLoaded ? new Set(cameras.map((camera) => camera.camera_id)) : null;
  const items = (layout?.layout_items || []).filter((item) => item.camera_id && (!visibleCameraIDs || visibleCameraIDs.has(item.camera_id)));
  const hasRanges = (timeline?.camera_availability || []).some((item) => (item.ranges || []).length > 0);
  const scaleLabels = timelineScaleLabels(start, end);

  useEffect(() => {
    setVisibleWindow((current) => {
      const [currentDayStart, currentDayEnd] = dayBounds(current.start);
      if (selectedTime >= currentDayStart && selectedTime < currentDayEnd) return current;
      return timelineWindowForSelectedTime(selectedTime);
    });
  }, [selectedTime]);

  function selectFromEvent(event) {
    const rect = event.currentTarget.getBoundingClientRect();
    const ratio = Math.min(Math.max((event.clientX - rect.left) / rect.width, 0), 1);
    onSelect(new Date(start.getTime() + ratio * duration));
  }

  function zoomFromWheel(event) {
    event.preventDefault();
    const rect = event.currentTarget.getBoundingClientRect();
    const anchorRatio = Math.min(Math.max((event.clientX - rect.left) / rect.width, 0), 1);
    const anchorTime = start.getTime() + anchorRatio * duration;
    const zoomFactor = event.deltaY < 0 ? 0.72 : 1.35;
    setVisibleWindow((current) => zoomTimelineWindow(current, dayStart, dayEnd, anchorTime, anchorRatio, zoomFactor));
  }

  function resetZoom() {
    setVisibleWindow({ start: dayStart, end: dayEnd });
  }

  return (
    <section className="timeline-panel">
      <div className="timeline-header">
        <div>
          <strong>Recording timeline</strong>
          <span>{formatDateTime(selectedTime)} · visible {formatTime(start)} - {formatTime(end)} ({formatTimelineDuration(duration)})</span>
        </div>
        <div className="timeline-actions">
          <span>{hasRanges ? 'Recorded ranges are highlighted' : 'No stored video on this day'}</span>
          <button type="button" onClick={resetZoom}>Reset zoom</button>
        </div>
      </div>
      <div className="timeline-scale">
        {scaleLabels.map((label) => <span key={label.value}>{label.text}</span>)}
      </div>
      <div className="timeline-stack">
        {items.map((item) => {
          const cameraID = item.camera_id;
          const row = availability.get(cameraID) || { ranges: [] };
          return (
            <div className="timeline-row" key={item.id || item.item_id || cameraID}>
              <span title={cameraNames.get(cameraID) || cameraID}>{cameraNames.get(cameraID) || shortID(cameraID)}</span>
              <button
                className="timeline-track"
                type="button"
                onClick={selectFromEvent}
                onWheel={zoomFromWheel}
                aria-label={`Select playback time for ${cameraNames.get(cameraID) || cameraID}`}
                title="Click to select time. Use mouse wheel over the bar to zoom around the pointer."
              >
                {(row.ranges || []).map((range, index) => (
                  <span
                    className="timeline-range"
                    key={`${range.start_time}-${index}`}
                    style={rangeStyle(range.start_time, range.end_time, start, end)}
                  />
                ))}
                {(row.events || []).map((event, index) => (
                  <span
                    className="timeline-event timeline-event-motion"
                    key={`${event.id || event.start_time}-${index}`}
                    style={rangeStyle(event.start_time, event.end_time, start, end)}
                    title={`Motion event · ${formatDateTime(event.start_time)} · score ${Number(event.score || 0).toFixed(2)}`}
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

function PlaybackLayoutGrid({ layout, cameras, playing, videoRefs, focusedCameraId, onFocusCamera, simpleList = false, selectedTime, timeline, onSelectTime, onTogglePlayback }) {
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
  const focusedItem = simpleList ? null : items.find((item) => item.camera_id === focusedCameraId);
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
            {!simpleList && !options.compact && (
              <button type="button" onClick={() => (isFocused ? onFocusCamera('') : onFocusCamera(camera.camera_id))}>
                {isFocused ? 'Exit focus' : 'Focus'}
              </button>
            )}
            {!simpleList && options.compact && <button type="button" onClick={() => onFocusCamera(camera.camera_id)}>Focus</button>}
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
    <div
      className={'layout-playback-grid' + (simpleList ? ' simple-list' : '')}
      style={simpleList ? undefined : { gridTemplateColumns: `repeat(${columns}, minmax(0, 1fr))`, gridAutoRows: 'minmax(180px, auto)' }}
    >
      {items.map((item) => renderTile(item, {
        style: simpleList ? undefined : {
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
          {(availability?.events || []).map((event, index) => (
            <span
              className="timeline-event timeline-event-motion"
              key={`${event.id || event.start_time}-${index}`}
              style={rangeStyle(event.start_time, event.end_time, start, end)}
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

function timelineWindowForSelectedTime(date) {
  const [start, end] = dayBounds(date);
  return { start, end };
}

function zoomTimelineWindow(current, dayStart, dayEnd, anchorTime, anchorRatio, zoomFactor) {
  const dayMin = dayStart.getTime();
  const dayMax = dayEnd.getTime();
  const dayDuration = dayMax - dayMin;
  const currentDuration = current.end.getTime() - current.start.getTime();
  const minDuration = 60 * 1000;
  const nextDuration = Math.min(dayDuration, Math.max(minDuration, currentDuration * zoomFactor));
  let nextStart = anchorTime - anchorRatio * nextDuration;
  let nextEnd = nextStart + nextDuration;

  if (nextStart < dayMin) {
    nextStart = dayMin;
    nextEnd = nextStart + nextDuration;
  }
  if (nextEnd > dayMax) {
    nextEnd = dayMax;
    nextStart = nextEnd - nextDuration;
  }

  return { start: new Date(nextStart), end: new Date(nextEnd) };
}

function timelineScaleLabels(start, end) {
  const count = 7;
  const min = start.getTime();
  const max = end.getTime();
  const step = (max - min) / (count - 1);
  return Array.from({ length: count }, (_, index) => {
    const value = new Date(min + step * index);
    return { value: value.toISOString(), text: formatTime(value) };
  });
}

function formatTime(value) {
  return new Date(value).toLocaleTimeString([], { hour: '2-digit', minute: '2-digit' });
}

function formatTimelineDuration(ms) {
  const minutes = Math.max(1, Math.round(ms / 60000));
  if (minutes < 60) return `${minutes} min`;
  const hours = minutes / 60;
  if (hours < 24) return `${Number(hours.toFixed(hours >= 10 ? 0 : 1))} hr`;
  return '24 hr';
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
  if (end < min || start > max) return { display: 'none' };
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

  function gridMetrics() {
    const el = gridRef.current;
    if (!el) return null;
    const r = el.getBoundingClientRect();
    const style = window.getComputedStyle(el);
    const paddingLeft = Number.parseFloat(style.paddingLeft) || 0;
    const paddingRight = Number.parseFloat(style.paddingRight) || 0;
    const paddingTop = Number.parseFloat(style.paddingTop) || 0;
    const paddingBottom = Number.parseFloat(style.paddingBottom) || 0;
    const columnGap = Number.parseFloat(style.columnGap) || 0;
    const rowGap = Number.parseFloat(style.rowGap) || 0;
    const contentWidth = Math.max(1, r.width - paddingLeft - paddingRight - columnGap * Math.max(0, cols - 1));
    const contentHeight = Math.max(1, r.height - paddingTop - paddingBottom - rowGap * Math.max(0, rows - 1));
    return {
      cellWidth: contentWidth / cols,
      cellHeight: contentHeight / rows,
    };
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
    const metrics = gridMetrics();
    if (!metrics) return items;
    const dxc = Math.round((event.clientX - drag.startMouseX) / metrics.cellWidth);
    const dyc = Math.round((event.clientY - drag.startMouseY) / metrics.cellHeight);
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
      style={{
        '--layout-columns': cols,
        gridTemplateColumns: `repeat(${cols}, minmax(0, 1fr))`,
        gridTemplateRows: `repeat(${rows}, var(--layout-row-height))`,
      }}
      onMouseMove={onMouseMove}
      onMouseUp={onMouseUp}
      onMouseLeave={onMouseUp}
    >
      {items.map((item) => {
        const id = itemID(item);
        const camera = byCamera.get(item.camera_id) || { camera_id: item.camera_id, status: 'stream_unavailable' };
        const hasActiveEvent = Boolean(camera.active_event);
        return (
          <section
            key={id}
            className={'video-tile live-layout-tile' + (selectedId === id ? ' selected' : '') + (hasActiveEvent ? ' event-active' : '')}
            style={{
              gridColumn: `${Number(item.x || 0) + 1} / span ${Math.max(1, Number(item.width || 1))}`,
              gridRow: `${Number(item.y || 0) + 1} / span ${Math.max(1, Number(item.height || 1))}`,
            }}
            onClick={() => editable && setSelectedId(id)}
          >
            <div className="live-tile-bar" onMouseDown={(event) => startDrag(item, 'move', event)}>
              <strong>{camera.camera_name || shortID(camera.camera_id)}</strong>
              <span>
                {hasActiveEvent && <StatusBadge kind="warning" text="motion" />}
                <LiveStatusBadge status={camera.status} />
              </span>
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
    const insight = storageInsights(item);
    const status = insight.status;
    acc.configured += 1;
    acc.used += insight.used;
    acc.free += insight.free;
    acc.total += insight.total;
    if (item.is_enabled) acc.enabled += 1;
    if (!insight.attention) acc.ready += 1;
    if (insight.attention) {
      acc.problemCount += 1;
      acc.attentionItems.push({ id: item.id, name: item.name, reasons: insight.reasons, status });
    }
    return acc;
  }, { used: 0, free: 0, total: 0, configured: 0, enabled: 0, ready: 0, problemCount: 0, attentionItems: [] });
  summary.usedPercent = summary.total ? (summary.used / summary.total) * 100 : 0;
  return summary;
}

function healthSummary(data) {
  const storage = storageSummary(data.storage || []);
  const heartbeats = data.recorder?.heartbeats || [];
  const jobs = data.recorder?.active_jobs || [];
  const cameras = data.cameras || [];
  const alerts = data.openAlerts || [];
  const recorderOnline = heartbeats.filter((row) => recorderStatusKind(row) === 'ok').length;
  const enabledCameras = cameras.filter((camera) => camera.enabled).length;
  const recordingCameras = cameras.filter((camera) => camera.enabled && camera.recording_enabled).length;
  const streamingCameras = cameras.filter((camera) => camera.enabled && camera.stream_enabled !== false).length;
  const backendOk = data.health?.status === 'ok' && data.health?.database === 'ok';
  const errorAlerts = alerts.filter((alertItem) => alertItem.severity === 'error').length;
  const storageProblems = storage.problemCount;
  const storageNotReady = storage.problemCount;
  let score = 100;
  if (!backendOk) score -= 30;
  if (heartbeats.length && !recorderOnline) score -= 25;
  if (!heartbeats.length) score -= 15;
  score -= Math.min(errorAlerts * 18, 36);
  score -= Math.min(storageProblems * 12, 30);
  score -= Math.min(storageNotReady * 16, 32);
  if (enabledCameras && !recordingCameras) score -= 10;
  score = Math.max(0, Math.min(100, score));

  let tone = 'ok';
  let title = 'System is healthy';
  let message = 'Backend, database, storage, and recorder status look ready.';
  if (score < 60 || errorAlerts || !backendOk) {
    tone = 'error';
    title = 'Attention required';
    message = 'One or more critical services or alerts need action.';
  } else if (score < 85 || alerts.length || storageProblems || storageNotReady || !recorderOnline) {
    tone = 'warning';
    title = 'Watch closely';
    message = 'The system is usable, but there are warnings worth checking.';
  }

  return {
    activeJobs: jobs.length,
    enabledCameras,
    message,
    recordingCameras,
    recorderOnline,
    recorderTotal: heartbeats.length,
    score,
    storageFree: storage.free,
    storageHealthy: storage.ready,
    storageProblems,
    storageTotal: storage.configured,
    streamingCameras,
    title,
    tone,
  };
}

function storageStatus(item) {
  return storageInsights(item).status;
}

function storageInsights(item) {
  const enabled = Boolean(item.is_enabled);
  const used = Number(item.used_bytes || 0);
  const detectedFree = Number(item.free_bytes || 0);
  const detectedTotal = Number(item.total_bytes || used + detectedFree || 0);
  const configuredLimit = Number(item.max_storage_bytes || 0);
  const total = configuredLimit || detectedTotal;
  const free = configuredLimit ? Math.max(0, configuredLimit - used) : detectedFree;
  const usedPercent = total ? clampPercent((used / total) * 100) : clampPercent(item.used_percent);
  const healthStatus = String(item.health_status || 'unknown').toLowerCase();
  const reasons = [];
  let status = { kind: 'ok', badge: 'ok', label: 'healthy' };

  if (!enabled) {
    return {
      attention: false,
      configuredLimit,
      detectedTotal,
      free,
      reasons: ['Disabled by admin. This location is not available for new recording assignments.'],
      status: { kind: 'muted', badge: 'muted', label: 'disabled' },
      total,
      used,
      usedPercent,
    };
  }

  if (!item.exists) reasons.push('Folder does not exist inside the backend container.');
  if (!item.writable) reasons.push('Folder is not writable by the backend container.');
  if (item.latest_validation_error) reasons.push(item.latest_validation_error);
  if (healthStatus === 'unhealthy' || healthStatus === 'error') reasons.push(`Backend validation status is ${healthStatus}.`);
  if (configuredLimit && used >= configuredLimit) reasons.push('Configured storage limit has been reached.');
  if (usedPercent >= 90 && (!configuredLimit || used < configuredLimit)) reasons.push(`${usedPercent.toFixed(1)}% of ${configuredLimit ? 'configured limit' : 'detected disk capacity'} is used.`);

  const hasError = !item.exists || !item.writable || Boolean(item.latest_validation_error) || healthStatus === 'unhealthy' || healthStatus === 'error' || (configuredLimit && used >= configuredLimit);
  const hasWarning = healthStatus === 'warning' || usedPercent >= 90;
  if (hasError) {
    status = { kind: 'error', badge: 'error', label: 'error' };
  } else if (hasWarning) {
    status = { kind: 'warning', badge: 'warning', label: 'warning' };
  }

  if (reasons.length === 0) {
    reasons.push('Enabled, folder exists, writable, and capacity is within limits.');
  }

  return {
    attention: status.kind === 'warning' || status.kind === 'error',
    configuredLimit,
    detectedTotal,
    free,
    reasons,
    status,
    total,
    used,
    usedPercent,
  };
}

function recorderStatusText(row) {
  const kind = recorderStatusKind(row);
  if (kind === 'ok') return 'online';
  if (kind === 'muted') return 'stopped';
  return 'stale';
}

function humanizeKey(value) {
  return String(value || 'unknown')
    .replace(/[_-]+/g, ' ')
    .replace(/\s+/g, ' ')
    .trim()
    .replace(/\b\w/g, (char) => char.toUpperCase());
}

function formatRelativeTime(value) {
  if (!value) return 'never';
  const date = value instanceof Date ? value : new Date(value);
  if (Number.isNaN(date.getTime())) return 'unknown';
  const diffMs = date.getTime() - Date.now();
  const absMs = Math.abs(diffMs);
  const units = [
    ['day', 86_400_000],
    ['hour', 3_600_000],
    ['minute', 60_000],
    ['second', 1_000],
  ];
  const formatter = new Intl.RelativeTimeFormat(undefined, { numeric: 'auto' });
  for (const [unit, size] of units) {
    if (absMs >= size || unit === 'second') {
      return formatter.format(Math.round(diffMs / size), unit);
    }
  }
  return date.toLocaleString();
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

function Field({ label, help, children, className = '' }) {
  return <label className={`field${className ? ` ${className}` : ''}`}><span>{label}</span>{children}{help && <small>{help}</small>}</label>;
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

function NotificationsPage() {
  const [channels, setChannels] = useState([]);
  const [rules, setRules] = useState([]);
  const [cameras, setCameras] = useState([]);
  const [channelForm, setChannelForm] = useState({ name: '', bot_token: '', chat_id: '', enabled: true });
  const [ruleForm, setRuleForm] = useState({
    name: 'Motion to Telegram',
    notification_channel_id: '',
    camera_id: '',
    cooldown_seconds: 300,
    message_template: 'Motion detected on {{camera_name}}\nTime: {{event_time}}\nScore: {{score}}',
    attach_image: true,
    attach_video: true,
    pre_event_seconds: 7,
    post_event_seconds: 3,
    video_fps: 4,
    enabled: true,
  });
  const [actionError, setActionError] = useState('');
  const { loading, error, run } = useLoader(load);

  async function load() {
    const [channelsData, rulesData, camerasData] = await Promise.all([
      api('/api/notification-channels'),
      api('/api/notification-rules'),
      api('/api/cameras'),
    ]);
    const nextChannels = channelsData.notification_channels || [];
    setChannels(nextChannels);
    setRules(rulesData.notification_rules || []);
    setCameras(camerasData.cameras || []);
    setRuleForm((current) => (
      current.notification_channel_id || !nextChannels.length
        ? current
        : { ...current, notification_channel_id: nextChannels[0].id }
    ));
  }

  useEffect(() => { run(); }, []);

  async function createChannel(event) {
    event.preventDefault();
    setActionError('');
    try {
      await api('/api/notification-channels', {
        method: 'POST',
        body: JSON.stringify({
          name: channelForm.name.trim(),
          method: 'telegram',
          enabled: channelForm.enabled,
          config: {
            bot_token: channelForm.bot_token.trim(),
            chat_id: channelForm.chat_id.trim(),
          },
        }),
      });
      setChannelForm({ name: '', bot_token: '', chat_id: '', enabled: true });
      run();
    } catch (err) {
      setActionError(err.message);
    }
  }

  async function createRule(event) {
    event.preventDefault();
    setActionError('');
    try {
      await api('/api/notification-rules', {
        method: 'POST',
        body: JSON.stringify({
          name: ruleForm.name.trim(),
          event_type: 'motion_detected',
          enabled: ruleForm.enabled,
          notification_channel_id: ruleForm.notification_channel_id,
          camera_id: ruleForm.camera_id || null,
          cooldown_seconds: Number(ruleForm.cooldown_seconds),
          message_template: ruleForm.message_template,
          attach_image: ruleForm.attach_image,
          attach_video: ruleForm.attach_video,
          pre_event_seconds: Number(ruleForm.pre_event_seconds),
          post_event_seconds: Number(ruleForm.post_event_seconds),
          video_fps: Number(ruleForm.video_fps),
        }),
      });
      setRuleForm((current) => ({ ...current, name: 'Motion to Telegram' }));
      run();
    } catch (err) {
      setActionError(err.message);
    }
  }

  async function deleteChannel(channel) {
    if (!window.confirm(`Delete notification channel "${channel.name}"?`)) return;
    await api(`/api/notification-channels/${channel.id}`, { method: 'DELETE' });
    run();
  }

  async function deleteRule(rule) {
    if (!window.confirm(`Delete notification rule "${rule.name}"?`)) return;
    await api(`/api/notification-rules/${rule.id}`, { method: 'DELETE' });
    run();
  }

  const cameraNames = new Map(cameras.map((camera) => [camera.id, camera.name]));
  const channelNames = new Map(channels.map((channel) => [channel.id, channel.name]));

  return (
    <Panel title="Notifications">
      <State loading={loading} error={error} />
      {actionError && <ErrorText message={actionError} />}
      <section className="notification-admin-grid">
        <form className="notification-card" onSubmit={createChannel}>
          <div className="notification-card-heading">
            <strong>Telegram channel</strong>
            <span>Configure where detector notifications are delivered.</span>
          </div>
          <Field label="Display name">
            <input value={channelForm.name} onChange={(event) => setChannelForm({ ...channelForm, name: event.target.value })} placeholder="Home Telegram" required />
          </Field>
          <Field label="Bot token">
            <input type="password" value={channelForm.bot_token} onChange={(event) => setChannelForm({ ...channelForm, bot_token: event.target.value })} placeholder="123456:telegram-bot-token" required autoComplete="new-password" />
          </Field>
          <Field label="Chat ID">
            <input value={channelForm.chat_id} onChange={(event) => setChannelForm({ ...channelForm, chat_id: event.target.value })} placeholder="123456789" required />
          </Field>
          <label className="switch-row">
            <input type="checkbox" checked={channelForm.enabled} onChange={(event) => setChannelForm({ ...channelForm, enabled: event.target.checked })} />
            <span><strong>Channel enabled</strong><small>Allow rules to send messages through this Telegram destination.</small></span>
          </label>
          <div className="form-actions"><button>Create channel</button></div>
        </form>

        <form className="notification-card" onSubmit={createRule}>
          <div className="notification-card-heading">
            <strong>Motion notification rule</strong>
            <span>Choose which camera sends which Telegram message, and how often.</span>
          </div>
          <div className="notification-rule-grid">
            <Field label="Rule name">
              <input value={ruleForm.name} onChange={(event) => setRuleForm({ ...ruleForm, name: event.target.value })} required />
            </Field>
            <Field label="Telegram channel">
              <select value={ruleForm.notification_channel_id} onChange={(event) => setRuleForm({ ...ruleForm, notification_channel_id: event.target.value })} required>
                <option value="">Choose channel</option>
                {channels.map((channel) => <option key={channel.id} value={channel.id}>{channel.name}</option>)}
              </select>
            </Field>
            <Field label="Camera">
              <select value={ruleForm.camera_id} onChange={(event) => setRuleForm({ ...ruleForm, camera_id: event.target.value })}>
                <option value="">All cameras</option>
                {cameras.map((camera) => <option key={camera.id} value={camera.id}>{camera.name}</option>)}
              </select>
            </Field>
            <Field label="Cooldown seconds">
              <input type="number" min="0" value={ruleForm.cooldown_seconds} onChange={(event) => setRuleForm({ ...ruleForm, cooldown_seconds: event.target.value })} />
            </Field>
          </div>
          <Field label="Telegram message" help="Supported placeholders: {{camera_name}}, {{camera_id}}, {{event_time}}, {{score}}.">
            <textarea rows="4" value={ruleForm.message_template} onChange={(event) => setRuleForm({ ...ruleForm, message_template: event.target.value })} />
          </Field>
          <div className="notification-rule-grid compact">
            <Field label="Pre seconds">
              <input type="number" min="0" max="120" value={ruleForm.pre_event_seconds} onChange={(event) => setRuleForm({ ...ruleForm, pre_event_seconds: event.target.value })} />
            </Field>
            <Field label="Post seconds">
              <input type="number" min="0" max="120" value={ruleForm.post_event_seconds} onChange={(event) => setRuleForm({ ...ruleForm, post_event_seconds: event.target.value })} />
            </Field>
            <Field label="Video FPS">
              <input type="number" min="1" max="15" value={ruleForm.video_fps} onChange={(event) => setRuleForm({ ...ruleForm, video_fps: event.target.value })} />
            </Field>
          </div>
          <div className="camera-switches two-column">
            <label className="switch-row"><input type="checkbox" checked={ruleForm.enabled} onChange={(event) => setRuleForm({ ...ruleForm, enabled: event.target.checked })} /> Enabled</label>
            <label className="switch-row"><input type="checkbox" checked={ruleForm.attach_image} onChange={(event) => setRuleForm({ ...ruleForm, attach_image: event.target.checked })} /> Attach image</label>
            <label className="switch-row"><input type="checkbox" checked={ruleForm.attach_video} onChange={(event) => setRuleForm({ ...ruleForm, attach_video: event.target.checked })} /> Attach video</label>
          </div>
          <div className="form-actions"><button disabled={!channels.length}>Create rule</button></div>
        </form>
      </section>

      <h3>Telegram channels</h3>
      <DataTable
        columns={['name', 'method', 'enabled', 'chat_id', 'actions']}
        rows={channels.map((channel) => ({
          id: channel.id,
          name: channel.name,
          method: channel.method,
          enabled: channel.enabled ? <StatusBadge kind="ok" text="enabled" /> : <StatusBadge kind="muted" text="disabled" />,
          chat_id: channel.config?.chat_id,
          actions: <button type="button" onClick={() => deleteChannel(channel)}>Delete</button>,
        }))}
      />

      <h3>Notification rules</h3>
      <DataTable
        columns={['name', 'event', 'channel', 'camera', 'cooldown', 'enabled', 'actions']}
        rows={rules.map((rule) => ({
          id: rule.id,
          name: rule.name,
          event: rule.event_type,
          channel: channelNames.get(rule.notification_channel_id) || shortID(rule.notification_channel_id),
          camera: rule.camera_id ? (cameraNames.get(rule.camera_id) || shortID(rule.camera_id)) : 'All cameras',
          cooldown: `${rule.cooldown_seconds}s`,
          enabled: rule.enabled ? <StatusBadge kind="ok" text="enabled" /> : <StatusBadge kind="muted" text="disabled" />,
          actions: <button type="button" onClick={() => deleteRule(rule)}>Delete</button>,
        }))}
      />
    </Panel>
  );
}

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
