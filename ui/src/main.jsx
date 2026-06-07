import React, { useEffect, useMemo, useState } from 'react';
import { createRoot } from 'react-dom/client';
import { FileText, FolderOpen, MessageSquare, Moon, Power, RefreshCw, Save, Send, Settings, Shield, Sun } from 'lucide-react';
import './styles.css';

const fallback = {
  GetState: async () => ({
    enabled: true,
    status: 'Preview mode',
    config: { startWithWindows: false, globalIgnorePath: '%USERPROFILE%\\.ignore', logDir: '' },
    metrics: { filesCopied: 0, filesSkipped: 0, directoriesSkipped: 0, errors: 0, bytesCopied: 0, operations: 0, lastActivity: 'No activity yet', lastDurationMs: 0 }
  }),
  ReadGlobalIgnore: async () => '# Global rules for all projects\n\n[IGNORE]\n\nnode_modules\nvendor\ndist\nbuild\n.next\n.git\n\n.env\n*.log\n*.tmp\n',
  SaveGlobalIgnore: async () => {},
  ReloadRules: async () => {},
  SetEnabled: async () => {},
  SetStartWithWindows: async () => {},
  GetLogs: async () => 'Logs appear here after the desktop app starts.',
  OpenLogs: async () => {},
  GetFeedbackStatus: async () => ({ canSend: true, sentToday: 0, remaining: 3, limit: 3, sent: 0 }),
  SubmitFeedback: async () => ({ canSend: true, sentToday: 1, remaining: 2, limit: 3, sent: 1 }),
  GetFeedbacks: async () => []
};

function api() {
  return window.go?.app?.Ignore ?? fallback;
}

function fmtBytes(value = 0) {
  if (value < 1024) return `${value} B`;
  const units = ['KB', 'MB', 'GB', 'TB'];
  let n = value / 1024;
  let i = 0;
  while (n >= 1024 && i < units.length - 1) {
    n /= 1024;
    i++;
  }
  return `${n.toFixed(n >= 10 ? 1 : 2)} ${units[i]}`;
}

function App() {
  const [state, setState] = useState(null);
  const [content, setContent] = useState('');
  const [logs, setLogs] = useState('');
  const [tab, setTab] = useState('ignore');
  const [saving, setSaving] = useState(false);
  const [theme, setTheme] = useState(() => localStorage.getItem('ignore-theme') || 'light');
  const [feedback, setFeedback] = useState('');
  const [fbStatus, setFbStatus] = useState(null);
  const [fbList, setFbList] = useState([]);
  const [sending, setSending] = useState(false);
  const [fbMsg, setFbMsg] = useState('');

  const load = async () => {
    const svc = api();
    const [nextState, ignoreText, logText, feedbackStatus, feedbackList] = await Promise.all([
      svc.GetState(),
      svc.ReadGlobalIgnore().catch(() => ''),
      svc.GetLogs().catch(() => ''),
      svc.GetFeedbackStatus?.().catch(() => null) ?? null,
      svc.GetFeedbacks?.().catch(() => null) ?? null
    ]);
    setState(nextState);
    setContent(ignoreText);
    setLogs(logText);
    if (feedbackStatus) setFbStatus(feedbackStatus);
    if (Array.isArray(feedbackList)) setFbList(feedbackList);
  };

  const sendFeedback = async () => {
    if (!feedback.trim()) { setFbMsg('Please write a message first.'); return; }
    setSending(true);
    setFbMsg('');
    try {
      const status = await api().SubmitFeedback(feedback);
      if (status) setFbStatus(status);
      setFeedback('');
      setFbMsg('Thanks! Your feedback was sent to the developer.');
      try { localStorage.setItem('ignore-feedback-last', new Date().toISOString()); } catch {}
      const list = await api().GetFeedbacks?.().catch(() => null);
      if (Array.isArray(list)) setFbList(list);
    } catch (e) {
      setFbMsg(String(e?.message || e || 'Could not send feedback.'));
    } finally {
      setSending(false);
    }
  };

  const fmtFeedbackTime = (iso) => {
    if (!iso) return '';
    const d = new Date(iso);
    if (isNaN(d)) return iso;
    return d.toLocaleString();
  };

  useEffect(() => {
    load();
    const id = setInterval(load, 4000);
    return () => clearInterval(id);
  }, []);

  useEffect(() => {
    document.documentElement.dataset.theme = theme;
    localStorage.setItem('ignore-theme', theme);
  }, [theme]);

  const metrics = state?.metrics ?? {};
  const cards = useMemo(() => [
    ['Copied', metrics.filesCopied ?? 0],
    ['Skipped files', metrics.filesSkipped ?? 0],
    ['Skipped dirs', metrics.directoriesSkipped ?? metrics.directoriesSkip ?? 0],
    ['Bytes copied', fmtBytes(metrics.bytesCopied ?? 0)],
    ['Errors', metrics.errors ?? 0],
    ['Operations', metrics.operations ?? 0]
  ], [metrics]);

  const save = async () => {
    setSaving(true);
    try {
      await api().SaveGlobalIgnore(content);
      await load();
    } finally {
      setSaving(false);
    }
  };

  const toggleProtection = async () => {
    await api().SetEnabled(!state.enabled);
    await load();
  };

  const toggleStartup = async () => {
    await api().SetStartWithWindows(!state.config.startWithWindows);
    await load();
  };

  if (!state) {
    return (
      <main className="loading">
        <img src="/brand/ignore-logo-128.png" alt="" />
        <span>Ignore</span>
      </main>
    );
  }

  const tabs = [
    ['ignore', FileText, 'Ignore'],
    ['status', Settings, 'Status'],
    ['feedback', MessageSquare, 'Feedback'],
    ['about', Shield, 'About']
  ];

  const titles = {
    ignore: 'Global Ignore Editor',
    status: 'Current Status',
    feedback: 'Send Feedback',
    about: 'About Ignore'
  };
  const canSendFeedback = !fbStatus || fbStatus.canSend !== false;

  return (
    <main className="shell">
      <aside className="sidebar">
        <div className="brand">
          <img src="/brand/ignore-logo-64.png" alt="Ignore" />
          <div>
            <h1>Ignore</h1>
            <p>{state.status}</p>
          </div>
        </div>
        <nav>
          {tabs.map(([id, Icon, label]) => (
            <button key={id} className={tab === id ? 'active' : ''} onClick={() => setTab(id)} title={label}>
              <Icon size={17} />
              <span>{label}</span>
            </button>
          ))}
        </nav>
      </aside>

      <section className="content">
        <header>
          <div className="titleBlock">
            <span className={state.enabled ? 'pill on' : 'pill'}>{state.enabled ? 'Enabled' : 'Disabled'}</span>
            <h2>{titles[tab] ?? 'Ignore'}</h2>
          </div>
          <div className="actions">
            {tab === 'ignore' && (
              <button className="primary" onClick={save} disabled={saving}>
                <Save size={17} />
                <span>{saving ? 'Saving' : 'Save'}</span>
              </button>
            )}
            <button title="Reload rules" onClick={async () => { await api().ReloadRules(); await load(); }}>
              <RefreshCw size={18} />
              <span>Reload</span>
            </button>
            <button title={state.enabled ? 'Disable protection' : 'Enable protection'} className={state.enabled ? 'stateButton on' : 'stateButton'} onClick={toggleProtection}>
              <Power size={18} />
              <span>{state.enabled ? 'Enabled' : 'Disabled'}</span>
            </button>
            <button title={theme === 'light' ? 'Dark theme' : 'Light theme'} className="stateButton" onClick={() => setTheme(theme === 'light' ? 'dark' : 'light')}>
              {theme === 'light' ? <Moon size={18} /> : <Sun size={18} />}
              <span>{theme === 'light' ? 'Light' : 'Dark'}</span>
            </button>
          </div>
        </header>

        {tab === 'ignore' && (
          <div className="editorPane">
            <div className="pathLine">{state.config.globalIgnorePath}</div>
            <textarea value={content} onChange={(e) => setContent(e.target.value)} spellCheck="false" />
          </div>
        )}

        {tab === 'status' && (
          <div className="statusPane">
            <div className="toggles">
              <label><input type="checkbox" checked={state.enabled} onChange={toggleProtection} /> Enable protection</label>
              <label><input type="checkbox" checked={state.config.startWithWindows} onChange={toggleStartup} /> Start with Windows</label>
            </div>
            <div className="stats">{cards.map(([label, value]) => <div className="stat" key={label}><span>{label}</span><strong>{value}</strong></div>)}</div>
            <div className="statusGrid">
              <div className="activity statusCard">
                <h3>Activity</h3>
                <div className="activityRows">
                  <div><span>Latest</span><strong>{metrics.lastActivity || 'No activity yet'}</strong></div>
                  <div><span>Duration</span><strong>{metrics.lastDurationMs ? `${metrics.lastDurationMs} ms` : 'Idle'}</strong></div>
                </div>
              </div>
              <div className="logsPane statusCard">
                <div className="sectionHeader">
                  <h3>Logs</h3>
                  <div className="miniActions">
                    <button onClick={async () => setLogs(await api().GetLogs())}><RefreshCw size={16} /><span>Refresh</span></button>
                    <button onClick={() => api().OpenLogs()}><FolderOpen size={16} /><span>Open</span></button>
                  </div>
                </div>
                <pre>{logs}</pre>
              </div>
            </div>
          </div>
        )}

        {tab === 'feedback' && (
          <div className="feedbackPane">
            <div className="feedbackCard">
              <h3>Tell the developer what you think</h3>
              <p className="feedbackIntro">Found a bug, have an idea, or something not working? Send a short message. You can send <strong>up to 3 messages per day</strong>. Each message is saved locally and delivered to the developer.</p>
              <textarea
                className="feedbackInput"
                value={feedback}
                maxLength={300}
                placeholder={canSendFeedback ? 'Type your feedback here…' : 'You have reached today’s limit of 3 messages. Please come back tomorrow.'}
                onChange={(e) => setFeedback(e.target.value)}
                disabled={!canSendFeedback || sending}
                spellCheck="true"
              />
              <div className="feedbackFooter">
                <span className="feedbackHint">
                  {canSendFeedback
                    ? `${feedback.length}/300 · ${fbStatus?.remaining ?? 3} of ${fbStatus?.limit ?? 3} left today`
                    : 'Daily limit reached — 3 messages per day.'}
                </span>
                <button className="primary" onClick={sendFeedback} disabled={sending || !canSendFeedback}>
                  <Send size={16} />
                  <span>{sending ? 'Sending…' : 'Send feedback'}</span>
                </button>
              </div>
              {fbMsg && <div className="feedbackMsg">{fbMsg}</div>}
            </div>

            <div className="feedbackCard">
              <div className="sectionHeader">
                <h3>Your feedback</h3>
                <span className="feedbackHint">{fbList.length} message{fbList.length === 1 ? '' : 's'}</span>
              </div>
              {fbList.length === 0 ? (
                <p className="feedbackEmpty">You haven’t sent any feedback yet. Your messages will appear here.</p>
              ) : (
                <ul className="feedbackList">
                  {fbList.map((f, i) => (
                    <li key={i} className="feedbackRow">
                      <div className="feedbackRowHead">
                        <span className="feedbackTime">{fmtFeedbackTime(f.time)}</span>
                        <span className={f.delivered ? 'feedbackTag sent' : 'feedbackTag pending'}>
                          {f.delivered ? 'Delivered' : 'Saved locally'}
                        </span>
                      </div>
                      <p className="feedbackText">{f.message}</p>
                    </li>
                  ))}
                </ul>
              )}
            </div>
          </div>
        )}

        {tab === 'about' && (
          <div className="aboutPane">
            <div className="about">
              <img className="aboutLogo" src="/brand/ignore-logo-128.png" alt="Ignore" />
              <h3>Developer transfers without cleanup chores.</h3>
              <p>Ignore keeps a cached rule engine ready in the background, applies `.ignore` rules to high-performance filtered copies, and records activity in rotating local logs.</p>
              <p>Explorer clipboard fallback is enabled on Windows: when a file-list copy is detected, Ignore prepares a filtered staging copy and replaces the clipboard with the cleaned paths before paste.</p>
              <p>Deep Explorer transfer interception and universal browser upload filtering still require native shell or browser-extension integration. The current fallback is practical, but the shell-extension roadmap remains the long-term transparent solution.</p>

              <div className="guideBlock">
                <h3>Creating an .ignore file</h3>
                <p>Create a file named `.ignore` in your Windows user folder for global rules, or inside a project folder for project-specific rules.</p>
                <pre className="miniCode">{`# Rules above this line are ignored

[IGNORE]

node_modules
vendor
dist
build
.next
.git

.env
*.log
*.tmp
*.cache`}</pre>
                <p>Rules only work after `[IGNORE]`. Empty lines and lines starting with `#` are skipped. Folder names match recursively, and wildcards like `*.log` are supported.</p>
              </div>

              <div className="profileBlock">
                <h3>About the creator</h3>
                <p>Created by Muhammad Shoaib.</p>
                <div className="profileLinks">
                  <a href="https://github.com/ShoaibShokat03" target="_blank" rel="noreferrer">GitHub</a>
                  <a href="https://www.linkedin.com/in/muhammad-shoaib-776521204" target="_blank" rel="noreferrer">LinkedIn</a>
                </div>
              </div>
            </div>
          </div>
        )}
      </section>
    </main>
  );
}

createRoot(document.getElementById('root')).render(<App />);
