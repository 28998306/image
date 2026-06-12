import {useEffect, useMemo, useState} from "react";
import {LoadHistory, OpenLocalFile} from "../../wailsjs/go/main/App";
import {BrowserOpenURL, ClipboardSetText} from "../../wailsjs/runtime";
import type {HistoryItem} from "../types";

const PAGE_SIZE = 12;

const modeLabels = {
  text: "文生图",
  image: "图生图",
  video: "视频生成",
};

function formatTime(value: string) {
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) {
    return value;
  }
  return date.toLocaleString("zh-CN", {
    month: "2-digit",
    day: "2-digit",
    hour: "2-digit",
    minute: "2-digit",
  });
}

interface HistoryPageProps {
  statsVersion: number;
}

export function HistoryPage({statsVersion}: HistoryPageProps) {
  const [history, setHistory] = useState<HistoryItem[]>([]);
  const [selectedId, setSelectedId] = useState("");
  const [page, setPage] = useState(1);

  const totalPages = Math.max(Math.ceil(history.length / PAGE_SIZE), 1);
  const pagedHistory = useMemo(
    () => history.slice((page - 1) * PAGE_SIZE, page * PAGE_SIZE),
    [history, page],
  );

  const selectedItem = useMemo(
    () => history.find((item) => item.id === selectedId) || history[0],
    [history, selectedId],
  );

  const openSelectedItem = async () => {
    if (!selectedItem) {
      return;
    }
    const opened = selectedItem.localPath ? await OpenLocalFile(selectedItem.localPath) : false;
    if (!opened) {
      BrowserOpenURL(selectedItem.videoUrl || selectedItem.imageUrl);
    }
  };

  useEffect(() => {
    LoadHistory().then((items) => {
      const nextItems = items as HistoryItem[];
      setHistory(nextItems);
      setSelectedId((currentId) => currentId || nextItems[0]?.id || "");
    });
  }, [statsVersion]);

  return (
    <main className="flex min-h-0 flex-1 flex-col overflow-hidden p-4">
      <section className="panel min-h-0 flex-1 overflow-hidden">
        <div className="panel-header flex items-center justify-between">
          <div>
            <h1 className="section-title">历史记录</h1>
            <p className="section-desc">查看已生成图片、提示词和生成参数</p>
          </div>
          <div className="flex items-center gap-2">
            <span className="status-chip">{history.length} 条记录</span>
            <button className="btn-secondary" disabled={page <= 1} type="button" onClick={() => setPage((value) => Math.max(value - 1, 1))}>
              上一页
            </button>
            <span className="text-xs text-slate-500">{page} / {totalPages}</span>
            <button className="btn-secondary" disabled={page >= totalPages} type="button" onClick={() => setPage((value) => Math.min(value + 1, totalPages))}>
              下一页
            </button>
          </div>
        </div>

        {history.length === 0 ? (
          <div className="flex h-[calc(100%-65px)] items-center justify-center">
            <div className="notice-box max-w-[360px] text-center">暂无历史记录。生成成功后会自动保存到这里。</div>
          </div>
        ) : (
          <div className="grid h-[calc(100%-65px)] grid-cols-[minmax(420px,1fr)_340px] gap-4 p-4">
            <div className="grid auto-rows-min grid-cols-3 gap-3 overflow-auto pr-1">
              {pagedHistory.map((item) => (
                <button
                  key={item.id}
                  className={`overflow-hidden border bg-white text-left transition ${
                    selectedItem?.id === item.id ? "border-brand-600" : "border-slate-200 hover:border-blue-300"
                  }`}
                  type="button"
                  onClick={() => setSelectedId(item.id)}
                >
                  <div className="flex h-36 items-center justify-center bg-slate-100">
                    {item.videoUrl ? (
                      item.coverUrl || item.imageUrl ? (
                        <img alt={item.title} className="h-full w-full object-cover" src={item.coverUrl || item.imageUrl} />
                      ) : (
                        <span className="text-xs text-slate-500">视频</span>
                      )
                    ) : (
                      <img alt={item.title} className="h-full w-full object-cover" src={item.imageUrl} />
                    )}
                  </div>
                  <div className="p-3">
                    <div className="truncate text-sm text-slate-900">{item.title}</div>
                    <div className="mt-1 text-[11px] text-slate-500">{formatTime(item.createdAt)}</div>
                  </div>
                </button>
              ))}
            </div>

            <aside className="flex min-h-0 flex-col border border-slate-200 bg-slate-50 p-4">
              {selectedItem ? (
                <>
                  <div className="flex min-h-[220px] items-center justify-center border border-slate-200 bg-white">
                    {selectedItem.videoUrl ? (
                      <video className="max-h-[280px] w-full bg-black" controls poster={selectedItem.coverUrl || selectedItem.imageUrl}>
                        <source src={selectedItem.videoUrl} type="video/mp4" />
                      </video>
                    ) : (
                      <img alt={selectedItem.title} className="max-h-[280px] w-full object-contain" src={selectedItem.imageUrl} />
                    )}
                  </div>

                  <h2 className="mt-4 text-base font-medium text-slate-900">{selectedItem.title}</h2>
                  <div className="mt-2 flex gap-2">
                    <span className="tag min-h-0 text-[11px]">{modeLabels[selectedItem.mode]}</span>
                    <span className="tag min-h-0 text-[11px]">{selectedItem.quality.toUpperCase()}</span>
                  </div>

                  <dl className="mt-4 space-y-2 text-xs">
                    <div className="flex justify-between gap-3">
                      <dt className="text-slate-500">模型</dt>
                      <dd className="text-right text-slate-800">{selectedItem.model}</dd>
                    </div>
                    <div className="flex justify-between">
                      <dt className="text-slate-500">尺寸</dt>
                      <dd className="text-slate-800">{selectedItem.size}</dd>
                    </div>
                    <div className="flex justify-between">
                      <dt className="text-slate-500">时间</dt>
                      <dd className="text-slate-800">{formatTime(selectedItem.createdAt)}</dd>
                    </div>
                    <div className="flex justify-between gap-3">
                      <dt className="text-slate-500">本地文件</dt>
                      <dd className="truncate text-right text-slate-800">{selectedItem.localPath || "未保存"}</dd>
                    </div>
                  </dl>

                  <label className="mt-4 flex min-h-0 flex-1 flex-col">
                    <span className="field-label">提示词</span>
                    <textarea className="field-input min-h-[140px] flex-1 resize-none" readOnly value={selectedItem.prompt} />
                  </label>

                  <div className="mt-4 grid gap-2">
                    <button className="btn-secondary w-full" type="button" onClick={() => ClipboardSetText(selectedItem.prompt)}>
                      复制提示词
                    </button>
                    <button className="btn-primary w-full" type="button" onClick={openSelectedItem}>
                      {selectedItem.videoUrl ? "打开视频" : "打开图片"}
                    </button>
                  </div>
                </>
              ) : null}
            </aside>
          </div>
        )}
      </section>
    </main>
  );
}
