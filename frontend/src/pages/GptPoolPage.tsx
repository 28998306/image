import {useCallback, useEffect, useMemo, useState} from "react";
import {
  RegGptBatchDelete,
  RegGptBatchRefresh,
  RegGptDelete,
  RegGptDeleteInvalid,
  RegGptDetail,
  RegGptExport,
  RegGptImport,
  RegGptList,
  RegGptQuota,
  RegGptRefresh,
  RegGptStats,
  RegGptUpdate,
} from "../../wailsjs/go/main/App";
import {dto} from "../../wailsjs/go/models";
import {confirmDialog} from "../components/ConfirmDialog";

type GptRow = dto.GptPoolResp;
type GptStats = dto.GptPoolStatsResp;

const statusLabels: Record<string, string> = {
  valid: "有效",
  invalid: "失效",
  disabled: "停用",
  cooldown: "冷却中",
};

const statusStyles: Record<string, string> = {
  valid: "bg-emerald-50 text-emerald-700 border-emerald-200",
  invalid: "bg-rose-50 text-rose-700 border-rose-200",
  disabled: "bg-slate-100 text-slate-500 border-slate-200",
  cooldown: "bg-amber-50 text-amber-700 border-amber-200",
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

function fmtTime(ms?: number): string {
  if (!ms) return "-";
  return new Date(ms).toLocaleString();
}

function fmtExpiry(ms?: number): {text: string; cls: string} {
  if (!ms) return {text: "未知", cls: "text-slate-400"};
  const now = Date.now();
  const d = new Date(ms);
  if (ms <= now) return {text: `已过期 ${d.toLocaleDateString()}`, cls: "text-rose-600"};
  const hrs = (ms - now) / 3600000;
  const cls = hrs < 6 ? "text-amber-600" : "text-slate-600";
  return {text: d.toLocaleString(), cls};
}

function fmtQuota(row: GptRow): string {
  const r = row.image_quota_remaining;
  if (r == null) return "-";
  return `剩 ${r}%`;
}

export function GptPoolPage() {
  const [rows, setRows] = useState<GptRow[]>([]);
  const [total, setTotal] = useState(0);
  const [stats, setStats] = useState<GptStats | null>(null);
  const [page, setPage] = useState(1);
  const [statusFilter, setStatusFilter] = useState("");
  const [keyword, setKeyword] = useState("");
  const [keywordInput, setKeywordInput] = useState("");
  const [selected, setSelected] = useState<Set<number>>(new Set());
  const [loading, setLoading] = useState(false);
  const [busy, setBusy] = useState(false);
  const [message, setMessage] = useState("");

  const [exportText, setExportText] = useState<string | null>(null);
  const [copied, setCopied] = useState(false);

  const [importOpen, setImportOpen] = useState(false);
  const [importText, setImportText] = useState("");

  const [editRow, setEditRow] = useState<GptRow | null>(null);
  const [editForm, setEditForm] = useState({
    status: "",
    notes: "",
    accessToken: "",
    refreshToken: "",
    expiresAt: "",
  });

  const totalPages = Math.max(1, Math.ceil(total / pageSize));

  const refresh = useCallback(async () => {
    setLoading(true);
    try {
      const [list, st] = await Promise.all([
        RegGptList(statusFilter, keyword, page, pageSize),
        RegGptStats(),
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
  }, [statusFilter, keyword, page]);

  useEffect(() => {
    void refresh();
  }, [refresh]);

  const allChecked = rows.length > 0 && rows.every((r) => selected.has(r.id));
  const toggleAll = () => setSelected(allChecked ? new Set() : new Set(rows.map((r) => r.id)));
  const toggleOne = (id: number) => {
    setSelected((prev) => {
      const next = new Set(prev);
      next.has(id) ? next.delete(id) : next.add(id);
      return next;
    });
  };

  const selectedIds = useMemo(() => Array.from(selected), [selected]);

  const doBatchDelete = async () => {
    if (selectedIds.length === 0) return;
    if (!(await confirmDialog(`确定删除选中的 ${selectedIds.length} 个账号？`, {danger: true}))) return;
    const n = await RegGptBatchDelete(selectedIds);
    setMessage(`已删除 ${n} 个账号`);
    await refresh();
  };

  const doDelete = async (id: number) => {
    if (!(await confirmDialog("确定删除该账号？", {danger: true}))) return;
    await RegGptDelete(id);
    setMessage("已删除");
    await refresh();
  };

  const doDeleteInvalid = async () => {
    if (!(await confirmDialog("确定删除所有「失效」状态的账号？", {danger: true}))) return;
    const n = await RegGptDeleteInvalid();
    setMessage(`已删除 ${n} 个失效账号`);
    await refresh();
  };

  const doRefreshOne = async (id: number) => {
    setBusy(true);
    setMessage("正在刷新有效期…");
    try {
      const r = await RegGptRefresh(id);
      if (r?.message) {
        setMessage(r.message);
      } else {
        setMessage(r?.ok ? `刷新成功，新有效期 ${fmtExpiry(r.expires_at).text}` : "刷新完成");
      }
      await refresh();
    } catch (err) {
      setMessage(`刷新失败：${String(err)}`);
    } finally {
      setBusy(false);
    }
  };

  const doBatchRefresh = async () => {
    const ids = selectedIds;
    const scopeLabel = ids.length > 0 ? `选中 ${ids.length} 个` : "全部可刷新";
    if (!(await confirmDialog(`确定刷新${scopeLabel}账号的有效期？该操作会逐个调用 OpenAI，可能较慢。`))) return;
    setBusy(true);
    setMessage("正在批量刷新有效期…");
    try {
      const r = await RegGptBatchRefresh(ids);
      setMessage(`刷新完成：成功 ${r?.refreshed ?? 0}，失败 ${r?.failed ?? 0}`);
      await refresh();
    } catch (err) {
      setMessage(`批量刷新失败：${String(err)}`);
    } finally {
      setBusy(false);
    }
  };

  const doQuota = async (id: number) => {
    setBusy(true);
    setMessage("正在查询额度…");
    try {
      const q = await RegGptQuota(id);
      if (!q?.ok) {
        setMessage(`额度查询失败：${q?.message || "未知错误"}`);
      } else {
        const parts: string[] = [];
        if (q.image_quota_remaining != null) {
          parts.push(
            `5小时剩余 ${q.image_quota_remaining}%${q.image_quota_reset_at ? `（${fmtTime(q.image_quota_reset_at)} 重置）` : ""}`,
          );
        }
        if (q.weekly_remaining != null) {
          parts.push(
            `每周剩余 ${q.weekly_remaining}%${q.weekly_reset_at ? `（${fmtTime(q.weekly_reset_at)} 重置）` : ""}`,
          );
        }
        if (q.credits_balance) parts.push(`余额 ${q.credits_balance}`);
        const detail = parts.length ? parts.join(" · ") : "无用量信息";
        setMessage(`查询成功 · 套餐 ${q.plan_type || "?"} · ${detail}`);
      }
      await refresh();
    } catch (err) {
      setMessage(`额度查询失败：${String(err)}`);
    } finally {
      setBusy(false);
    }
  };

  const doExport = async (scope: string) => {
    try {
      const ids = scope === "selected" ? selectedIds : [];
      if (scope === "selected" && ids.length === 0) {
        setMessage("请先勾选要导出的账号");
        return;
      }
      const text = await RegGptExport(scope, ids);
      setExportText(text || "");
      setCopied(false);
    } catch (err) {
      setMessage(`导出失败：${String(err)}`);
    }
  };

  const copyExport = async () => {
    if (exportText == null) return;
    try {
      await navigator.clipboard.writeText(exportText);
      setCopied(true);
    } catch {
      setCopied(false);
    }
  };

  const onPickImportFile = async (file: File | undefined) => {
    if (!file) return;
    const text = await file.text();
    setImportText((prev) => (prev.trim() ? prev + "\n" + text : text));
  };

  const doImport = async () => {
    if (!importText.trim()) {
      setMessage("请粘贴或选择要导入的 JSON");
      return;
    }
    setBusy(true);
    try {
      const r = await RegGptImport(importText);
      const errPart = r?.errors?.length ? `，错误 ${r.errors.length} 条` : "";
      setMessage(`导入完成：新增 ${r?.imported ?? 0}，更新 ${r?.updated ?? 0}，跳过 ${r?.skipped ?? 0}${errPart}`);
      setImportOpen(false);
      setImportText("");
      await refresh();
    } catch (err) {
      setMessage(`导入失败：${String(err)}`);
    } finally {
      setBusy(false);
    }
  };

  const openEdit = async (row: GptRow) => {
    setEditRow(row);
    setEditForm({
      status: row.status || "",
      notes: row.notes || "",
      accessToken: "",
      refreshToken: "",
      expiresAt: row.expires_at ? new Date(row.expires_at).toISOString().slice(0, 16) : "",
    });
    try {
      const d = await RegGptDetail(row.id);
      setEditForm((f) => ({...f, accessToken: d?.access_token || "", refreshToken: d?.refresh_token || ""}));
    } catch {
      // 读取明文失败时保持留空（留空=不改）
    }
  };

  const saveEdit = async () => {
    if (!editRow) return;
    setBusy(true);
    try {
      let expMs = 0;
      if (editForm.expiresAt) {
        const t = new Date(editForm.expiresAt).getTime();
        if (!Number.isNaN(t)) expMs = t;
      }
      await RegGptUpdate(
        editRow.id,
        editForm.status,
        editForm.notes,
        "",
        editForm.accessToken.trim(),
        editForm.refreshToken.trim(),
        "",
        expMs,
      );
      setMessage("已保存");
      setEditRow(null);
      await refresh();
    } catch (err) {
      setMessage(`保存失败：${String(err)}`);
    } finally {
      setBusy(false);
    }
  };

  const exportLines = exportText ? exportText.split("\n").filter(Boolean).length : 0;

  return (
    <main className="flex min-h-0 flex-1 flex-col overflow-hidden">
      <div className="flex shrink-0 items-center justify-between border-b border-slate-200 bg-white px-5 py-3">
        <div>
          <h1 className="app-title">
            号池管理
            <span className="ml-2 align-middle text-xs font-normal text-blue-600">账号购买联系 QQ 28998306</span>
          </h1>
          <p className="mt-0.5 text-sm text-slate-500">管理已注册的 GPT 账号，支持导入导出、编辑、额度查询、有效期刷新</p>
        </div>
        <div className="flex items-center gap-2">
          {message ? <span className="max-w-[300px] truncate text-xs text-slate-500">{message}</span> : null}
          <button className="btn-secondary" type="button" onClick={() => setImportOpen(true)}>
            导入
          </button>
          <button className="btn-secondary" type="button" onClick={() => void doExport("valid")}>
            导出有效
          </button>
          <button className="btn-secondary" type="button" onClick={() => void doExport("all")}>
            导出全部
          </button>
          <button className="btn-secondary" type="button" disabled={busy || loading} onClick={() => void refresh()}>
            刷新
          </button>
        </div>
      </div>

      <div className="min-h-0 flex-1 overflow-auto p-4">
        <div className="mb-4 grid grid-cols-3 gap-2 sm:grid-cols-5">
          <StatCard label="总数" value={stats?.total ?? 0} tone="border-slate-200 bg-white text-slate-700" />
          <StatCard label="有效" value={stats?.valid ?? 0} tone={statusStyles.valid} />
          <StatCard label="失效" value={stats?.invalid ?? 0} tone={statusStyles.invalid} />
          <StatCard label="冷却中" value={stats?.cooldown ?? 0} tone={statusStyles.cooldown} />
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

            <div className="ml-auto flex items-center gap-2">
              <button
                className="btn-secondary"
                type="button"
                disabled={busy}
                onClick={() => void doBatchRefresh()}
              >
                更新有效期{selectedIds.length > 0 ? `(${selectedIds.length})` : "(全部)"}
              </button>
              <button
                className="btn-secondary"
                type="button"
                disabled={selectedIds.length === 0}
                onClick={() => void doExport("selected")}
              >
                导出选中({selectedIds.length})
              </button>
              <button
                className="btn-secondary border-rose-300 text-rose-600"
                type="button"
                onClick={() => void doDeleteInvalid()}
              >
                删除失效
              </button>
              <button
                className="btn-secondary border-rose-300 text-rose-600"
                type="button"
                disabled={selectedIds.length === 0}
                onClick={() => void doBatchDelete()}
              >
                删除选中({selectedIds.length})
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
                  <th className="w-[220px] px-3 py-2">邮箱</th>
                  <th className="px-3 py-2">状态</th>
                  <th className="px-3 py-2">套餐</th>
                  <th className="px-3 py-2">用量(5h)</th>
                  <th className="px-3 py-2">有效期至</th>
                  <th className="px-3 py-2">备注</th>
                  <th className="w-[180px] px-3 py-2">操作</th>
                </tr>
              </thead>
              <tbody>
                {rows.map((row) => {
                  const exp = fmtExpiry(row.expires_at);
                  return (
                    <tr key={row.id} className="border-b border-slate-100 hover:bg-slate-50">
                      <td className="px-3 py-2">
                        <input
                          type="checkbox"
                          className="accent-brand-600"
                          checked={selected.has(row.id)}
                          onChange={() => toggleOne(row.id)}
                        />
                      </td>
                      <td className="px-3 py-2 text-slate-800">
                        <span className="block max-w-[200px] truncate" title={row.email}>
                          {row.email}
                        </span>
                      </td>
                      <td className="px-3 py-2">
                        <span className={`inline-flex border px-1.5 py-0.5 text-[11px] ${statusStyles[row.status] || ""}`}>
                          {statusLabels[row.status] || row.status}
                        </span>
                      </td>
                      <td className="px-3 py-2 text-xs text-slate-500">{row.plan_type || "-"}</td>
                      <td className="px-3 py-2 text-xs tabular-nums text-slate-600">{fmtQuota(row)}</td>
                      <td className={`px-3 py-2 text-xs ${exp.cls}`} title={fmtTime(row.last_refresh_at)}>
                        {exp.text}
                      </td>
                      <td className="px-3 py-2 text-xs text-slate-500">
                        <span className="block max-w-[160px] truncate" title={row.notes}>
                          {row.notes || "-"}
                        </span>
                      </td>
                      <td className="px-3 py-2">
                        <div className="flex items-center gap-2 whitespace-nowrap text-[11px]">
                          <button className="btn-text text-brand-600" type="button" disabled={busy} onClick={() => void doRefreshOne(row.id)}>
                            刷新
                          </button>
                          <button className="btn-text text-brand-600" type="button" disabled={busy} onClick={() => void doQuota(row.id)}>
                            额度
                          </button>
                          <button className="btn-text" type="button" onClick={() => void openEdit(row)}>
                            编辑
                          </button>
                          <button className="btn-text text-rose-600" type="button" onClick={() => void doDelete(row.id)}>
                            删除
                          </button>
                        </div>
                      </td>
                    </tr>
                  );
                })}
                {rows.length === 0 ? (
                  <tr>
                    <td colSpan={8} className="px-3 py-10 text-center text-sm text-slate-400">
                      {loading ? "加载中…" : "暂无已注册账号"}
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

      {importOpen ? (
        <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/40 p-4">
          <div className="flex max-h-[85vh] w-full max-w-2xl flex-col border border-slate-200 bg-white shadow-xl">
            <div className="flex items-center justify-between border-b border-slate-200 px-4 py-3">
              <div>
                <h2 className="section-title">导入号池</h2>
                <p className="section-desc">支持 sub2api-data 导出、codex 单文件、扁平 JSON 或 JSON 数组，按邮箱去重更新</p>
              </div>
              <button className="btn-text" type="button" onClick={() => setImportOpen(false)}>
                关闭
              </button>
            </div>
            <div className="min-h-0 flex-1 space-y-3 overflow-auto p-4">
              <div className="flex items-center gap-2">
                <label className="btn-secondary cursor-pointer">
                  选择文件
                  <input
                    type="file"
                    accept=".json,application/json"
                    multiple
                    className="hidden"
                    onChange={(e) => {
                      const files = Array.from(e.target.files || []);
                      void Promise.all(files.map((f) => onPickImportFile(f)));
                      e.target.value = "";
                    }}
                  />
                </label>
                <button className="btn-text" type="button" onClick={() => setImportText("")}>
                  清空
                </button>
              </div>
              <textarea
                className="field-input h-72 w-full resize-none font-mono text-xs"
                placeholder='粘贴 JSON，例如 {"type":"codex","email":"...","access_token":"...","refresh_token":"..."}'
                value={importText}
                onChange={(e) => setImportText(e.target.value)}
              />
            </div>
            <div className="flex items-center justify-end gap-2 border-t border-slate-200 px-4 py-3">
              <button className="btn-text" type="button" onClick={() => setImportOpen(false)}>
                取消
              </button>
              <button className="btn-primary" type="button" disabled={busy} onClick={() => void doImport()}>
                {busy ? "导入中…" : "开始导入"}
              </button>
            </div>
          </div>
        </div>
      ) : null}

      {editRow ? (
        <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/40 p-4">
          <div className="flex max-h-[85vh] w-full max-w-lg flex-col border border-slate-200 bg-white shadow-xl">
            <div className="flex items-center justify-between border-b border-slate-200 px-4 py-3">
              <div>
                <h2 className="section-title">编辑账号</h2>
                <p className="section-desc">{editRow.email}</p>
              </div>
              <button className="btn-text" type="button" onClick={() => setEditRow(null)}>
                关闭
              </button>
            </div>
            <div className="min-h-0 flex-1 space-y-3 overflow-auto p-4">
              <div>
                <label className="field-label">状态</label>
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
              </div>
              <div>
                <label className="field-label">有效期至</label>
                <input
                  type="datetime-local"
                  className="field-input"
                  value={editForm.expiresAt}
                  onChange={(e) => setEditForm((f) => ({...f, expiresAt: e.target.value}))}
                />
              </div>
              <div>
                <label className="field-label">备注</label>
                <input
                  className="field-input"
                  value={editForm.notes}
                  onChange={(e) => setEditForm((f) => ({...f, notes: e.target.value}))}
                />
              </div>
              <div>
                <label className="field-label">access_token（可编辑，留空则不改）</label>
                <textarea
                  className="field-input h-24 resize-none font-mono text-xs"
                  placeholder={editRow.has_access_token ? "读取中…" : "未设置"}
                  value={editForm.accessToken}
                  onChange={(e) => setEditForm((f) => ({...f, accessToken: e.target.value}))}
                />
              </div>
              <div>
                <label className="field-label">refresh_token（可编辑，留空则不改）</label>
                <textarea
                  className="field-input h-24 resize-none font-mono text-xs"
                  placeholder={editRow.has_refresh_token ? "读取中…" : "未设置"}
                  value={editForm.refreshToken}
                  onChange={(e) => setEditForm((f) => ({...f, refreshToken: e.target.value}))}
                />
              </div>
            </div>
            <div className="flex items-center justify-end gap-2 border-t border-slate-200 px-4 py-3">
              <button className="btn-text" type="button" onClick={() => setEditRow(null)}>
                取消
              </button>
              <button className="btn-primary" type="button" disabled={busy} onClick={() => void saveEdit()}>
                {busy ? "保存中…" : "保存"}
              </button>
            </div>
          </div>
        </div>
      ) : null}

      {exportText != null ? (
        <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/40 p-4">
          <div className="flex max-h-[80vh] w-full max-w-2xl flex-col border border-slate-200 bg-white shadow-xl">
            <div className="flex items-center justify-between border-b border-slate-200 px-4 py-3">
              <div>
                <h2 className="section-title">导出结果</h2>
                <p className="section-desc">共 {exportLines} 条 · 格式 email----password----access----refresh</p>
              </div>
              <div className="flex items-center gap-2">
                <button className="btn-secondary" type="button" onClick={() => void copyExport()}>
                  {copied ? "已复制" : "复制"}
                </button>
                <button className="btn-text" type="button" onClick={() => setExportText(null)}>
                  关闭
                </button>
              </div>
            </div>
            <div className="min-h-0 flex-1 overflow-auto p-4">
              <textarea readOnly className="field-input h-72 w-full resize-none font-mono text-xs" value={exportText} />
            </div>
          </div>
        </div>
      ) : null}
    </main>
  );
}
