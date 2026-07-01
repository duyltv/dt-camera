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
            <Route index element={<Dashboard />} />
            <Route path="storage" element={<AdminOnly><StoragePage /></AdminOnly>} />
            <Route path="cameras" element={<AdminOnly><CamerasPage /></AdminOnly>} />
            <Route path="users" element={<AdminOnly><UsersPage /></AdminOnly>} />
            <Route path="permissions" element={<AdminOnly><PermissionsPage /></AdminOnly>} />
            <Route path="health" element={<AdminOnly><HealthDashboardPage /></AdminOnly>} />
            <Route path="alerts" element={<AdminOnly><AlertsPage /></AdminOnly>} />
            <Route path="layouts" element={<LayoutsPage />} />
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
          <NavLink to="/layouts">Layouts</NavLink>
          <NavLink to="/live">Live</NavLink>
          <NavLink to="/playback">Playback</NavLink>
        </nav>
        {user.role === 'admin' && (
          <nav className="nav-group nav-group-admin">
            <span className="nav-label">Admin <AdminBadge /></span>
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

  return (
    <Panel title="Storage Locations">
      <FormGrid onSubmit={create}>
        <input placeholder="Name" value={form.name} onChange={(e) => setForm({ ...form, name: e.target.value })} />
        <input placeholder="/recordings" value={form.container_path} onChange={(e) => setForm({ ...form, container_path: e.target.value })} />
        <label className="check"><input type="checkbox" checked={form.enabled} onChange={(e) => setForm({ ...form, enabled: e.target.checked })} /> Enabled</label>
        <button>Create</button>
      </FormGrid>
      <State loading={loading} error={error} />
      {items.length ? (
        <DataTable rows={items.map(formatStorageRow)} columns={['name', 'container_path', 'enabled', 'health_status', 'exists', 'writable', 'used_percent', 'free_bytes', 'latest_validation_error']} />
      ) : (
        !loading && <EmptyState title="No storage locations" body="Create a storage location to enable recording." />
      )}
    </Panel>
  );
}

function CamerasPage() {
  const [cameras, setCameras] = useState([]);
  const [storage, setStorage] = useState([]);
  const [form, setForm] = useState(newCameraForm());
  const [editingId, setEditingId] = useState('');
  const [editForm, setEditForm] = useState(null);
  const [actionError, setActionError] = useState('');
  const { loading, error, run } = useLoader(load);

  async function load() {
    const [cameraData, storageData] = await Promise.all([api('/api/cameras'), api('/api/storage-locations')]);
    setCameras(cameraData.cameras || []);
    setStorage(storageData.storage_locations || []);
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

  return (
    <Panel title="Cameras">
      <section className="camera-admin-section">
        <div className="section-heading">
          <h3>Add Camera</h3>
          <p>RTSP credentials are stored by the backend and never shown back in the browser.</p>
        </div>
        <CameraForm
          form={form}
          setForm={setForm}
          storage={storage}
          onSubmit={create}
          submitText="Create camera"
          rtspRequired
        />
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
    retention_days: 30,
    max_storage_bytes: '',
  };
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
    retention_days: camera.retention_days || 30,
    max_storage_bytes: camera.max_storage_bytes || '',
  };
}

function CameraForm({ form, setForm, storage, onSubmit, submitText, rtspRequired = false, secondaryAction }) {
  return (
    <form className="camera-form" onSubmit={onSubmit}>
      <Field label="Camera name">
        <input value={form.name} onChange={(e) => setForm({ ...form, name: e.target.value })} required />
      </Field>
      <Field label={rtspRequired ? 'RTSP URL' : 'Replace RTSP URL'}>
        <input
          value={form.rtsp_url}
          onChange={(e) => setForm({ ...form, rtsp_url: e.target.value })}
          placeholder={rtspRequired ? 'rtsp://user:password@camera/stream' : 'Leave blank to keep current private URL'}
          required={rtspRequired}
        />
      </Field>
      <Field label="Storage location">
        <select value={form.storage_location_id} onChange={(e) => setForm({ ...form, storage_location_id: e.target.value })}>
          <option value="">No storage</option>
          {storage.map((item) => <option key={item.id} value={item.id}>{item.name}</option>)}
        </select>
      </Field>
      <Field label="Location">
        <input value={form.location} onChange={(e) => setForm({ ...form, location: e.target.value })} placeholder="Gate, lobby, office..." />
      </Field>
      <Field label="Group">
        <input value={form.camera_group} onChange={(e) => setForm({ ...form, camera_group: e.target.value })} placeholder="Outdoor, warehouse..." />
      </Field>
      <Field label="Retention days">
        <input type="number" min="1" value={form.retention_days} onChange={(e) => setForm({ ...form, retention_days: e.target.value })} />
      </Field>
      <Field label="Max storage bytes">
        <input type="number" min="1" value={form.max_storage_bytes} onChange={(e) => setForm({ ...form, max_storage_bytes: e.target.value })} placeholder="Optional" />
      </Field>
      <div className="camera-switches">
        <label className="switch-row"><input type="checkbox" checked={form.enabled} onChange={(e) => setForm({ ...form, enabled: e.target.checked })} /> Camera enabled</label>
        <label className="switch-row"><input type="checkbox" checked={form.recording_enabled} onChange={(e) => setForm({ ...form, recording_enabled: e.target.checked })} /> Record video</label>
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
  // Visual drag-and-drop editor for a single layout. Renders a CSS grid
  // sized to `layout.settings.columns` (default 4). Each tile is positioned
  // via `gridColumn` / `gridRow` derived from its (x, y, width, height).
  //
  // Interactions:
  //   - Click on empty grid cell -> open "add camera" picker for that cell.
  //   - Drag tile body -> move tile (snap to grid on release).
  //   - Drag one of 8 handles -> resize tile from that corner/edge.
  //   - Click tile body without dragging -> select it; show Edit / Delete.
  //
  // All edits are saved via the `onChange` callback (which is wired to a
  // PATCH/POST/DELETE in the parent). The canvas never blocks on the
  // network: state is optimistic and the parent's `run()` re-fetches the
  // canonical state after the save completes.
  const items = (layout.layout_items || []).slice().sort((a, b) => (a.display_order || 0) - (b.display_order || 0));
  const cols = Math.max(1, Number(layout.settings?.columns) || 4);
  const maxRow = items.reduce((m, it) => Math.max(m, (it.y || 0) + (it.height || 1)), 0);
  const rows = Math.max(3, maxRow + 2);
  const [selectedId, setSelectedId] = React.useState(null);
  const [pendingCell, setPendingCell] = React.useState(null); // {x,y}
  const [drag, setDrag] = React.useState(null); // active drag/resize state
  const gridRef = React.useRef(null);
  const cameraById = React.useMemo(() => {
    const m = new Map();
    (cameras || []).forEach((c) => m.set(c.id, c));
    return m;
  }, [cameras]);

  function gridRect() {
    const el = gridRef.current;
    if (!el) return null;
    const r = el.getBoundingClientRect();
    return { left: r.left, top: r.top, width: r.width, height: r.height, cols, rows };
  }
  function cellWidth() {
    const r = gridRect();
    return r ? r.width / r.cols : 0;
  }
  function cellHeight() {
    const r = gridRect();
    return r ? r.height / r.rows : 0;
  }
  function pixelToCell(px, py) {
    const r = gridRect();
    if (!r) return { x: 0, y: 0 };
    const cw = r.width / r.cols;
    const ch = r.height / r.rows;
    let cx = Math.floor((px - r.left) / cw);
    let cy = Math.floor((py - r.top) / ch);
    cx = Math.max(0, Math.min(cols - 1, cx));
    cy = Math.max(0, cy);
    return { x: cx, y: cy };
  }

  function startDrag(item, mode, event) {
    event.preventDefault();
    event.stopPropagation();
    setSelectedId(item.id);
    setDrag({
      itemId: item.id,
      mode, // 'move' | 'nw' | 'n' | 'ne' | 'e' | 'se' | 's' | 'sw' | 'w'
      startMouseX: event.clientX,
      startMouseY: event.clientY,
      startX: item.x,
      startY: item.y,
      startW: item.width,
      startH: item.height,
    });
  }

  function onGridMouseMove(event) {
    if (!drag) return;
    const dx = event.clientX - drag.startMouseX;
    const dy = event.clientY - drag.startMouseY;
    const cw = cellWidth() || 1;
    const ch = cellHeight() || 1;
    const dxc = Math.round(dx / cw);
    const dyc = Math.round(dy / ch);
    const it = items.find((x) => x.id === drag.itemId);
    if (!it) return;
    let { x, y, width, height } = it;
    const startX = drag.startX, startY = drag.startY, startW = drag.startW, startH = drag.startH;
    if (drag.mode === 'move') {
      x = Math.max(0, Math.min(cols - width, startX + dxc));
      y = Math.max(0, startY + dyc);
    } else {
      // Resize: each handle moves one or two of {x, y, width, height}.
      let nx = startX, ny = startY, nw = startW, nh = startH;
      if (drag.mode.includes('w')) { nx = Math.max(0, startX + dxc); nw = Math.max(1, startW - (nx - startX)); }
      if (drag.mode.includes('e')) { nw = Math.max(1, startW + dxc); }
      if (drag.mode.includes('n')) { ny = Math.max(0, startY + dyc); nh = Math.max(1, startH - (ny - startY)); }
      if (drag.mode.includes('s')) { nh = Math.max(1, startH + dyc); }
      // Keep within grid bounds.
      if (nx + nw > cols) nw = Math.max(1, cols - nx);
      if (nw < 1) nw = 1;
      if (nh < 1) nh = 1;
      x = nx; y = ny; width = nw; height = nh;
    }
    // Mutate the in-memory items copy; we save on mouseup.
    const idx = items.findIndex((x) => x.id === drag.itemId);
    if (idx >= 0 && (items[idx].x !== x || items[idx].y !== y || items[idx].width !== width || items[idx].height !== height)) {
      items[idx] = { ...items[idx], x, y, width, height };
    }
    // Force re-render with the same array reference is hard; just trigger.
    setDrag({ ...drag, _bump: (drag._bump || 0) + 1 });
  }

  function onGridMouseUp() {
    if (!drag) return;
    const it = items.find((x) => x.id === drag.itemId);
    setDrag(null);
    if (!it) return;
    onChange(items.map((x) => x.id === it.id ? { ...x, x: it.x, y: it.y, width: it.width, height: it.height } : x));
  }

  async function saveItem(item) {
    try {
      await api(`/api/layouts/${layout.id}/items/${item.id}`, { method: 'PATCH', body: JSON.stringify({
        camera_id: item.camera_id,
        x: item.x, y: item.y, width: item.width, height: item.height,
        display_order: item.display_order, tile_type: item.tile_type,
      })});
    } catch (e) { onError(e.message); }
  }
  async function deleteItem(itemId) {
    try {
      await api(`/api/layouts/${layout.id}/items/${itemId}`, { method: 'DELETE' });
      onChange(items.filter((x) => x.id !== itemId));
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
      if (created && created.id) onChange([...items, created]);
    } catch (e) { onError(e.message); }
  }

  function onGridClick(event) {
    if (drag) return; // a drag just ended; ignore the synthetic click
    if (event.target !== gridRef.current) return;
    const cell = pixelToCell(event.clientX, event.clientY);
    setSelectedId(null);
    setPendingCell(cell);
  }

  const selected = selectedId ? items.find((x) => x.id === selectedId) : null;
  const tileCamera = selected ? cameraById.get(selected.camera_id) : null;

  return (
    <div className="layout-canvas-wrap">
      <div
        ref={gridRef}
        className="layout-canvas"
        style={{ gridTemplateColumns: `repeat(${cols}, 1fr)`, gridAutoRows: `minmax(64px, auto)` }}
        onMouseMove={onGridMouseMove}
        onMouseUp={onGridMouseUp}
        onMouseLeave={onGridMouseUp}
        onClick={onGridClick}
      >
        {items.map((it) => {
          const cam = cameraById.get(it.camera_id);
          return (
            <div
              key={it.id}
              className={'layout-tile' + (selectedId === it.id ? ' selected' : '')}
              style={{ gridColumn: `${(it.x || 0) + 1} / span ${Math.max(1, it.width || 1)}`, gridRow: `${(it.y || 0) + 1} / span ${Math.max(1, it.height || 1)}` }}
              onMouseDown={(e) => startDrag(it, 'move', e)}
              onClick={(e) => { e.stopPropagation(); setSelectedId(it.id); }}
            >
              <div className="layout-tile-label">
                <strong>{cam ? cam.name : shortID(it.camera_id)}</strong>
                <span className="muted">{it.width || 1}×{it.height || 1}</span>
              </div>
              {selectedId === it.id && (
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
            </div>
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
  function onItemsChanged(updatedItems) {
    // Optimistic local update; refetch canonical state in background.
    setLayouts((prev) => prev.map((l) => l.id === activeId ? { ...l, layout_items: updatedItems } : l));
    run();
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
  const { loading, error, run } = useLoader(load);

  async function load() {
    const data = await api('/api/layouts');
    const list = data.layouts || [];
    setLayouts(list);
    if (!layoutId && list.length) setLayoutId(list[0].id);
  }
  useEffect(() => { run(); }, []);

  const layout = layouts.find((l) => l.id === layoutId) || null;

  async function openLive() {
    if (!layoutId) return;
    setOpening(true);
    setActionError('');
    try {
      setResult(await api(`/api/live/layouts/${layoutId}`));
    } catch (err) {
      setActionError(err.message || 'Unable to open live view.');
    } finally {
      setOpening(false);
    }
  }

  return (
    <Panel title="Live Layout">
      <Toolbar>
        <select value={layoutId} onChange={(e) => setLayoutId(e.target.value)} disabled={!layouts.length}>
          {layouts.length === 0 && <option value="">No layouts</option>}
          {layouts.map((l) => <option key={l.id} value={l.id}>{l.name}</option>)}
        </select>
        <button onClick={openLive} disabled={!layoutId || opening}>{opening ? 'Opening...' : 'Open live'}</button>
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
  const [result, setResult] = useState(null);
  const [actionError, setActionError] = useState('');
  const [opening, setOpening] = useState(false);
  const { loading, error, run } = useLoader(load);

  async function load() {
    const data = await api('/api/layouts');
    const list = data.layouts || [];
    setLayouts(list);
    if (!layoutId && list.length) setLayoutId(list[0].id);
  }
  useEffect(() => { run(); }, []);

  async function openPlayback() {
    if (!layoutId) return;
    setOpening(true);
    setActionError('');
    try {
      setResult(await api('/api/playback/prepare', {
        method: 'POST',
        body: JSON.stringify({ layout_id: layoutId, selected_timestamp: new Date().toISOString() }),
      }));
    } catch (err) {
      setActionError(err.message || 'Unable to open playback.');
    } finally {
      setOpening(false);
    }
  }

  return (
    <Panel title="Playback Review">
      <Toolbar>
        <select value={layoutId} onChange={(e) => setLayoutId(e.target.value)} disabled={!layouts.length}>
          {layouts.length === 0 && <option value="">No layouts</option>}
          {layouts.map((l) => <option key={l.id} value={l.id}>{l.name}</option>)}
        </select>
        <button onClick={openPlayback} disabled={!layoutId || opening}>{opening ? 'Opening...' : 'Open playback'}</button>
      </Toolbar>
      <State loading={loading} error={error} />
      {actionError && <ErrorText message={actionError} />}
      {!layouts.length && !loading && <EmptyState title="No layouts" body="Create a layout to start playing recorded footage." />}
      {result && <VideoGrid cameras={result?.cameras || []} />}
    </Panel>
  );
}

function PlaybackLayoutGrid({ layout, cameras, playing, videoRefs }) {
  const byCamera = new Map(cameras.map((camera) => [camera.camera_id, camera]));
  const items = layout?.layout_items || cameras.map((camera, index) => ({
    camera_id: camera.camera_id,
    x: index % 2,
    y: Math.floor(index / 2),
    width: 1,
    height: 1,
  }));
  const columns = Math.max(1, Number(layout?.settings?.columns) || Math.max(...items.map((item) => Number(item.x || 0) + Number(item.width || 1)), 1));
  const rows = Math.max(1, Math.max(...items.map((item) => Number(item.y || 0) + Number(item.height || 1)), 1));

  if (!items.length) return <EmptyState title="Nothing to play" body="The selected layout has no cameras yet." />;

  return (
    <div className="layout-playback-grid" style={{ gridTemplateColumns: `repeat(${columns}, minmax(0, 1fr))`, gridAutoRows: 'minmax(180px, auto)' }}>
      {items.map((item) => {
        const camera = byCamera.get(item.camera_id) || { camera_id: item.camera_id, status: 'no_recording' };
        const register = (video) => {
          if (video) videoRefs.current.set(camera.camera_id, video);
          else videoRefs.current.delete(camera.camera_id);
        };
        return (
          <section
            className="video-tile layout-video-tile"
            key={item.id || item.item_id || item.camera_id}
            style={{
              gridColumn: `${Number(item.x || 0) + 1} / span ${Math.max(Number(item.width || 1), 1)}`,
              gridRow: `${Number(item.y || 0) + 1} / span ${Math.max(Number(item.height || 1), 1)}`,
            }}
          >
            <strong>{shortID(camera.camera_id)}</strong>
            <span><PlaybackStatusBadge status={camera.status} /></span>
            {camera.status === 'ok' && (
              <VideoPlayer
                src={camera.segment?.playback_url}
                offsetSeconds={camera.offset_seconds || 0}
                controlled
                playing={playing}
                register={register}
              />
            )}
            {camera.status !== 'ok' && <p>{playbackStatusText(camera.status)}</p>}
          </section>
        );
      })}
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

function LiveLayoutGrid({ layout, cameras, editable, onChange, onError }) {
  const items = (layout.layout_items || []).slice().sort((a, b) => (a.display_order || 0) - (b.display_order || 0));
  const byCamera = new Map(cameras.map((camera) => [camera.camera_id, camera]));
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

  if (!items.length) return <EmptyState title="No cameras in layout" body="Add cameras to this layout before opening live view." />;

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
              <strong>{shortID(camera.camera_id)}</strong>
              <span><LiveStatusBadge status={camera.status} /></span>
            </div>
            <div className="live-tile-video">
              {camera.status === 'ok' && <VideoPlayer src={camera.hls_url} autoPlay />}
              {camera.status !== 'ok' && <p>{camera.status}</p>}
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

function shortID(id) {
  return id ? id.slice(0, 8) : 'camera';
}

function playbackStatusText(status) {
  if (status === 'ok') return 'Ready';
  if (status === 'no_recording') return 'No recording for this time.';
  if (status === 'forbidden') return 'Permission required.';
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

function Field({ label, children }) {
  return <label className="field"><span>{label}</span>{children}</label>;
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
  if (status === 'camera_disabled') return <StatusBadge kind="muted" text="disabled" />;
  if (status === 'no_permission') return <StatusBadge kind="warning" text="no permission" />;
  if (status === 'stream_unavailable') return <StatusBadge kind="error" text="stream unavailable" />;
  return <StatusBadge kind="muted" text={status || 'unknown'} />;
}

function PlaybackStatusBadge({ status }) {
  if (status === 'ok') return <StatusBadge kind="ok" text="ready" />;
  if (status === 'no_recording') return <StatusBadge kind="muted" text="no recording" />;
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
