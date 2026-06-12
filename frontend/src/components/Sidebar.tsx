import {useEffect, useState} from "react";
import {LoadHistory} from "../../wailsjs/go/main/App";
import type {AppPage} from "../types";

const navItems = [
  {id: "poolgen", label: "号池生图", badge: "Codex"},
  {id: "mailpool", label: "邮箱管理", badge: ""},
  {id: "register", label: "号池注册", badge: ""},
  {id: "gptpool", label: "号池管理", badge: ""},
  {id: "history", label: "历史记录", badge: ""},
  {id: "studio", label: "API生图生视频", badge: "API"},
  {id: "settings", label: "系统设置", badge: ""},
];

interface SidebarProps {
  activePage: AppPage;
  statsVersion: number;
  onNavigate: (page: AppPage) => void;
}

export function Sidebar({activePage, statsVersion, onNavigate}: SidebarProps) {
  const [historyCount, setHistoryCount] = useState(0);

  useEffect(() => {
    LoadHistory()
      .then((items) => setHistoryCount(items.length))
      .catch(() => setHistoryCount(0));
  }, [statsVersion]);
  return (
    <aside className="flex h-full w-[180px] flex-col bg-brand-900 text-white">
      <div className="border-b border-white/10 px-4 py-4">
        <div className="text-base font-medium tracking-wide">Web2Img AI</div>
        <div className="mt-1 text-xs text-blue-100">企业级 AI 生图工作台</div>
      </div>

      <nav className="flex-1 px-2.5 py-4">
        <div className="mb-3 px-2 text-[11px] uppercase tracking-[0.18em] text-blue-200">
          功能导航
        </div>
        <div className="space-y-1">
          {navItems.map((item) => (
            <button
              key={item.label}
              className={`flex w-full items-center justify-between rounded px-3 py-2.5 text-left text-xs transition ${
                activePage === item.id
                  ? "border border-blue-300/30 bg-blue-600 text-white"
                  : "text-blue-100 hover:bg-white/8 hover:text-white"
              }`}
              type="button"
              onClick={() => onNavigate(item.id as AppPage)}
            >
              <span>{item.label}</span>
              {item.id === "history" && historyCount > 0 ? (
                <span className="rounded-sm bg-white/12 px-1.5 py-0.5 text-[10px] text-blue-50">
                  {historyCount}
                </span>
              ) : item.badge ? (
                <span className="rounded-sm bg-white/12 px-1.5 py-0.5 text-[10px] text-blue-50">
                  {item.badge}
                </span>
              ) : null}
            </button>
          ))}
        </div>
      </nav>

      <div className="border-t border-white/10 p-3">
        <div className="rounded border border-blue-300/20 bg-white/8 p-3">
          <div className="text-xs text-blue-50">API 状态</div>
          <div className="mt-2 flex items-center gap-2 text-xs text-blue-100">
            <span className="h-2 w-2 rounded-sm bg-emerald-400" />
            已连接模拟服务
          </div>
        </div>
      </div>
    </aside>
  );
}
