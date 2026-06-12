import {useCallback, useEffect, useMemo, useState} from "react";
import {
  RegConfig,
  RegSetConfig,
  RegTaskCancel,
  RegTaskCreate,
  RegTaskDelete,
  RegTaskList,
  RegTaskLogs,
  RegTaskPurge,
  RegTaskStats,
} from "../../wailsjs/go/main/App";
import {dto} from "../../wailsjs/go/models";

type TaskRow = dto.RegisterTaskResp;
type TaskStats = dto.RegisterTaskStatsResp;
type LogRow = dto.RegisterTaskLogResp;

const statusLabels: Record<string, string> = {
  pending: "排队中",
  running: "执行中",
  success: "成功",
  failed: "失败",
  cancelled: "已取消",
};

const statusStyles: Record<string, string> = {
  pending: "bg-slate-100 text-slate-600 border-slate-200",
  running: "bg-amber-50 text-amber-700 border-amber-200",
  success: "bg-emerald-50 text-emerald-700 border-emerald-200",
  failed: "bg-rose-50 text-rose-700 border-rose-200",
  cancelled: "bg-slate-100 text-slate-500 border-slate-200",
};

const levelStyles: Record<string, string> = {
  info: "text-slate-600",
  warn: "text-amber-600",
  error: "text-rose-600",
};

const pageSize = 20;

const PROXY_KEY = "proxy.dynamic_url";

const configFields: Array<{key: string; label: string; placeholder?: string; type?: string; hint?: string}> = [
  {key: "register.worker_concurrency", label: "并发数", placeholder: "3", type: "number"},
  {key: "mail.default_backend", label: "默认收件后端", placeholder: "留空=邮箱池 / cf / tempmail"},
];

function StatCard({label, value, tone}: {label: string; value: number; tone: string}) {
  return (
    <div className={`border px-3 py-2 ${tone}`}>
      <div className="text-[11px] opacity-80">{label}</div>
      <div className="mt-0.5 text-lg font-medium tabular-nums">{value}</div>
    </div>
  );
}

export function RegisterPoolPage() {
  const [rows, setRows] = useState<TaskRow[]>([]);
  const [total, setTotal] = useState(0);
  const [stats, setStats] = useState<TaskStats | null>(null);
  const [page, setPage] = useState(1);
  const [statusFilter, setStatusFilter] = useState("");
  const [keyword, setKeyword] = useState("");
  const [keywordInput, setKeywordInput] = useState("");
  const [loading, setLoading] = useState(false);
  const [message, setMessage] = useState("");

  const [count, setCount] = useState(1);
  const [creating, setCreating] = useState(false);

  const [showConfig, setShowConfig] = useState(false);
  const [config, setConfig] = useState<Record<string, string>>({});
  const [savingConfig, setSavingConfig] = useState(false);

  const [logTask, setLogTask] = useState<TaskRow | null>(null);
  const [logs, setLogs] = useState<LogRow[]>([]);
  const [logLoading, setLogLoading] = useState(false);

  const [proxyConfigured, setProxyConfigured] = useState<boolean | null>(null);

  const totalPages = Math.max(1, Math.ceil(total / pageSize));

  useEffect(() => {
    RegConfig()
      .then((cfg) => setProxyConfigured(Boolean(cfg && (cfg[PROXY_KEY] || "").trim())))
      .catch(() => setProxyConfigured(null));
  }, [showConfig]);

  const refresh = useCallback(async () => {
    setLoading(true);
    try {
      const [list, st] = await Promise.all([
        RegTaskList("gpt", statusFilter, keyword, page, pageSize),
        RegTaskStats("gpt"),
      ]);
      setRows(list.items || []);
      setTotal(list.total || 0);
      setStats(st);
    } catch (err) {
      setMessage(`加载失败：${String(err)}`);
    } finally {
      setLoading(false);
    }
  }, [statusFilter, keyword, page]);

  useEffect(() => {
    void refresh();
  }, [refresh]);

  useEffect(() => {
    const hasActive = rows.some((r) => r.status === "pending" || r.status === "running");
    if (!hasActive) return;
    const timer = setInterval(() => void refresh(), 3000);
    return () => clearInterval(timer);
  }, [rows, refresh]);

  const loadConfig = async () => {
    try {
      const cfg = await RegConfig();
      setConfig(cfg || {});
      setShowConfig(true);
    } catch (err) {
      setMessage(`读取配置失败：${String(err)}`);
    }
  };

  const saveConfig = async () => {
    setSavingConfig(true);
    try {
      await RegSetConfig(config);
      setMessage("注册配置已保存");
      setShowConfig(false);
    } catch (err) {
      setMessage(`保存失败：${String(err)}`);
    } finally {
      setSavingConfig(false);
    }
  };

  const createTasks = async () => {
    setCreating(true);
    try {
      const res = await RegTaskCreate("gpt", count, {});
      setMessage(`已创建 ${res.created} 个注册任务`);
      setPage(1);
      await refresh();
    } catch (err) {
      setMessage(`创建失败：${String(err)}`);
    } finally {
      setCreating(false);
    }
  };

  const cancelTask = async (id: number) => {
    await RegTaskCancel(id);
    await refresh();
  };
  const deleteTask = async (id: number) => {
    await RegTaskDelete(id);
    await refresh();
  };
  const purge = async () => {
    const n = await RegTaskPurge("gpt");
    setMessage(`已清理 ${n} 个已结束任务`);
    await refresh();
  };

  const openLogs = async (task: TaskRow) => {
    setLogTask(task);
    setLogLoading(true);
    try {
      const data = await RegTaskLogs(task.id, "", 300);
      setLogs(data || []);
    } catch (err) {
      setMessage(`读取日志失败：${String(err)}`);
    } finally {
      setLogLoading(false);
    }
  };

  const configEntries = useMemo(() => configFields, []);

  return (
    <main className="flex min-h-0 flex-1 flex-col overflow-hidden">
      <div className="flex shrink-0 items-center justify-between border-b border-slate-200 bg-white px-5 py-3">
        <div>
          <h1 className="app-title">号池注册</h1>
          <p className="mt-0.5 text-sm text-slate-500">批量注册 GPT 账号，自动领用邮箱池并写入号池</p>
        </div>
        <div className="flex items-center gap-2">
          {message ? <span className="max-w-[260px] truncate text-xs text-slate-500">{message}</span> : null}
          <button className="btn-secondary" type="button" onClick={() => void loadConfig()}>
            注册设置
          </button>
          <button className="btn-secondary" type="button" onClick={() => void purge()}>
            清理已结束
          </button>
          <button className="btn-secondary" type="button" onClick={() => void refresh()}>
            刷新
          </button>
        </div>
      </div>

      <div className="min-h-0 flex-1 overflow-auto p-4">
        {proxyConfigured === false ? (
          <div className="mb-4 flex items-center justify-between gap-3 border-l-4 border-amber-500 bg-amber-50 px-4 py-3">
            <div className="text-sm text-amber-800">
              <span className="font-medium">⚠ 尚未配置代理网关</span>
              <span className="ml-2 text-xs text-amber-700">
                当前所有注册/出图请求都直连本机 IP，极易触发风控被封号，强烈建议先在「系统设置 → 公共代理网关」配置动态代理。
              </span>
            </div>
          </div>
        ) : null}
        <div className="mb-4 grid grid-cols-3 gap-2 sm:grid-cols-6">
          <StatCard label="总数" value={stats?.total ?? 0} tone="border-slate-200 bg-white text-slate-700" />
          <StatCard label="排队中" value={stats?.pending ?? 0} tone={statusStyles.pending} />
          <StatCard label="执行中" value={stats?.running ?? 0} tone={statusStyles.running} />
          <StatCard label="成功" value={stats?.success ?? 0} tone={statusStyles.success} />
          <StatCard label="失败" value={stats?.failed ?? 0} tone={statusStyles.failed} />
          <StatCard label="已取消" value={stats?.cancelled ?? 0} tone={statusStyles.cancelled} />
        </div>

        <div className="panel mb-4">
          <div className="panel-header">
            <h2 className="section-title">新建注册任务</h2>
            <p className="section-desc">姓名/密码自动生成；邮箱从可用邮箱池自动领取</p>
          </div>
          <div className="flex flex-wrap items-end gap-3 p-4">
            <label className="w-40">
              <span className="field-label">数量</span>
              <input
                className="field-input"
                type="number"
                min={1}
                max={5000}
                value={count}
                onChange={(e) => setCount(Math.max(1, Number(e.target.value) || 1))}
              />
            </label>
            <button className="btn-primary" type="button" disabled={creating} onClick={() => void createTasks()}>
              {creating ? "提交中…" : `创建 ${count} 个任务`}
            </button>
          </div>
        </div>

        <div className="panel">
          <div className="panel-header flex flex-wrap items-center gap-2">
            <select
              className="field-input max-w-[140px]"
              value={statusFilter}
              onChange={(e) => {
                setPage(1);
                setStatusFilter(e.target.value);
              }}
            >
              <option value="">全部状态</option>
              {Object.entries(statusLabels).map(([k, v]) => (
                <option key={k} value={k}>
                  {v}
                </option>
              ))}
            </select>
            <form
              className="flex items-center gap-2"
              onSubmit={(e) => {
                e.preventDefault();
                setPage(1);
                setKeyword(keywordInput.trim());
              }}
            >
              <input
                className="field-input max-w-[220px]"
                placeholder="搜索邮箱"
                value={keywordInput}
                onChange={(e) => setKeywordInput(e.target.value)}
              />
              <button className="btn-secondary shrink-0 whitespace-nowrap" type="submit">
                搜索
              </button>
            </form>
          </div>

          <div className="overflow-auto">
            <table className="w-full border-collapse text-sm">
              <thead>
                <tr className="border-b border-slate-200 bg-slate-50 text-left text-xs text-slate-500">
                  <th className="w-16 px-3 py-2">ID</th>
                  <th className="px-3 py-2">邮箱</th>
                  <th className="px-3 py-2">状态</th>
                  <th className="px-3 py-2">进度</th>
                  <th className="px-3 py-2">步骤 / 错误</th>
                  <th className="px-3 py-2">创建时间</th>
                  <th className="w-32 px-3 py-2">操作</th>
                </tr>
              </thead>
              <tbody>
                {rows.map((row) => (
                  <tr key={row.id} className="border-b border-slate-100 hover:bg-slate-50">
                    <td className="px-3 py-2 tabular-nums text-slate-500">{row.id}</td>
                    <td className="px-3 py-2 text-slate-800">{row.email || "-"}</td>
                    <td className="px-3 py-2">
                      <span className={`inline-flex border px-1.5 py-0.5 text-[11px] ${statusStyles[row.status] || ""}`}>
                        {statusLabels[row.status] || row.status}
                      </span>
                    </td>
                    <td className="px-3 py-2">
                      <div className="flex items-center gap-2">
                        <div className="h-1.5 w-20 overflow-hidden rounded-sm bg-slate-100">
                          <div className="h-full bg-brand-600" style={{width: `${row.progress || 0}%`}} />
                        </div>
                        <span className="text-[11px] tabular-nums text-slate-500">{row.progress || 0}%</span>
                      </div>
                    </td>
                    <td className="px-3 py-2">
                      {row.error ? (
                        <span className="block max-w-[240px] truncate text-[11px] text-rose-500" title={row.error}>
                          {row.error}
                        </span>
                      ) : (
                        <span className="text-[11px] text-slate-500">{row.step || "-"}</span>
                      )}
                    </td>
                    <td className="px-3 py-2 text-xs text-slate-500">
                      {row.created_at ? new Date(row.created_at).toLocaleString() : "-"}
                    </td>
                    <td className="px-3 py-2">
                      <div className="flex items-center gap-2">
                        <button className="btn-text text-[11px]" type="button" onClick={() => void openLogs(row)}>
                          日志
                        </button>
                        {row.status === "pending" || row.status === "running" ? (
                          <button
                            className="btn-text text-[11px] text-amber-600"
                            type="button"
                            onClick={() => void cancelTask(row.id)}
                          >
                            取消
                          </button>
                        ) : (
                          <button
                            className="btn-text text-[11px] text-rose-600"
                            type="button"
                            onClick={() => void deleteTask(row.id)}
                          >
                            删除
                          </button>
                        )}
                      </div>
                    </td>
                  </tr>
                ))}
                {rows.length === 0 ? (
                  <tr>
                    <td colSpan={7} className="px-3 py-10 text-center text-sm text-slate-400">
                      {loading ? "加载中…" : "暂无注册任务"}
                    </td>
                  </tr>
                ) : null}
              </tbody>
            </table>
          </div>

          <div className="flex items-center justify-between border-t border-slate-200 px-3 py-2 text-xs text-slate-500">
            <span>
              共 {total} 条 · 第 {page}/{totalPages} 页
            </span>
            <div className="flex items-center gap-2">
              <button
                className="btn-secondary"
                type="button"
                disabled={page <= 1}
                onClick={() => setPage((p) => Math.max(1, p - 1))}
              >
                上一页
              </button>
              <button
                className="btn-secondary"
                type="button"
                disabled={page >= totalPages}
                onClick={() => setPage((p) => Math.min(totalPages, p + 1))}
              >
                下一页
              </button>
            </div>
          </div>
        </div>
      </div>

      {showConfig ? (
        <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/40 p-4">
          <div className="w-full max-w-xl border border-slate-200 bg-white shadow-xl">
            <div className="flex items-center justify-between border-b border-slate-200 px-4 py-3">
              <h2 className="section-title">注册设置</h2>
              <button className="btn-text" type="button" onClick={() => setShowConfig(false)}>
                关闭
              </button>
            </div>
            <div className="max-h-[60vh] space-y-3 overflow-auto p-4">
              {configEntries.map((f) => (
                <label key={f.key} className="block">
                  <span className="field-label">{f.label}</span>
                  <input
                    className="field-input"
                    type={f.type || "text"}
                    placeholder={f.placeholder}
                    value={config[f.key] ?? ""}
                    onChange={(e) => setConfig((c) => ({...c, [f.key]: e.target.value}))}
                  />
                  {f.hint ? <span className="mt-1 block text-[11px] text-slate-500">{f.hint}</span> : null}
                </label>
              ))}
              <div className="notice-box notice-box-blue text-xs">
                本流程无需打码（captcha），仅需配置动态代理 IP 网关。邮箱后端等高级 JSON 配置可直接编辑 reg.db 的 system_config 表。
              </div>
            </div>
            <div className="flex items-center justify-end gap-2 border-t border-slate-200 px-4 py-3">
              <button className="btn-secondary" type="button" onClick={() => setShowConfig(false)}>
                取消
              </button>
              <button className="btn-primary" type="button" disabled={savingConfig} onClick={() => void saveConfig()}>
                {savingConfig ? "保存中…" : "保存设置"}
              </button>
            </div>
          </div>
        </div>
      ) : null}

      {logTask ? (
        <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/40 p-4">
          <div className="flex max-h-[80vh] w-full max-w-3xl flex-col border border-slate-200 bg-white shadow-xl">
            <div className="flex items-center justify-between border-b border-slate-200 px-4 py-3">
              <div>
                <h2 className="section-title">任务日志 #{logTask.id}</h2>
                <p className="section-desc">{logTask.email || "未分配邮箱"}</p>
              </div>
              <div className="flex items-center gap-2">
                <button className="btn-secondary" type="button" onClick={() => void openLogs(logTask)}>
                  刷新
                </button>
                <button className="btn-text" type="button" onClick={() => setLogTask(null)}>
                  关闭
                </button>
              </div>
            </div>
            <div className="min-h-0 flex-1 overflow-auto bg-slate-50 p-4 font-mono text-xs">
              {logLoading ? (
                <div className="text-slate-400">加载中…</div>
              ) : logs.length === 0 ? (
                <div className="text-slate-400">暂无日志</div>
              ) : (
                logs.map((l) => (
                  <div key={l.id} className="flex gap-2 border-b border-slate-200/60 py-1">
                    <span className="shrink-0 text-slate-400">
                      {l.created_at ? new Date(l.created_at).toLocaleTimeString() : ""}
                    </span>
                    <span className={`shrink-0 uppercase ${levelStyles[l.level] || "text-slate-600"}`}>{l.level}</span>
                    {l.step ? <span className="shrink-0 text-brand-700">[{l.step}]</span> : null}
                    <span className="text-slate-700">{l.message}</span>
                  </div>
                ))
              )}
            </div>
          </div>
        </div>
      ) : null}
    </main>
  );
}
