import {useCallback, useEffect, useMemo, useState} from "react";
import {
  RegMailBatchDelete,
  RegMailClearAll,
  RegMailDelete,
  RegMailDeleteByStatus,
  RegMailFetch,
  RegMailImport,
  RegMailList,
  RegMailReset,
  RegMailStats,
  RegMailUpdate,
} from "../../wailsjs/go/main/App";
import {dto, mailbox} from "../../wailsjs/go/models";
import {confirmDialog} from "../components/ConfirmDialog";

type MailRow = dto.MailPoolResp;
type MailStats = dto.MailPoolStatsResp;
type MailMessage = mailbox.MailMessage;

const statusLabels: Record<string, string> = {
  available: "可用",
  in_use: "占用中",
  registered: "已注册",
  failed: "失败",
  disabled: "停用",
};

const statusStyles: Record<string, string> = {
  available: "bg-emerald-50 text-emerald-700 border-emerald-200",
  in_use: "bg-amber-50 text-amber-700 border-amber-200",
  registered: "bg-blue-50 text-brand-700 border-blue-200",
  failed: "bg-rose-50 text-rose-700 border-rose-200",
  disabled: "bg-slate-100 text-slate-500 border-slate-200",
};

const modeLabels: Record<string, string> = {
  outlook_graph: "Outlook Graph",
  outlook_imap: "Outlook IMAP",
  tempmail: "TempMail",
  cf: "Cloudflare",
};

const pageSize = 12;

function StatCard({label, value, tone}: {label: string; value: number; tone: string}) {
  return (
    <div className={`border px-3 py-2 ${tone}`}>
      <div className="text-[11px] opacity-80">{label}</div>
      <div className="mt-0.5 text-lg font-medium tabular-nums">{value}</div>
    </div>
  );
}

export function MailPoolPage() {
  const [rows, setRows] = useState<MailRow[]>([]);
  const [total, setTotal] = useState(0);
  const [stats, setStats] = useState<MailStats | null>(null);
  const [page, setPage] = useState(1);
  const [statusFilter, setStatusFilter] = useState("");
  const [modeFilter, setModeFilter] = useState("");
  const [keyword, setKeyword] = useState("");
  const [keywordInput, setKeywordInput] = useState("");
  const [selected, setSelected] = useState<Set<number>>(new Set());
  const [loading, setLoading] = useState(false);
  const [message, setMessage] = useState("");

  const [showImport, setShowImport] = useState(false);
  const [importText, setImportText] = useState("");
  const [importMode, setImportMode] = useState("outlook_graph");
  const [importSep, setImportSep] = useState("----");
  const [importing, setImporting] = useState(false);

  const [editRow, setEditRow] = useState<MailRow | null>(null);
  const [editForm, setEditForm] = useState({email: "", password: "", clientId: "", refreshToken: "", mode: "", status: ""});
  const [saving, setSaving] = useState(false);

  const [fetchRow, setFetchRow] = useState<MailRow | null>(null);
  const [messages, setMessages] = useState<MailMessage[]>([]);
  const [fetching, setFetching] = useState(false);
  const [fetchError, setFetchError] = useState("");
  const [openMsgId, setOpenMsgId] = useState<string | null>(null);

  const totalPages = Math.max(1, Math.ceil(total / pageSize));

  const refresh = useCallback(async () => {
    setLoading(true);
    try {
      const [list, st] = await Promise.all([
        RegMailList(statusFilter, modeFilter, keyword, page, pageSize),
        RegMailStats(),
      ]);
      setRows(list.items || []);
      setTotal(list.total || 0);
      setStats(st);
      setSelected(new Set());
    } catch (err) {
      setMessage(`加载失败：${String(err)}`);
    } finally {
      setLoading(false);
    }
  }, [statusFilter, modeFilter, keyword, page]);

  useEffect(() => {
    void refresh();
  }, [refresh]);

  const allChecked = rows.length > 0 && rows.every((r) => selected.has(r.id));
  const toggleAll = () => {
    setSelected(allChecked ? new Set() : new Set(rows.map((r) => r.id)));
  };
  const toggleOne = (id: number) => {
    setSelected((prev) => {
      const next = new Set(prev);
      next.has(id) ? next.delete(id) : next.add(id);
      return next;
    });
  };

  const selectedIds = useMemo(() => Array.from(selected), [selected]);

  const doImport = async () => {
    if (!importText.trim()) {
      setMessage("请粘贴邮箱数据");
      return;
    }
    setImporting(true);
    try {
      const res = await RegMailImport(importText, importMode, importSep);
      setMessage(`导入完成：成功 ${res.imported} 条，跳过 ${res.skipped} 条`);
      setShowImport(false);
      setImportText("");
      setPage(1);
      await refresh();
    } catch (err) {
      setMessage(`导入失败：${String(err)}`);
    } finally {
      setImporting(false);
    }
  };

  const doBatchDelete = async () => {
    if (selectedIds.length === 0) return;
    const n = await RegMailBatchDelete(selectedIds);
    setMessage(`已删除 ${n} 条`);
    await refresh();
  };

  const doReset = async () => {
    if (selectedIds.length === 0) return;
    const n = await RegMailReset(selectedIds);
    setMessage(`已重置 ${n} 条为可用`);
    await refresh();
  };

  const doDeleteByStatus = async (status: string) => {
    if (!(await confirmDialog(`确定删除全部「${statusLabels[status]}」邮箱？`, {danger: true}))) return;
    const n = await RegMailDeleteByStatus(status);
    setMessage(`已清理 ${n} 条「${statusLabels[status]}」邮箱`);
    setPage(1);
    await refresh();
  };

  const doClearAll = async () => {
    if (!(await confirmDialog("确定清空全部邮箱？此操作不可恢复！", {danger: true}))) return;
    const n = await RegMailClearAll();
    setMessage(`已清空 ${n} 条邮箱`);
    setPage(1);
    await refresh();
  };

  const doDelete = async (id: number) => {
    await RegMailDelete(id);
    setMessage("已删除");
    await refresh();
  };

  const openEdit = (row: MailRow) => {
    setEditRow(row);
    setEditForm({email: row.email, password: "", clientId: row.client_id, refreshToken: "", mode: row.mode, status: row.status});
  };

  const saveEdit = async () => {
    if (!editRow) return;
    setSaving(true);
    try {
      await RegMailUpdate(
        editRow.id,
        editForm.email,
        editForm.password,
        editForm.clientId,
        editForm.refreshToken,
        editForm.mode,
        editForm.status,
      );
      setMessage("已保存");
      setEditRow(null);
      await refresh();
    } catch (err) {
      setMessage(`保存失败：${String(err)}`);
    } finally {
      setSaving(false);
    }
  };

  const openFetch = async (row: MailRow) => {
    setFetchRow(row);
    setMessages([]);
    setFetchError("");
    setOpenMsgId(null);
    setFetching(true);
    try {
      const data = await RegMailFetch(row.id, 15);
      setMessages(data || []);
    } catch (err) {
      setFetchError(String(err));
    } finally {
      setFetching(false);
    }
  };

  return (
    <main className="flex min-h-0 flex-1 flex-col overflow-hidden">
      <div className="flex shrink-0 items-center justify-between border-b border-slate-200 bg-white px-5 py-3">
        <div>
          <h1 className="app-title">
            邮箱管理
            <span className="ml-2 align-middle text-xs font-normal text-blue-600">账号购买联系 QQ 28998306</span>
          </h1>
          <p className="mt-0.5 text-sm text-slate-500">共享邮箱池，供号池注册任务自动领用</p>
        </div>
        <div className="flex items-center gap-2">
          {message ? <span className="text-xs text-slate-500">{message}</span> : null}
          <button className="btn-secondary" type="button" onClick={() => void refresh()}>
            刷新
          </button>
          <button className="btn-primary" type="button" onClick={() => setShowImport(true)}>
            导入邮箱
          </button>
        </div>
      </div>

      <div className="min-h-0 flex-1 overflow-auto p-4">
        <div className="mb-4 grid grid-cols-3 gap-2 sm:grid-cols-6">
          <StatCard label="总数" value={stats?.total ?? 0} tone="border-slate-200 bg-white text-slate-700" />
          <StatCard label="可用" value={stats?.available ?? 0} tone={statusStyles.available} />
          <StatCard label="占用中" value={stats?.in_use ?? 0} tone={statusStyles.in_use} />
          <StatCard label="已注册" value={stats?.registered ?? 0} tone={statusStyles.registered} />
          <StatCard label="失败" value={stats?.failed ?? 0} tone={statusStyles.failed} />
          <StatCard label="停用" value={stats?.disabled ?? 0} tone={statusStyles.disabled} />
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
            <select
              className="field-input max-w-[160px]"
              value={modeFilter}
              onChange={(e) => {
                setPage(1);
                setModeFilter(e.target.value);
              }}
            >
              <option value="">全部后端</option>
              {Object.entries(modeLabels).map(([k, v]) => (
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
                placeholder="搜索邮箱地址"
                value={keywordInput}
                onChange={(e) => setKeywordInput(e.target.value)}
              />
              <button className="btn-secondary shrink-0 whitespace-nowrap" type="submit">
                搜索
              </button>
            </form>

            <div className="ml-auto flex items-center gap-2">
              <button
                className="btn-secondary"
                type="button"
                disabled={selectedIds.length === 0}
                onClick={() => void doReset()}
              >
                重置可用({selectedIds.length})
              </button>
              <button
                className="btn-secondary"
                type="button"
                disabled={selectedIds.length === 0}
                onClick={() => void doBatchDelete()}
              >
                删除选中({selectedIds.length})
              </button>
              <button className="btn-secondary" type="button" onClick={() => void doDeleteByStatus("failed")}>
                清理失败
              </button>
              <button className="btn-secondary" type="button" onClick={() => void doDeleteByStatus("registered")}>
                删除已注册
              </button>
              <button
                className="btn-secondary border-rose-300 text-rose-600"
                type="button"
                onClick={() => void doClearAll()}
              >
                清空全部
              </button>
            </div>
          </div>

          <div className="overflow-auto">
            <table className="w-full border-collapse text-sm">
              <thead>
                <tr className="border-b border-slate-200 bg-slate-50 text-left text-xs text-slate-500">
                  <th className="w-10 px-3 py-2">
                    <input type="checkbox" className="accent-brand-600" checked={allChecked} onChange={toggleAll} />
                  </th>
                  <th className="px-3 py-2">邮箱</th>
                  <th className="px-3 py-2">后端</th>
                  <th className="px-3 py-2">状态</th>
                  <th className="px-3 py-2">失败次数</th>
                  <th className="px-3 py-2">占用方</th>
                  <th className="px-3 py-2">导入时间</th>
                  <th className="w-40 px-3 py-2">操作</th>
                </tr>
              </thead>
              <tbody>
                {rows.map((row) => (
                  <tr key={row.id} className="border-b border-slate-100 hover:bg-slate-50">
                    <td className="px-3 py-2">
                      <input
                        type="checkbox"
                        className="accent-brand-600"
                        checked={selected.has(row.id)}
                        onChange={() => toggleOne(row.id)}
                      />
                    </td>
                    <td className="px-3 py-2">
                      <div className="text-slate-800">{row.email}</div>
                      {row.last_error ? (
                        <div className="mt-0.5 max-w-[280px] truncate text-[11px] text-rose-500" title={row.last_error}>
                          {row.last_error}
                        </div>
                      ) : null}
                    </td>
                    <td className="px-3 py-2 text-xs text-slate-500">{modeLabels[row.mode] || row.mode}</td>
                    <td className="px-3 py-2">
                      <span className={`inline-flex border px-1.5 py-0.5 text-[11px] ${statusStyles[row.status] || ""}`}>
                        {statusLabels[row.status] || row.status}
                      </span>
                    </td>
                    <td className="px-3 py-2 tabular-nums text-slate-600">{row.failure_count}</td>
                    <td className="px-3 py-2 text-xs text-slate-500">{row.used_by_provider || "-"}</td>
                    <td className="px-3 py-2 text-xs text-slate-500">
                      {row.imported_at ? new Date(row.imported_at).toLocaleString() : "-"}
                    </td>
                    <td className="px-3 py-2">
                      <div className="flex items-center gap-2">
                        <button className="btn-text text-[11px]" type="button" onClick={() => void openFetch(row)}>
                          收件
                        </button>
                        <button className="btn-text text-[11px]" type="button" onClick={() => openEdit(row)}>
                          编辑
                        </button>
                        <button className="btn-text text-[11px] text-rose-600" type="button" onClick={() => void doDelete(row.id)}>
                          删除
                        </button>
                      </div>
                    </td>
                  </tr>
                ))}
                {rows.length === 0 ? (
                  <tr>
                    <td colSpan={8} className="px-3 py-10 text-center text-sm text-slate-400">
                      {loading ? "加载中…" : "暂无邮箱数据，点击右上角导入"}
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

      {editRow ? (
        <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/40 p-4">
          <div className="w-full max-w-lg border border-slate-200 bg-white shadow-xl">
            <div className="flex items-center justify-between border-b border-slate-200 px-4 py-3">
              <h2 className="section-title">编辑邮箱 #{editRow.id}</h2>
              <button className="btn-text" type="button" onClick={() => setEditRow(null)}>
                关闭
              </button>
            </div>
            <div className="space-y-3 p-4">
              <label className="block">
                <span className="field-label">邮箱地址</span>
                <input
                  className="field-input"
                  value={editForm.email}
                  onChange={(e) => setEditForm((f) => ({...f, email: e.target.value}))}
                />
              </label>
              <div className="grid grid-cols-2 gap-3">
                <label>
                  <span className="field-label">收件后端</span>
                  <select
                    className="field-input"
                    value={editForm.mode}
                    onChange={(e) => setEditForm((f) => ({...f, mode: e.target.value}))}
                  >
                    {Object.entries(modeLabels).map(([k, v]) => (
                      <option key={k} value={k}>
                        {v}
                      </option>
                    ))}
                  </select>
                </label>
                <label>
                  <span className="field-label">状态</span>
                  <select
                    className="field-input"
                    value={editForm.status}
                    onChange={(e) => setEditForm((f) => ({...f, status: e.target.value}))}
                  >
                    {Object.entries(statusLabels).map(([k, v]) => (
                      <option key={k} value={k}>
                        {v}
                      </option>
                    ))}
                  </select>
                </label>
              </div>
              <label className="block">
                <span className="field-label">Client ID</span>
                <input
                  className="field-input"
                  value={editForm.clientId}
                  onChange={(e) => setEditForm((f) => ({...f, clientId: e.target.value}))}
                />
              </label>
              <label className="block">
                <span className="field-label">密码（留空不修改）</span>
                <input
                  className="field-input"
                  placeholder="••••••"
                  value={editForm.password}
                  onChange={(e) => setEditForm((f) => ({...f, password: e.target.value}))}
                />
              </label>
              <label className="block">
                <span className="field-label">Refresh Token（留空不修改）</span>
                <textarea
                  className="field-input h-20 resize-none font-mono text-xs"
                  placeholder="留空保持原值"
                  value={editForm.refreshToken}
                  onChange={(e) => setEditForm((f) => ({...f, refreshToken: e.target.value}))}
                />
              </label>
            </div>
            <div className="flex items-center justify-end gap-2 border-t border-slate-200 px-4 py-3">
              <button className="btn-secondary" type="button" onClick={() => setEditRow(null)}>
                取消
              </button>
              <button className="btn-primary" type="button" disabled={saving} onClick={() => void saveEdit()}>
                {saving ? "保存中…" : "保存"}
              </button>
            </div>
          </div>
        </div>
      ) : null}

      {fetchRow ? (
        <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/40 p-4">
          <div className="flex max-h-[80vh] w-full max-w-3xl flex-col border border-slate-200 bg-white shadow-xl">
            <div className="flex items-center justify-between border-b border-slate-200 px-4 py-3">
              <div>
                <h2 className="section-title">收取邮件</h2>
                <p className="section-desc">{fetchRow.email}</p>
              </div>
              <div className="flex items-center gap-2">
                <button className="btn-secondary" type="button" disabled={fetching} onClick={() => void openFetch(fetchRow)}>
                  {fetching ? "收取中…" : "重新收取"}
                </button>
                <button className="btn-text" type="button" onClick={() => setFetchRow(null)}>
                  关闭
                </button>
              </div>
            </div>
            <div className="min-h-0 flex-1 overflow-auto p-4">
              {fetching ? (
                <div className="py-10 text-center text-sm text-slate-400">正在连接邮箱并拉取最近邮件…</div>
              ) : fetchError ? (
                <div className="notice-box border-rose-200 bg-rose-50 text-xs text-rose-600">{fetchError}</div>
              ) : messages.length === 0 ? (
                <div className="py-10 text-center text-sm text-slate-400">收件箱为空</div>
              ) : (
                <div className="space-y-2">
                  {messages.map((msg) => (
                    <div key={msg.id} className="border border-slate-200">
                      <button
                        className="flex w-full items-start justify-between gap-3 px-3 py-2 text-left hover:bg-slate-50"
                        type="button"
                        onClick={() => setOpenMsgId((cur) => (cur === msg.id ? null : msg.id))}
                      >
                        <div className="min-w-0">
                          <div className="truncate text-sm text-slate-800">{msg.subject || "(无主题)"}</div>
                          <div className="mt-0.5 truncate text-[11px] text-slate-500">
                            {msg.from} · {msg.folder}
                          </div>
                          {openMsgId !== msg.id ? (
                            <div className="mt-0.5 truncate text-[11px] text-slate-400">{msg.preview}</div>
                          ) : null}
                        </div>
                        <span className="shrink-0 text-[11px] text-slate-400">
                          {msg.received ? new Date(msg.received).toLocaleString() : ""}
                        </span>
                      </button>
                      {openMsgId === msg.id ? (
                        <div className="border-t border-slate-200 bg-slate-50 px-3 py-2 text-xs leading-5 text-slate-700">
                          <div
                            className="max-h-72 overflow-auto whitespace-pre-wrap break-words"
                            dangerouslySetInnerHTML={{__html: msg.body || msg.preview}}
                          />
                        </div>
                      ) : null}
                    </div>
                  ))}
                </div>
              )}
            </div>
          </div>
        </div>
      ) : null}

      {showImport ? (
        <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/40 p-4">
          <div className="w-full max-w-2xl border border-slate-200 bg-white shadow-xl">
            <div className="flex items-center justify-between border-b border-slate-200 px-4 py-3">
              <h2 className="section-title">导入邮箱</h2>
              <button className="btn-text" type="button" onClick={() => setShowImport(false)}>
                关闭
              </button>
            </div>
            <div className="space-y-3 p-4">
              <div className="grid grid-cols-2 gap-3">
                <label>
                  <span className="field-label">收件后端</span>
                  <select className="field-input" value={importMode} onChange={(e) => setImportMode(e.target.value)}>
                    {Object.entries(modeLabels).map(([k, v]) => (
                      <option key={k} value={k}>
                        {v}
                      </option>
                    ))}
                  </select>
                </label>
                <label>
                  <span className="field-label">分隔符</span>
                  <input className="field-input" value={importSep} onChange={(e) => setImportSep(e.target.value)} />
                </label>
              </div>
              <label className="block">
                <span className="field-label">邮箱数据（每行一条，4 段或 7 段格式）</span>
                <textarea
                  className="field-input h-48 resize-none font-mono text-xs"
                  placeholder={"email----password----client_id----refresh_token\n或 7 段 Outlook 格式"}
                  value={importText}
                  onChange={(e) => setImportText(e.target.value)}
                />
              </label>
              <div className="notice-box notice-box-blue text-xs">
                支持「邮箱----密码----ClientID----RefreshToken」4 段，或包含 7 段的 Outlook 导出格式。密码与令牌将本地加密存储。
              </div>
            </div>
            <div className="flex items-center justify-end gap-2 border-t border-slate-200 px-4 py-3">
              <button className="btn-secondary" type="button" onClick={() => setShowImport(false)}>
                取消
              </button>
              <button className="btn-primary" type="button" disabled={importing} onClick={() => void doImport()}>
                {importing ? "导入中…" : "开始导入"}
              </button>
            </div>
          </div>
        </div>
      ) : null}
    </main>
  );
}
