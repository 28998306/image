import {useEffect, useMemo, useRef, useState} from "react";
import {
  OpenLocalFile,
  OpenOutputDir,
  RegConfig,
  RegGptStats,
  RegPoolGenModels,
  RegPoolGenerate,
  RegPoolGenerateFile,
} from "../../wailsjs/go/main/App";
import {EventsOff, EventsOn} from "../../wailsjs/runtime";
import {main} from "../../wailsjs/go/models";

interface LogLine {
  ts: number;
  msg: string;
}

type GenModel = main.PoolGenModel;
type GenImage = main.PoolGenImage;
type FileResult = main.PoolFileResult;
type GenKind = "image" | "ppt" | "psd";
type TaskStatus = "running" | "success" | "error";

interface GenTask {
  id: string;
  seq: number;
  kind: GenKind;
  model: string;
  prompt: string;
  count: number;
  size: string;
  refsCount: number;
  status: TaskStatus;
  account: string;
  images: GenImage[];
  file: FileResult | null;
  error: string;
  startedAt: number;
  endedAt: number;
}

const kindTabs: {id: GenKind; label: string}[] = [
  {id: "image", label: "图片"},
  {id: "ppt", label: "可编辑 PPT"},
  {id: "psd", label: "可编辑 PSD"},
];

const kindLabel: Record<GenKind, string> = {
  image: "图片",
  ppt: "可编辑 PPT",
  psd: "可编辑 PSD",
};

// GPT Image 2 专用尺寸表（最长边 ≤ 3840；4K 档实际面积约 ~2480²，非真 4096²）。
const sizeOptions = [
  {value: "1024x1024", label: "1K · 1:1 · 1024²"},
  {value: "1536x1024", label: "1K · 3:2 · 1536×1024"},
  {value: "1024x1536", label: "1K · 2:3 · 1024×1536"},
  {value: "1152x864", label: "1K · 4:3 · 1152×864"},
  {value: "864x1152", label: "1K · 3:4 · 864×1152"},
  {value: "1120x896", label: "1K · 5:4 · 1120×896"},
  {value: "896x1120", label: "1K · 4:5 · 896×1120"},
  {value: "1280x720", label: "1K · 16:9 · 1280×720"},
  {value: "720x1280", label: "1K · 9:16 · 720×1280"},
  {value: "1456x624", label: "1K · 21:9 · 1456×624"},
  {value: "2048x2048", label: "2K · 1:1 · 2048²"},
  {value: "2496x1664", label: "2K · 3:2 · 2496×1664"},
  {value: "1664x2496", label: "2K · 2:3 · 1664×2496"},
  {value: "2304x1728", label: "2K · 4:3 · 2304×1728"},
  {value: "1728x2304", label: "2K · 3:4 · 1728×2304"},
  {value: "2240x1792", label: "2K · 5:4 · 2240×1792"},
  {value: "1792x2240", label: "2K · 4:5 · 1792×2240"},
  {value: "2560x1440", label: "2K · 16:9 · 2560×1440"},
  {value: "1440x2560", label: "2K · 9:16 · 1440×2560"},
  {value: "3024x1296", label: "2K · 21:9 · 3024×1296"},
  {value: "2480x2480", label: "4K · 1:1 · 2480²"},
  {value: "3056x2032", label: "4K · 3:2 · 3056×2032"},
  {value: "2032x3056", label: "4K · 2:3 · 2032×3056"},
  {value: "2880x2160", label: "4K · 4:3 · 2880×2160"},
  {value: "2160x2880", label: "4K · 3:4 · 2160×2880"},
  {value: "2784x2224", label: "4K · 5:4 · 2784×2224"},
  {value: "2224x2784", label: "4K · 4:5 · 2224×2784"},
  {value: "3328x1872", label: "4K · 16:9 · 3328×1872"},
  {value: "1872x3328", label: "4K · 9:16 · 1872×3328"},
  {value: "3808x1632", label: "4K · 21:9 · 3808×1632"},
];

const qualityOptions = [
  {value: "", label: "自动"},
  {value: "low", label: "低（快）"},
  {value: "medium", label: "中"},
  {value: "high", label: "高"},
];

const formatOptions = [
  {value: "png", label: "PNG"},
  {value: "jpeg", label: "JPEG"},
  {value: "webp", label: "WebP"},
];

function fmtElapsed(ms: number): string {
  const s = Math.max(0, Math.floor(ms / 1000));
  if (s < 60) return `${s}s`;
  return `${Math.floor(s / 60)}m${(s % 60).toString().padStart(2, "0")}s`;
}

function fmtClock(ts: number): string {
  const d = new Date(ts);
  const p = (n: number) => n.toString().padStart(2, "0");
  return `${p(d.getHours())}:${p(d.getMinutes())}:${p(d.getSeconds())}`;
}

interface PoolGenPageProps {
  onStatsUpdate?: () => void;
}

export function PoolGenPage({onStatsUpdate}: PoolGenPageProps) {
  const [models, setModels] = useState<GenModel[]>([]);
  const [model, setModel] = useState("codex-gpt-image-2");
  const [prompt, setPrompt] = useState("");
  const [size, setSize] = useState("1024x1024");
  const [quality, setQuality] = useState("");
  const [format, setFormat] = useState("png");
  const [count, setCount] = useState(1);
  const [refs, setRefs] = useState<string[]>([]);

  const [kind, setKind] = useState<GenKind>("image");
  const [validCount, setValidCount] = useState(0);
  const [message, setMessage] = useState("");
  const [tasks, setTasks] = useState<GenTask[]>([]);
  const [expandedId, setExpandedId] = useState<string | null>(null);
  const [logOpenId, setLogOpenId] = useState<string | null>(null);
  const [logs, setLogs] = useState<Record<string, LogLine[]>>({});
  const [preview, setPreview] = useState<string | null>(null);
  const [proxyConfigured, setProxyConfigured] = useState<boolean | null>(null);
  const seqRef = useRef(0);
  const [, setTick] = useState(0);

  // 监听后端推送的上游日志（poolgen:log），按 taskId 归档。
  useEffect(() => {
    EventsOn("poolgen:log", (payload: {taskId: string; ts: number; msg: string}) => {
      if (!payload || !payload.taskId) return;
      setLogs((prev) => {
        const list = prev[payload.taskId] ? [...prev[payload.taskId]] : [];
        list.push({ts: payload.ts || Date.now(), msg: payload.msg || ""});
        return {...prev, [payload.taskId]: list};
      });
    });
    return () => EventsOff("poolgen:log");
  }, []);

  useEffect(() => {
    RegPoolGenModels()
      .then((m) => {
        setModels(m || []);
        if (m && m.length > 0) setModel(m[0].id);
      })
      .catch(() => setModels([]));
  }, []);

  useEffect(() => {
    RegGptStats()
      .then((s) => setValidCount(s?.valid ?? 0))
      .catch(() => setValidCount(0));
    RegConfig()
      .then((cfg) => setProxyConfigured(Boolean(cfg && (cfg["proxy.dynamic_url"] || "").trim())))
      .catch(() => setProxyConfigured(null));
  }, []);

  const runningCount = useMemo(() => tasks.filter((t) => t.status === "running").length, [tasks]);

  // 有运行中任务时，每秒刷新一次用于显示耗时计时。
  useEffect(() => {
    if (runningCount === 0) return;
    const h = window.setInterval(() => setTick((t) => t + 1), 1000);
    return () => window.clearInterval(h);
  }, [runningCount]);

  const isEdit = refs.length > 0;

  const onAddRefs = (files: FileList | null) => {
    if (!files) return;
    Array.from(files).forEach((file) => {
      const reader = new FileReader();
      reader.onload = () => {
        if (typeof reader.result === "string") {
          setRefs((prev) => [...prev, reader.result as string].slice(0, 4));
        }
      };
      reader.readAsDataURL(file);
    });
  };

  const updateTask = (id: string, patch: Partial<GenTask>) => {
    setTasks((prev) => prev.map((t) => (t.id === id ? {...t, ...patch} : t)));
  };

  const submit = () => {
    if (!prompt.trim()) {
      setMessage("请输入提示词");
      return;
    }
    if (kind === "psd" && refs.length === 0) {
      setMessage("PSD 生成需要至少一张参考图");
      return;
    }
    setMessage("");
    const id = `${Date.now()}-${Math.random().toString(36).slice(2, 7)}`;
    const seq = (seqRef.current += 1);
    // 快照当前参数，后续表单改动不影响已提交任务。
    const snap = {
      kind,
      model,
      prompt: prompt.trim(),
      size,
      quality,
      format,
      count,
      refs: [...refs],
    };
    const task: GenTask = {
      id,
      seq,
      kind: snap.kind,
      model: snap.model,
      prompt: snap.prompt,
      count: snap.count,
      size: snap.size,
      refsCount: snap.refs.length,
      status: "running",
      account: "",
      images: [],
      file: null,
      error: "",
      startedAt: Date.now(),
      endedAt: 0,
    };
    setTasks((prev) => [task, ...prev]);

    const run = async () => {
      try {
        if (snap.kind === "image") {
          const r = await RegPoolGenerate(
            snap.prompt,
            snap.model,
            snap.size,
            snap.quality,
            snap.format,
            snap.count,
            snap.refs,
            0,
            id,
          );
          updateTask(id, {
            status: "success",
            images: r?.images || [],
            account: r?.account || "",
            endedAt: Date.now(),
          });
        } else {
          const r = await RegPoolGenerateFile(snap.kind, snap.prompt, snap.refs, 0, id);
          updateTask(id, {
            status: "success",
            file: r,
            account: r?.account || "",
            endedAt: Date.now(),
          });
        }
        onStatsUpdate?.();
      } catch (err) {
        updateTask(id, {status: "error", error: String(err), endedAt: Date.now()});
      }
    };
    void run();
  };

  const canSubmit = prompt.trim().length > 0;

  return (
    <main className="flex min-h-0 flex-1 flex-col overflow-hidden">
      <div className="flex shrink-0 items-center justify-between border-b border-slate-200 bg-white px-5 py-3">
        <div>
          <h1 className="app-title">号池生图</h1>
          <p className="mt-0.5 text-sm text-slate-500">
            用已注册号池账号出图/生成可编辑文件（一号一并发，支持多任务并行、随机选号、出错自动换号重试）
          </p>
        </div>
        <div className="flex items-center gap-3 text-xs text-slate-500">
          <span>
            可用账号 <span className="font-medium text-emerald-600">{validCount}</span>
          </span>
          <button className="btn-secondary" type="button" onClick={() => void OpenOutputDir()}>
            打开输出目录
          </button>
        </div>
      </div>

      {proxyConfigured === false ? (
        <div className="shrink-0 border-b border-amber-200 bg-amber-50 px-5 py-2 text-xs text-amber-800">
          ⚠ 尚未配置代理网关，当前直连本机 IP 出图易触发风控；建议先到「系统设置 → 公共代理网关」配置动态代理。
        </div>
      ) : null}

      <div className="grid min-h-0 flex-1 grid-cols-[360px_1fr] overflow-hidden">
        {/* 左侧参数面板 */}
        <div className="min-h-0 overflow-auto border-r border-slate-200 bg-white p-4">
          <div className="space-y-4">
            <div className="flex gap-1 rounded bg-slate-100 p-1">
              {kindTabs.map((t) => (
                <button
                  key={t.id}
                  type="button"
                  className={`flex-1 rounded px-2 py-1.5 text-xs transition ${
                    kind === t.id ? "bg-white text-brand-700 shadow-sm" : "text-slate-500 hover:text-slate-700"
                  }`}
                  onClick={() => setKind(t.id)}
                >
                  {t.label}
                </button>
              ))}
            </div>

            {kind === "image" ? (
              <div>
                <label className="field-label">模型</label>
                <select className="field-input" value={model} onChange={(e) => setModel(e.target.value)}>
                  {models.map((m) => (
                    <option key={m.id} value={m.id}>
                      {m.name}
                    </option>
                  ))}
                </select>
                <p className="mt-1 text-[11px] text-slate-400">
                  {models.find((m) => m.id === model)?.description || "Codex 逆向画图"}
                </p>
              </div>
            ) : (
              <p className="rounded border border-amber-200 bg-amber-50 px-3 py-2 text-[11px] text-amber-700">
                {kind.toUpperCase()} 由 ChatGPT 网页会话生成（模型 gpt-5-5-thinking），仅 Plus/Team/Pro
                账号可用，耗时较长（最长约 20 分钟），完成后产出可编辑文件 + 素材压缩包。
                {kind === "psd" ? "PSD 需要先上传一张参考图。" : ""}
              </p>
            )}

            <div>
              <label className="field-label">提示词{kind !== "image" ? "（补充需求，可留模板默认）" : ""}</label>
              <textarea
                className="field-input h-36 resize-none"
                placeholder={
                  kind === "image"
                    ? isEdit
                      ? "描述你想如何修改参考图…"
                      : "描述你想生成的画面…"
                    : "例如：科技风产品发布会，蓝色主色调，6 页…"
                }
                value={prompt}
                onChange={(e) => setPrompt(e.target.value)}
              />
            </div>

            {kind === "image" && model !== "gpt-image-2" ? (
              <div className="grid grid-cols-2 gap-3">
                <div>
                  <label className="field-label">尺寸</label>
                  <select className="field-input" value={size} onChange={(e) => setSize(e.target.value)}>
                    {sizeOptions.map((s) => (
                      <option key={s.value} value={s.value}>
                        {s.label}
                      </option>
                    ))}
                  </select>
                </div>
                <div>
                  <label className="field-label">质量</label>
                  <select className="field-input" value={quality} onChange={(e) => setQuality(e.target.value)}>
                    {qualityOptions.map((q) => (
                      <option key={q.value} value={q.value}>
                        {q.label}
                      </option>
                    ))}
                  </select>
                </div>
              </div>
            ) : null}

            {kind === "image" ? (
              <div className="grid grid-cols-2 gap-3">
                {model !== "gpt-image-2" ? (
                  <div>
                    <label className="field-label">输出格式</label>
                    <select className="field-input" value={format} onChange={(e) => setFormat(e.target.value)}>
                      {formatOptions.map((f) => (
                        <option key={f.value} value={f.value}>
                          {f.label}
                        </option>
                      ))}
                    </select>
                  </div>
                ) : null}
                <div>
                  <label className="field-label">数量 (n){model === "gpt-image-2" ? "·套图" : ""}</label>
                  <select
                    className="field-input"
                    value={count}
                    onChange={(e) => setCount(parseInt(e.target.value, 10))}
                  >
                    {[1, 2, 3, 4].map((n) => (
                      <option key={n} value={n}>
                        {n} 张
                      </option>
                    ))}
                  </select>
                </div>
              </div>
            ) : null}

            <div>
              <label className="field-label">
                {kind === "image"
                  ? "参考图（编辑模式，最多 4 张）"
                  : kind === "psd"
                    ? "参考图（PSD 必填，最多 4 张）"
                    : "参考图（可选，最多 4 张）"}
              </label>
              <div className="flex flex-wrap items-center gap-2">
                {refs.map((r, i) => (
                  <div key={i} className="relative h-14 w-14 overflow-hidden border border-slate-200">
                    <img src={r} alt="" className="h-full w-full object-cover" />
                    <button
                      className="absolute right-0 top-0 bg-black/50 px-1 text-[10px] text-white"
                      type="button"
                      onClick={() => setRefs((prev) => prev.filter((_, idx) => idx !== i))}
                    >
                      ×
                    </button>
                  </div>
                ))}
                {refs.length < 4 ? (
                  <label className="flex h-14 w-14 cursor-pointer items-center justify-center border border-dashed border-slate-300 text-lg text-slate-400 hover:border-brand-400">
                    +
                    <input
                      type="file"
                      accept="image/*"
                      multiple
                      className="hidden"
                      onChange={(e) => {
                        onAddRefs(e.target.files);
                        e.target.value = "";
                      }}
                    />
                  </label>
                ) : null}
              </div>
              {kind === "image" && isEdit ? (
                <p className="mt-1 text-[11px] text-amber-600">已切换为「编辑」模式（基于参考图改图）</p>
              ) : null}
            </div>

            <button className="btn-primary w-full" type="button" disabled={!canSubmit} onClick={submit}>
              {kind === "image" ? (isEdit ? "提交编辑任务" : "提交出图任务") : `提交 ${kind.toUpperCase()} 任务`}
            </button>
            <p className="text-[11px] text-slate-400">
              点击即提交一个任务，可连续提交多个并行执行；每个任务独占一个账号，号都在忙时会自动排队。
            </p>
            {message ? <p className="text-xs text-rose-500">{message}</p> : null}
          </div>
        </div>

        {/* 右侧任务列表 */}
        <div className="flex min-h-0 flex-col bg-slate-50">
          <div className="flex shrink-0 items-center justify-between border-b border-slate-200 bg-white px-4 py-2">
            <div className="text-sm font-medium text-slate-700">
              任务列表
              <span className="ml-2 text-xs font-normal text-slate-400">
                共 {tasks.length} · 进行中 {runningCount}
              </span>
            </div>
            {tasks.some((t) => t.status !== "running") ? (
              <button
                className="btn-text text-xs text-slate-500"
                type="button"
                onClick={() => {
                  setTasks((prev) => {
                    const keep = prev.filter((t) => t.status === "running");
                    const keepIds = new Set(keep.map((t) => t.id));
                    setLogs((lp) => {
                      const next: Record<string, LogLine[]> = {};
                      for (const [k, v] of Object.entries(lp)) if (keepIds.has(k)) next[k] = v;
                      return next;
                    });
                    return keep;
                  });
                }}
              >
                清空已完成
              </button>
            ) : null}
          </div>

          <div className="min-h-0 flex-1 overflow-auto p-3">
            {tasks.length === 0 ? (
              <div className="flex h-full items-center justify-center text-sm text-slate-400">
                提交任务后，进度和结果会在这里一行一个显示
              </div>
            ) : (
              <div className="space-y-2">
                {tasks.map((t) => {
                  const expanded = expandedId === t.id;
                  const logOpen = logOpenId === t.id;
                  const taskLogs = logs[t.id] || [];
                  const elapsed = (t.endedAt || Date.now()) - t.startedAt;
                  return (
                    <div key={t.id} className="border border-slate-200 bg-white">
                      {/* 行头 */}
                      <div className="flex items-center gap-2 px-3 py-2">
                        <span className="text-[11px] tabular-nums text-slate-400">#{t.seq}</span>
                        <StatusBadge status={t.status} />
                        <span className="shrink-0 rounded-sm bg-slate-100 px-1.5 py-0.5 text-[11px] text-slate-600">
                          {kindLabel[t.kind]}
                        </span>
                        <span className="min-w-0 flex-1 truncate text-xs text-slate-600" title={t.prompt}>
                          {t.prompt}
                        </span>
                        <span className="shrink-0 text-[11px] tabular-nums text-slate-400">{fmtElapsed(elapsed)}</span>
                        <button
                          className="btn-text shrink-0 text-[11px] text-brand-600"
                          type="button"
                          onClick={() => setExpandedId(expanded ? null : t.id)}
                        >
                          {expanded ? "收起" : "详情"}
                        </button>
                        <button
                          className="btn-text shrink-0 text-[11px] text-slate-500"
                          type="button"
                          onClick={() => setLogOpenId(logOpen ? null : t.id)}
                        >
                          日志{taskLogs.length > 0 ? `(${taskLogs.length})` : ""}
                        </button>
                      </div>

                      {/* 进度条 / 结果摘要 */}
                      {t.status === "running" ? (
                        <div className="px-3 pb-2">
                          <div className="h-1 w-full overflow-hidden rounded bg-slate-100">
                            <div className="poolgen-progress h-full w-1/3 rounded bg-brand-500" />
                          </div>
                          <div className="mt-1 text-[11px] text-slate-400">
                            {t.account ? `账号 ${t.account} · ` : ""}生成中…（最长约 20 分钟）
                          </div>
                        </div>
                      ) : null}

                      {t.status === "error" ? (
                        <div className="border-t border-rose-100 bg-rose-50 px-3 py-2 text-[11px] text-rose-600">
                          {t.error}
                        </div>
                      ) : null}

                      {/* 上游日志面板 */}
                      {logOpen ? (
                        <div className="border-t border-slate-800 bg-slate-900 px-3 py-2">
                          {taskLogs.length === 0 ? (
                            <div className="text-[11px] text-slate-500">暂无日志…</div>
                          ) : (
                            <div className="max-h-52 space-y-0.5 overflow-auto font-mono text-[11px] leading-relaxed">
                              {taskLogs.map((l, i) => (
                                <div key={i} className="flex gap-2">
                                  <span className="shrink-0 text-slate-500">{fmtClock(l.ts)}</span>
                                  <span className="min-w-0 break-words text-slate-200">{l.msg}</span>
                                </div>
                              ))}
                            </div>
                          )}
                        </div>
                      ) : null}

                      {/* 缩略图（图片任务） */}
                      {t.status === "success" && t.images.length > 0 ? (
                        <div className="flex flex-wrap gap-2 px-3 pb-3">
                          {t.images.map((img, i) => (
                            <button
                              key={i}
                              type="button"
                              className="h-20 w-20 overflow-hidden border border-slate-200 bg-slate-100"
                              onClick={() => setPreview(img.dataUrl)}
                              title="点击放大"
                            >
                              <img src={img.dataUrl} alt="" className="h-full w-full object-cover" />
                            </button>
                          ))}
                        </div>
                      ) : null}

                      {/* 文件任务结果 */}
                      {t.status === "success" && t.file ? (
                        <div className="space-y-1.5 px-3 pb-3 text-[11px]">
                          <div className="flex items-center justify-between gap-3">
                            <span className="truncate text-slate-600">
                              {t.file.primaryName || `${t.kind.toUpperCase()} 主文件`}
                            </span>
                            {t.file.primaryPath ? (
                              <button
                                className="btn-secondary shrink-0"
                                type="button"
                                onClick={() => void OpenLocalFile(t.file!.primaryPath)}
                              >
                                打开文件
                              </button>
                            ) : null}
                          </div>
                          {t.file.zipPath ? (
                            <div className="flex items-center justify-between gap-3">
                              <span className="truncate text-slate-600">{t.file.zipName || "素材压缩包"}</span>
                              <button
                                className="btn-secondary shrink-0"
                                type="button"
                                onClick={() => void OpenLocalFile(t.file!.zipPath)}
                              >
                                打开压缩包
                              </button>
                            </div>
                          ) : null}
                        </div>
                      ) : null}

                      {/* 详情展开 */}
                      {expanded ? (
                        <div className="space-y-2 border-t border-slate-100 bg-slate-50 px-3 py-2 text-[11px] text-slate-500">
                          <DetailRow label="类型" value={kindLabel[t.kind]} />
                          {t.kind === "image" ? <DetailRow label="模型" value={t.model} /> : null}
                          {t.kind === "image" ? <DetailRow label="尺寸" value={t.size} /> : null}
                          {t.kind === "image" ? <DetailRow label="数量" value={`${t.count} 张`} /> : null}
                          <DetailRow label="参考图" value={`${t.refsCount} 张`} />
                          <DetailRow label="账号" value={t.account || "—"} />
                          <DetailRow
                            label="状态"
                            value={
                              t.status === "running" ? "生成中" : t.status === "success" ? "成功" : "失败"
                            }
                          />
                          <DetailRow label="耗时" value={fmtElapsed(elapsed)} />
                          <div>
                            <div className="text-slate-400">提示词</div>
                            <div className="mt-0.5 whitespace-pre-wrap break-words text-slate-600">{t.prompt}</div>
                          </div>
                          {t.status === "success" && t.images.length > 0 ? (
                            <div className="grid grid-cols-2 gap-2 lg:grid-cols-3">
                              {t.images.map((img, i) => (
                                <div key={i} className="border border-slate-200 bg-white">
                                  <button
                                    type="button"
                                    className="block aspect-square w-full overflow-hidden bg-slate-100"
                                    onClick={() => setPreview(img.dataUrl)}
                                  >
                                    <img src={img.dataUrl} alt="" className="h-full w-full object-contain" />
                                  </button>
                                  <div className="flex items-center justify-between px-2 py-1 text-[10px] text-slate-500">
                                    <span>
                                      {img.width}×{img.height}
                                    </span>
                                    {img.localPath ? (
                                      <button
                                        className="btn-text text-brand-600"
                                        type="button"
                                        onClick={() => void OpenLocalFile(img.localPath)}
                                      >
                                        打开
                                      </button>
                                    ) : (
                                      <span className="text-slate-300">未保存</span>
                                    )}
                                  </div>
                                </div>
                              ))}
                            </div>
                          ) : null}
                        </div>
                      ) : null}
                    </div>
                  );
                })}
              </div>
            )}
          </div>
        </div>
      </div>

      {preview ? (
        <div
          className="fixed inset-0 z-50 flex items-center justify-center bg-black/70 p-6"
          onClick={() => setPreview(null)}
        >
          <img src={preview} alt="" className="max-h-full max-w-full object-contain" />
        </div>
      ) : null}
    </main>
  );
}

function StatusBadge({status}: {status: TaskStatus}) {
  if (status === "running") {
    return (
      <span className="shrink-0 rounded-sm bg-blue-100 px-1.5 py-0.5 text-[10px] text-blue-700">生成中</span>
    );
  }
  if (status === "success") {
    return (
      <span className="shrink-0 rounded-sm bg-emerald-100 px-1.5 py-0.5 text-[10px] text-emerald-700">成功</span>
    );
  }
  return <span className="shrink-0 rounded-sm bg-rose-100 px-1.5 py-0.5 text-[10px] text-rose-700">失败</span>;
}

function DetailRow({label, value}: {label: string; value: string}) {
  return (
    <div className="flex gap-2">
      <span className="w-12 shrink-0 text-slate-400">{label}</span>
      <span className="min-w-0 break-words text-slate-600">{value}</span>
    </div>
  );
}
