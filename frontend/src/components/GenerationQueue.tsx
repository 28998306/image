import type {QueueItem} from "../types";

interface GenerationQueueProps {
  queue: QueueItem[];
}

const statusMeta = {
  running: {label: "生成中", className: "text-brand-700 bg-blue-50 border-blue-200"},
  waiting: {label: "等待中", className: "text-amber-700 bg-amber-50 border-amber-200"},
  done: {label: "已完成", className: "text-emerald-700 bg-emerald-50 border-emerald-200"},
  failed: {label: "失败", className: "text-red-700 bg-red-50 border-red-200"},
};

const modeLabels = {
  text: "文生图",
  image: "图生图",
  video: "视频生成",
};

export function GenerationQueue({queue}: GenerationQueueProps) {
  return (
    <section className="border-t border-slate-200 bg-white px-5 py-3">
      <div className="mb-2 flex items-center justify-between">
        <div className="text-sm font-medium text-slate-900">生成队列</div>
        <button className="btn-text" type="button">
          查看全部
        </button>
      </div>
      <div className="grid grid-cols-3 gap-3">
        {queue.map((item) => (
          <div key={item.id} className="border border-slate-200 bg-slate-50 p-3">
            <div className="flex items-center justify-between gap-2">
              <div className="truncate text-xs text-slate-900">{item.title}</div>
              <span className={`border px-1.5 py-0.5 text-[10px] ${statusMeta[item.status].className}`}>
                {statusMeta[item.status].label}
              </span>
            </div>
            <div className="mt-2 flex items-center justify-between text-[11px] text-slate-500">
              <span>{modeLabels[item.mode]}</span>
              <span>{item.progress}%</span>
            </div>
            {item.message ? <div className="mt-1 truncate text-[11px] text-slate-500">{item.message}</div> : null}
            <div className="mt-2 h-1.5 bg-slate-200">
              <div className="h-full bg-brand-600" style={{width: `${item.progress}%`}} />
            </div>
          </div>
        ))}
      </div>
    </section>
  );
}
